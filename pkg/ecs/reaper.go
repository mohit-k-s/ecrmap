package ecs

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"

	"ecrmap/pkg/model"
)

const (
	TagOriginalDesired = "ecrmap:original-desired-count"
	TagReapReason       = "ecrmap:reap-reason"
	TagReapedAt         = "ecrmap:reaped-at"
	TagRestoredAt       = "ecrmap:restored-at"
)

// FindReapCandidates scans a cluster for crash-looping services that should be scaled to zero.
func (c *Crawler) FindReapCandidates(ctx context.Context, clusterName string, window time.Duration, threshold int) ([]model.ReapCandidate, error) {
	if window <= 0 {
		window = 24 * time.Hour
	}
	if threshold <= 0 {
		threshold = 15
	}

	// 1. Get cluster ARN
	clusterArns, err := c.listClusters(ctx, clusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to find cluster %s: %w", clusterName, err)
	}
	if len(clusterArns) == 0 {
		return nil, fmt.Errorf("cluster '%s' not found in region '%s'", clusterName, c.region)
	}
	clusterArn := clusterArns[0]

	// 2. Get all services in cluster
	services, err := c.getClusterServices(ctx, clusterArn)
	if err != nil {
		return nil, fmt.Errorf("failed to list services for cluster %s: %w", clusterName, err)
	}

	var candidates []model.ReapCandidate
	since := time.Now().Add(-window)

	for _, svc := range services {
		desired := svc.DesiredCount
		serviceName := aws.ToString(svc.ServiceName)
		serviceArn := aws.ToString(svc.ServiceArn)

		// Create mock deployment object for churn inspector
		dep := model.ServiceDeployment{
			Region:       c.region,
			ClusterName:  clusterName,
			ClusterArn:   clusterArn,
			ServiceName:  serviceName,
			ServiceArn:   serviceArn,
			DesiredCount: desired,
			RunningCount: svc.RunningCount,
		}

		stat := c.inspectServiceChurn(ctx, c.client, clusterArn, clusterName, serviceName, dep, since, window)

		shouldReap := false
		reapReason := ""

		// Candidate if currently desired > 0 and stoppedCount >= threshold
		if desired > 0 {
			if stat.StoppedCount >= threshold {
				shouldReap = true
				if stat.PrimaryExitCode != 0 {
					reapReason = fmt.Sprintf("Crash-looping: %d restarts in %s (exit code %d)",
						stat.StoppedCount, window.String(), stat.PrimaryExitCode)
				} else if stat.PrimaryReason != "" {
					reapReason = fmt.Sprintf("High churn: %d restarts in %s (%s)",
						stat.StoppedCount, window.String(), stat.PrimaryReason)
				} else {
					reapReason = fmt.Sprintf("High churn: %d restarts in %s",
						stat.StoppedCount, window.String())
				}
			}
		}

		candidates = append(candidates, model.ReapCandidate{
			Region:          c.region,
			ClusterName:     clusterName,
			ClusterArn:      clusterArn,
			ServiceName:     serviceName,
			ServiceArn:      serviceArn,
			LaunchType:      stat.LaunchType,
			CurrentDesired:  desired,
			CurrentRunning:  svc.RunningCount,
			StoppedCount:    stat.StoppedCount,
			PrimaryReason:   stat.PrimaryReason,
			PrimaryExitCode: stat.PrimaryExitCode,
			ShouldReap:      shouldReap,
			ReapReason:      reapReason,
		})
	}

	return candidates, nil
}

// ExecuteReap scales an offending service to desiredCount = 0 and records audit tags.
func (c *Crawler) ExecuteReap(ctx context.Context, candidate model.ReapCandidate) error {
	// 1. Tag service with original count and reason
	tags := []ecstypes.Tag{
		{
			Key:   aws.String(TagOriginalDesired),
			Value: aws.String(strconv.FormatInt(int64(candidate.CurrentDesired), 10)),
		},
		{
			Key:   aws.String(TagReapReason),
			Value: aws.String(candidate.ReapReason),
		},
		{
			Key:   aws.String(TagReapedAt),
			Value: aws.String(time.Now().UTC().Format(time.RFC3339)),
		},
	}

	_, err := c.client.TagResource(ctx, &ecs.TagResourceInput{
		ResourceArn: aws.String(candidate.ServiceArn),
		Tags:        tags,
	})
	if err != nil {
		// Log tag error but continue to scale down
		fmt.Printf("Warning: failed to tag service %s: %v\n", candidate.ServiceName, err)
	}

	// 2. Scale service to 0
	_, err = c.client.UpdateService(ctx, &ecs.UpdateServiceInput{
		Cluster:      aws.String(candidate.ClusterArn),
		Service:      aws.String(candidate.ServiceName),
		DesiredCount: aws.Int32(0),
	})
	if err != nil {
		return fmt.Errorf("failed to scale down service %s to 0: %w", candidate.ServiceName, err)
	}

	return nil
}

// RestoreService restores a previously reaped service back to its original desired count.
func (c *Crawler) RestoreService(ctx context.Context, clusterName, serviceName string) (int32, error) {
	// 1. Describe service to get ARN
	resp, err := c.client.DescribeServices(ctx, &ecs.DescribeServicesInput{
		Cluster:  aws.String(clusterName),
		Services: []string{serviceName},
	})
	if err != nil || len(resp.Services) == 0 {
		return 0, fmt.Errorf("service %s not found in cluster %s: %w", serviceName, clusterName, err)
	}

	svc := resp.Services[0]
	serviceArn := aws.ToString(svc.ServiceArn)

	// 2. Fetch tags to retrieve original desired count
	tagResp, err := c.client.ListTagsForResource(ctx, &ecs.ListTagsForResourceInput{
		ResourceArn: aws.String(serviceArn),
	})

	targetCount := int32(1) // Default if tag not found
	if err == nil {
		for _, t := range tagResp.Tags {
			if aws.ToString(t.Key) == TagOriginalDesired {
				if parsed, parseErr := strconv.ParseInt(aws.ToString(t.Value), 10, 32); parseErr == nil && parsed > 0 {
					targetCount = int32(parsed)
				}
				break
			}
		}
	}

	// 3. Update service desired count
	_, err = c.client.UpdateService(ctx, &ecs.UpdateServiceInput{
		Cluster:      aws.String(clusterName),
		Service:      aws.String(serviceName),
		DesiredCount: aws.Int32(targetCount),
	})
	if err != nil {
		return 0, fmt.Errorf("failed to update service %s desiredCount to %d: %w", serviceName, targetCount, err)
	}

	// 4. Update restore tag
	_, _ = c.client.TagResource(ctx, &ecs.TagResourceInput{
		ResourceArn: aws.String(serviceArn),
		Tags: []ecstypes.Tag{
			{
				Key:   aws.String(TagRestoredAt),
				Value: aws.String(time.Now().UTC().Format(time.RFC3339)),
			},
		},
	})

	return targetCount, nil
}

// IsProductionCluster checks if a cluster name indicates a production environment.
func IsProductionCluster(clusterName string) bool {
	lower := strings.ToLower(clusterName)
	return strings.Contains(lower, "prod") || strings.Contains(lower, "production")
}
