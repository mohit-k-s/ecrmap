package ecs

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"

	"ecrmap/pkg/model"
	"ecrmap/pkg/parser"
)

// Crawler crawls ECS clusters to discover active services and their deployed images.
type Crawler struct {
	client  *ecs.Client
	region  string
	baseCfg aws.Config
}

// NewCrawler creates an ECS Crawler.
func NewCrawler(client *ecs.Client, region string) *Crawler {
	return &Crawler{
		client: client,
		region: region,
	}
}

// NewCrawlerWithConfig creates an ECS Crawler with base AWS configuration for multi-region support.
func NewCrawlerWithConfig(cfg aws.Config, region string) *Crawler {
	regionalCfg := cfg.Copy()
	if region != "" {
		regionalCfg.Region = region
	}
	return &Crawler{
		client:  ecs.NewFromConfig(regionalCfg),
		region:  regionalCfg.Region,
		baseCfg: cfg,
	}
}

// DiscoverOptions controls the discovery behavior.
type DiscoverOptions struct {
	ClusterFilter string // Optional filter for a specific cluster name or ARN
	MaxWorkers    int    // Concurrency limit for DescribeTaskDefinition (default 10)
}

// Discover fetches all active services and their deployed task definitions across clusters.
func (c *Crawler) Discover(ctx context.Context, opts DiscoverOptions) (*model.Snapshot, error) {
	if opts.MaxWorkers <= 0 {
		opts.MaxWorkers = 12
	}

	// 1. Discover Clusters
	clusterArns, err := c.listClusters(ctx, opts.ClusterFilter)
	if err != nil {
		return nil, fmt.Errorf("failed to list clusters: %w", err)
	}

	if len(clusterArns) == 0 {
		return &model.Snapshot{
			Version:     "1.0",
			CreatedAt:   time.Now(),
			Regions:     []string{c.region},
			Deployments: []model.ServiceDeployment{},
		}, nil
	}

	// 2. Crawl services per cluster concurrently
	type clusterResult struct {
		clusterName string
		clusterArn  string
		services    []ecstypes.Service
		err         error
	}

	var wg sync.WaitGroup
	resultChan := make(chan clusterResult, len(clusterArns))

	for _, arn := range clusterArns {
		wg.Add(1)
		go func(clusterArn string) {
			defer wg.Done()
			cName := extractResourceName(clusterArn)
			svcs, err := c.getClusterServices(ctx, clusterArn)
			resultChan <- clusterResult{
				clusterName: cName,
				clusterArn:  clusterArn,
				services:    svcs,
				err:         err,
			}
		}(arn)
	}

	wg.Wait()
	close(resultChan)

	// Collect all services and identify unique task definitions needed
	var allServices []struct {
		clusterName string
		clusterArn  string
		svc         ecstypes.Service
	}
	neededTaskDefs := make(map[string]bool)

	for res := range resultChan {
		if res.err != nil {
			// Log or record, but don't fail entire crawl if one cluster fails
			continue
		}
		for _, svc := range res.services {
			if svc.TaskDefinition != nil && *svc.TaskDefinition != "" {
				neededTaskDefs[*svc.TaskDefinition] = true
				allServices = append(allServices, struct {
					clusterName string
					clusterArn  string
					svc         ecstypes.Service
				}{
					clusterName: res.clusterName,
					clusterArn:  res.clusterArn,
					svc:         svc,
				})
			}
		}
	}

	// 3. Fetch unique Task Definitions in parallel with a worker pool
	taskDefMap, err := c.fetchTaskDefinitions(ctx, neededTaskDefs, opts.MaxWorkers)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch task definitions: %w", err)
	}

	// 4. Assemble final ServiceDeployment objects
	deployments := make([]model.ServiceDeployment, 0, len(allServices))
	for _, item := range allServices {
		svc := item.svc
		taskDefArn := aws.ToString(svc.TaskDefinition)
		td, ok := taskDefMap[taskDefArn]
		if !ok || td == nil {
			continue
		}

		containers := make([]model.ContainerDeployment, 0, len(td.ContainerDefinitions))
		for _, cd := range td.ContainerDefinitions {
			rawURI := aws.ToString(cd.Image)
			parsed := parser.ParseImageURI(rawURI)

			containers = append(containers, model.ContainerDeployment{
				ContainerName: aws.ToString(cd.Name),
				Image:         parsed,
				Essential:     aws.ToBool(cd.Essential),
			})
		}

		revisionStr := ""
		if td.Revision != 0 {
			revisionStr = strconv.FormatInt(int64(td.Revision), 10)
		}

		deployments = append(deployments, model.ServiceDeployment{
			Region:            c.region,
			ClusterName:       item.clusterName,
			ClusterArn:        item.clusterArn,
			ServiceName:       aws.ToString(svc.ServiceName),
			ServiceArn:        aws.ToString(svc.ServiceArn),
			Status:            aws.ToString(svc.Status),
			DesiredCount:      svc.DesiredCount,
			RunningCount:      svc.RunningCount,
			TaskDefinitionArn: taskDefArn,
			TaskFamily:        aws.ToString(td.Family),
			TaskRevision:      revisionStr,
			Containers:        containers,
		})
	}

	return &model.Snapshot{
		Version:     "1.0",
		CreatedAt:   time.Now(),
		Regions:     []string{c.region},
		Deployments: deployments,
	}, nil
}

