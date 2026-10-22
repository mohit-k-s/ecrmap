package ecs

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"

	"ecrmap/pkg/model"
)

// AnalyzeRepoChurn detects high pull counts and task churn for services running a repository.
func (c *Crawler) AnalyzeRepoChurn(ctx context.Context, snapshot *model.Snapshot, repoName string, window time.Duration) (*model.RepoChurnReport, error) {
	if window <= 0 {
		window = 24 * time.Hour
	}

	// 1. Identify services using this repository
	type svcKey struct {
		clusterArn  string
		clusterName string
		serviceName string
	}

	matchedServices := make(map[svcKey]model.ServiceDeployment)

	for _, dep := range snapshot.Deployments {
		for _, container := range dep.Containers {
			isMatch := strings.EqualFold(container.Image.Repository, repoName)
			if !isMatch {
				parts := strings.Split(container.Image.Repository, "/")
				if len(parts) > 1 && strings.EqualFold(parts[len(parts)-1], repoName) {
					isMatch = true
				}
			}
			if isMatch {
				k := svcKey{
					clusterArn:  dep.ClusterArn,
					clusterName: dep.ClusterName,
					serviceName: dep.ServiceName,
				}
				matchedServices[k] = dep
				break
			}
		}
	}

	if len(matchedServices) == 0 {
		return &model.RepoChurnReport{
			Repository:       repoName,
			Window:           window,
			AnalyzedServices: 0,
			Services:         []model.ServiceChurnStat{},
		}, nil
	}

	// 2. Concurrently inspect stopped tasks per service
	var wg sync.WaitGroup
	var mu sync.Mutex
	churnStats := make([]model.ServiceChurnStat, 0, len(matchedServices))
	since := time.Now().Add(-window)

	for key, dep := range matchedServices {
		wg.Add(1)
		go func(k svcKey, d model.ServiceDeployment) {
			defer wg.Done()

			client := c.client
			if d.Region != "" && d.Region != c.region {
				rcfg := c.baseCfg.Copy()
				rcfg.Region = d.Region
				client = ecs.NewFromConfig(rcfg)
			}

			stat := c.inspectServiceChurn(ctx, client, k.clusterArn, k.clusterName, k.serviceName, d, since, window)

			mu.Lock()
			churnStats = append(churnStats, stat)
			mu.Unlock()
		}(key, dep)
	}

	wg.Wait()

	// 3. Sort by highest estimated pull volume
	sort.Slice(churnStats, func(i, j int) bool {
		return churnStats[i].EstimatedPulls > churnStats[j].EstimatedPulls
	})

	totalPulls := 0
	for _, s := range churnStats {
		totalPulls += s.EstimatedPulls
	}

	return &model.RepoChurnReport{
		Repository:          repoName,
		Window:              window,
		AnalyzedServices:    len(churnStats),
		TotalEstimatedPulls: totalPulls,
		Services:            churnStats,
	}, nil
}

