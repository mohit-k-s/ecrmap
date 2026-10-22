package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/spf13/cobra"

	"ecrmap/pkg/cache"
	"ecrmap/pkg/ecs"
	"ecrmap/pkg/model"
)

var (
	flagProfile    string
	flagRegion     string
	flagAllRegions bool
	flagCluster    string
	flagFresh      bool
	flagCacheTTL   time.Duration
	flagJSON       bool
)

var rootCmd = &cobra.Command{
	Use:   "ecrmap",
	Short: "ecrmap - Instant reverse mapping from ECR images to live ECS services",
	Long: `ecrmap enables DevOps and SRE teams to instantly track where container images 
are actively running across AWS ECS clusters and services.

Evaluate ground-truth state in seconds with zero server infrastructure.
Supports querying single, multiple, or all AWS regions in parallel.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&flagProfile, "profile", "p", "", "AWS profile to use")
	rootCmd.PersistentFlags().StringVarP(&flagRegion, "region", "r", "", "AWS region(s) to query (comma-separated, e.g. 'us-east-1,us-west-2')")
	rootCmd.PersistentFlags().BoolVar(&flagAllRegions, "all-regions", false, "Discover and query all enabled AWS regions in parallel")
	rootCmd.PersistentFlags().StringVarP(&flagCluster, "cluster", "c", "", "Filter to a specific ECS cluster name or ARN")
	rootCmd.PersistentFlags().BoolVar(&flagFresh, "fresh", false, "Bypass local cache and force live AWS discovery")
	rootCmd.PersistentFlags().DurationVar(&flagCacheTTL, "cache-ttl", 5*time.Minute, "Cache duration for live discoveries (e.g. 5m, 10m)")
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "Output results in JSON format")
}

// getSnapshot retrieves ECS service deployment state across one or more regions.
func getSnapshot(ctx context.Context) (*model.Snapshot, error) {
	c, err := cache.New()
	if err != nil {
		return nil, fmt.Errorf("cache error: %w", err)
	}

	// 1. Load base AWS Config
	var optFns []func(*config.LoadOptions) error
	if flagProfile != "" {
		optFns = append(optFns, config.WithSharedConfigProfile(flagProfile))
	}

	cfg, err := config.LoadDefaultConfig(ctx, optFns...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// 2. Resolve target regions
	targetRegions := resolveRegions(ctx, cfg)
	sort.Strings(targetRegions)

	cacheKeyRegion := strings.Join(targetRegions, "-")
	if len(targetRegions) > 3 {
		cacheKeyRegion = fmt.Sprintf("multiregion_%d_regions", len(targetRegions))
	}

	// 3. Try cache if not forced fresh
	if !flagFresh {
		cached, found, err := c.Get(flagProfile, cacheKeyRegion, flagCacheTTL)
		if err == nil && found && cached != nil {
			return cached, nil
		}
	}

	// 4. Run concurrent multi-region ECS discovery
	opts := ecs.DiscoverOptions{
		ClusterFilter: flagCluster,
		MaxWorkers:    12,
	}

	snapshot, err := ecs.DiscoverMultiRegion(ctx, cfg, targetRegions, opts)
	if err != nil {
		return nil, err
	}

	// 5. Save to cache
	_ = c.Put(flagProfile, cacheKeyRegion, snapshot)

	return snapshot, nil
}

func resolveRegions(ctx context.Context, cfg aws.Config) []string {
	if flagAllRegions {
		ec2Client := ec2.NewFromConfig(cfg)
		resp, err := ec2Client.DescribeRegions(ctx, &ec2.DescribeRegionsInput{
			AllRegions: aws.Bool(false), // only enabled regions
		})
		if err == nil && len(resp.Regions) > 0 {
			var regions []string
			for _, r := range resp.Regions {
				if r.RegionName != nil {
					regions = append(regions, *r.RegionName)
				}
			}
			return regions
		}
		// Fallback to top common AWS regions if DescribeRegions fails
		return []string{
			"us-east-1", "us-east-2", "us-west-1", "us-west-2",
			"eu-west-1", "eu-central-1", "ap-south-1", "ap-southeast-1", "ap-northeast-1",
		}
	}

	if flagRegion != "" {
		var regions []string
		for _, part := range strings.Split(flagRegion, ",") {
			p := strings.TrimSpace(part)
			if p != "" {
				regions = append(regions, p)
			}
		}
		if len(regions) > 0 {
			return regions
		}
	}

	// Check environment variables or default config
	envRegion := os.Getenv("AWS_REGION")
	if envRegion == "" {
		envRegion = os.Getenv("AWS_DEFAULT_REGION")
	}
	if envRegion == "" {
		envRegion = cfg.Region
	}
	if envRegion == "" {
		envRegion = "us-east-1"
	}

	return []string{envRegion}
}
