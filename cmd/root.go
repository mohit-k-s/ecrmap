package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	awsecs "github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/spf13/cobra"

	"ecrmap/pkg/cache"
	"ecrmap/pkg/ecs"
	"ecrmap/pkg/model"
)

var (
	flagProfile  string
	flagRegion   string
	flagCluster  string
	flagFresh    bool
	flagCacheTTL time.Duration
	flagJSON     bool
)

var rootCmd = &cobra.Command{
	Use:   "ecrmap",
	Short: "ecrmap - Instant reverse mapping from ECR images to live ECS services",
	Long: `ecrmap enables DevOps and SRE teams to instantly track where container images 
are actively running across AWS ECS clusters and services.

Evaluate ground-truth state in seconds with zero server infrastructure.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&flagProfile, "profile", "p", "", "AWS profile to use")
	rootCmd.PersistentFlags().StringVarP(&flagRegion, "region", "r", "", "AWS region (overrides default/profile)")
	rootCmd.PersistentFlags().StringVarP(&flagCluster, "cluster", "c", "", "Filter to a specific ECS cluster name or ARN")
	rootCmd.PersistentFlags().BoolVar(&flagFresh, "fresh", false, "Bypass local cache and force live AWS discovery")
	rootCmd.PersistentFlags().DurationVar(&flagCacheTTL, "cache-ttl", 5*time.Minute, "Cache duration for live discoveries (e.g. 5m, 10m)")
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "Output results in JSON format")
}

// getSnapshot retrieves ECS service deployment state from cache or live crawl.
func getSnapshot(ctx context.Context) (*model.Snapshot, error) {
	c, err := cache.New()
	if err != nil {
		return nil, fmt.Errorf("cache error: %w", err)
	}

	effectiveRegion := flagRegion
	if effectiveRegion == "" {
		effectiveRegion = os.Getenv("AWS_REGION")
		if effectiveRegion == "" {
			effectiveRegion = os.Getenv("AWS_DEFAULT_REGION")
		}
	}

	// 1. Try cache if not forced fresh
	if !flagFresh {
		cached, found, err := c.Get(flagProfile, effectiveRegion, flagCacheTTL)
		if err == nil && found && cached != nil {
			return cached, nil
		}
	}

	// 2. Load AWS Config
	var optFns []func(*config.LoadOptions) error
	if flagProfile != "" {
		optFns = append(optFns, config.WithSharedConfigProfile(flagProfile))
	}
	if flagRegion != "" {
		optFns = append(optFns, config.WithRegion(flagRegion))
	}

	cfg, err := config.LoadDefaultConfig(ctx, optFns...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	if effectiveRegion == "" {
		effectiveRegion = cfg.Region
	}

	// 3. Run ECS discovery
	client := awsecs.NewFromConfig(cfg)
	crawler := ecs.NewCrawler(client, effectiveRegion)

	opts := ecs.DiscoverOptions{
		ClusterFilter: flagCluster,
		MaxWorkers:    12,
	}

	snapshot, err := crawler.Discover(ctx, opts)
	if err != nil {
		return nil, err
	}

	// 4. Save to cache
	_ = c.Put(flagProfile, effectiveRegion, snapshot)

	return snapshot, nil
}