// DiscoverMultiRegion crawls multiple AWS regions concurrently.
func DiscoverMultiRegion(ctx context.Context, baseCfg aws.Config, regions []string, opts DiscoverOptions) (*model.Snapshot, error) {
	if len(regions) == 0 {
		regions = []string{baseCfg.Region}
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var allDeployments []model.ServiceDeployment
	var firstErr error

	for _, region := range regions {
		r := strings.TrimSpace(region)
		if r == "" {
			continue
		}

		wg.Add(1)
		go func(targetRegion string) {
			defer wg.Done()

			regionalCfg := baseCfg.Copy()
			regionalCfg.Region = targetRegion

			client := ecs.NewFromConfig(regionalCfg)
			crawler := NewCrawler(client, targetRegion)

			snap, err := crawler.Discover(ctx, opts)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("region %s: %w", targetRegion, err)
				}
				mu.Unlock()
				return
			}

			mu.Lock()
			allDeployments = append(allDeployments, snap.Deployments...)
			mu.Unlock()
		}(r)
	}

	wg.Wait()

	// If all regions failed and we got zero deployments, return error
	if len(allDeployments) == 0 && firstErr != nil {
		return nil, firstErr
	}

	return &model.Snapshot{
		Version:     "1.0",
		CreatedAt:   time.Now(),
		Regions:     regions,
		Deployments: allDeployments,
	}, nil
}

func (c *Crawler) listClusters(ctx context.Context, filter string) ([]string, error) {
	paginator := ecs.NewListClustersPaginator(c.client, &ecs.ListClustersInput{})
	var clusterArns []string

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, arn := range page.ClusterArns {
			name := extractResourceName(arn)
			if filter == "" || filter == name || filter == arn {
				clusterArns = append(clusterArns, arn)
			}
		}
	}
	return clusterArns, nil
}

func (c *Crawler) getClusterServices(ctx context.Context, clusterArn string) ([]ecstypes.Service, error) {
	// List service ARNs
	paginator := ecs.NewListServicesPaginator(c.client, &ecs.ListServicesInput{
		Cluster: aws.String(clusterArn),
	})

	var serviceArns []string
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		serviceArns = append(serviceArns, page.ServiceArns...)
	}

	if len(serviceArns) == 0 {
		return nil, nil
	}

	// DescribeServices in batches of 10
	var services []ecstypes.Service
	batchSize := 10
	for i := 0; i < len(serviceArns); i += batchSize {
		end := i + batchSize
		if end > len(serviceArns) {
			end = len(serviceArns)
		}
		batch := serviceArns[i:end]

		resp, err := c.client.DescribeServices(ctx, &ecs.DescribeServicesInput{
			Cluster:  aws.String(clusterArn),
			Services: batch,
		})
		if err != nil {
			return nil, err
		}
		services = append(services, resp.Services...)
	}

	return services, nil
}

func (c *Crawler) fetchTaskDefinitions(ctx context.Context, taskDefArns map[string]bool, maxWorkers int) (map[string]*ecstypes.TaskDefinition, error) {
	result := make(map[string]*ecstypes.TaskDefinition)
	var mu sync.Mutex

	type job struct {
		arn string
	}

	jobs := make(chan job, len(taskDefArns))
	for arn := range taskDefArns {
		jobs <- job{arn: arn}
	}
	close(jobs)

	var wg sync.WaitGroup
	var workerErr error
	var errOnce sync.Once

	workers := maxWorkers
	if workers > len(taskDefArns) {
		workers = len(taskDefArns)
	}
	if workers <= 0 {
		workers = 1
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				if ctx.Err() != nil {
					return
				}
				resp, err := c.client.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{
					TaskDefinition: aws.String(j.arn),
				})
				if err != nil {
					errOnce.Do(func() {
						workerErr = fmt.Errorf("error describing task def %s: %w", j.arn, err)
					})
					continue
				}

				mu.Lock()
				result[j.arn] = resp.TaskDefinition
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	if workerErr != nil {
		return nil, workerErr
	}
	return result, nil
}

func extractResourceName(arn string) string {
	parts := strings.Split(arn, "/")
	return parts[len(parts)-1]
}