func (c *Crawler) inspectServiceChurn(ctx context.Context, client *ecs.Client, clusterArn, clusterName, serviceName string, dep model.ServiceDeployment, since time.Time, window time.Duration) model.ServiceChurnStat {
	stat := model.ServiceChurnStat{
		Region:         dep.Region,
		ClusterName:    clusterName,
		ServiceName:    serviceName,
		LaunchType:     "EC2", // Default unless found otherwise
		DesiredCount:   dep.DesiredCount,
		RunningCount:   dep.RunningCount,
		ExitCodeCounts: make(map[int32]int),
	}

	if client == nil {
		client = c.client
	}

	// Fetch stopped tasks for this service
	listResp, err := client.ListTasks(ctx, &ecs.ListTasksInput{
		Cluster:       aws.String(clusterArn),
		ServiceName:   aws.String(serviceName),
		DesiredStatus: ecstypes.DesiredStatusStopped,
		MaxResults:    aws.Int32(100),
	})
	if err != nil || len(listResp.TaskArns) == 0 {
		stat.HealthStatus = "HEALTHY"
		stat.EstimatedPulls = int(dep.RunningCount)
		stat.PullsPerHour = float64(stat.EstimatedPulls) / window.Hours()
		return stat
	}

	// Describe the stopped tasks
	descResp, err := client.DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Cluster: aws.String(clusterArn),
		Tasks:   listResp.TaskArns,
	})
	if err != nil {
		stat.HealthStatus = "HEALTHY"
		stat.EstimatedPulls = int(dep.RunningCount)
		return stat
	}

	reasonsCount := make(map[string]int)
	stoppedInWindow := 0

	for _, task := range descResp.Tasks {
		// Detect launch type
		if task.LaunchType != "" {
			stat.LaunchType = string(task.LaunchType)
		}

		// Filter by creation or stop time within window
		taskTime := time.Time{}
		if task.StoppedAt != nil {
			taskTime = *task.StoppedAt
		} else if task.CreatedAt != nil {
			taskTime = *task.CreatedAt
		}

		if !taskTime.IsZero() && taskTime.Before(since) {
			continue
		}

		stoppedInWindow++

		if task.StoppedReason != nil && *task.StoppedReason != "" {
			reasonsCount[*task.StoppedReason]++
		}

		for _, cont := range task.Containers {
			if cont.ExitCode != nil {
				code := *cont.ExitCode
				stat.ExitCodeCounts[code]++
			}
			if cont.Reason != nil && *cont.Reason != "" {
				reasonsCount[*cont.Reason]++
			}
		}
	}

	stat.StoppedCount = stoppedInWindow
	stat.EstimatedPulls = int(dep.RunningCount) + stoppedInWindow
	stat.PullsPerHour = float64(stat.EstimatedPulls) / window.Hours()

	// Determine dominant stop reason and exit code
	var dominantReason string
	maxReasonCount := 0
	for r, count := range reasonsCount {
		if count > maxReasonCount {
			maxReasonCount = count
			dominantReason = r
		}
	}
	stat.PrimaryReason = dominantReason

	var dominantCode int32
	maxCodeCount := 0
	for code, count := range stat.ExitCodeCounts {
		if count > maxCodeCount {
			maxCodeCount = count
			dominantCode = code
		}
	}
	stat.PrimaryExitCode = dominantCode

	// Determine Health Status
	oomCount := stat.ExitCodeCounts[137]
	crashCount := stat.ExitCodeCounts[1]

	if stoppedInWindow >= 10 && (oomCount > 0 || crashCount > 0) {
		stat.HealthStatus = "CRASH-LOOPING"
	} else if stoppedInWindow >= 5 {
		stat.HealthStatus = "HIGH_CHURN"
	} else {
		stat.HealthStatus = "HEALTHY"
	}

	// Generate actionable diagnostic recommendations
	if oomCount > 0 {
		stat.Recommendations = append(stat.Recommendations,
			fmt.Sprintf("%d task(s) exited with code 137 (OOM / Out-Of-Memory killed). Increase container/task memory limits.", oomCount))
	}
	if crashCount > 0 {
		stat.Recommendations = append(stat.Recommendations,
			fmt.Sprintf("%d task(s) crashed with exit code 1 (Application failure on boot). Check container logs.", crashCount))
	}
	if stat.LaunchType == "FARGATE" && stoppedInWindow > 5 {
		stat.Recommendations = append(stat.Recommendations,
			fmt.Sprintf("Running on FARGATE: all %d restarts required cold pulls without shared host layer cache.", stoppedInWindow))
	}
	if strings.Contains(strings.ToLower(dominantReason), "health check") {
		stat.Recommendations = append(stat.Recommendations,
			"Tasks failed ALB/ELB health checks. Increase health check grace period or verify endpoint status.")
	}

	return stat
}
