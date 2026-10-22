package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/spf13/cobra"

	"ecrmap/pkg/ecs"
	"ecrmap/pkg/model"
	"ecrmap/pkg/output"
)

var (
	flagReapCluster   string
	flagReapThreshold int
	flagReapWindow    time.Duration
	flagReapExecute   bool
	flagReapAllowProd bool
)

var reapCmd = &cobra.Command{
	Use:   "reap",
	Short: "Safely scale down crash-looping ECS services to zero in dev/staging to save costs",
	Long: `Identify and auto-scale runaway crash-looping services to desiredCount = 0.

By default, runs in safe DRY-RUN mode.
Refuses to run against clusters containing 'prod' unless --allow-prod is explicitly passed.
Writes audit tags (ecrmap:original-desired-count, ecrmap:reap-reason) to allow instant restoration.`,
	Example: `  # Safe dry-run inspection of a dev cluster:
  ecrmap reap --cluster dev-cluster

  # Actually scale down crash-looping services in dev:
  ecrmap reap --cluster dev-cluster --execute

  # Adjust threshold (e.g. >= 20 restarts over 12 hours):
  ecrmap reap --cluster dev-cluster --threshold 20 --window 12h --execute`,
	RunE: func(cmd *cobra.Command, args []string) error {
		targetCluster := flagReapCluster
		if targetCluster == "" {
			targetCluster = flagCluster
		}
		if targetCluster == "" {
			return fmt.Errorf("cluster is required for reap command. Use --cluster <cluster-name>")
		}

		// Production safeguard
		if ecs.IsProductionCluster(targetCluster) && !flagReapAllowProd {
			return fmt.Errorf("safety lockout: refusing to reap services in production cluster '%s'\nIf you intentionally want to run this in production, pass --allow-prod", targetCluster)
		}

		ctx := cmd.Context()

		var optFns []func(*config.LoadOptions) error
		if flagProfile != "" {
			optFns = append(optFns, config.WithSharedConfigProfile(flagProfile))
		}
		if flagRegion != "" {
			optFns = append(optFns, config.WithRegion(flagRegion))
		}

		cfg, err := config.LoadDefaultConfig(ctx, optFns...)
		if err != nil {
			return fmt.Errorf("failed to load AWS config: %w", err)
		}

		effectiveRegion := flagRegion
		if effectiveRegion == "" {
			effectiveRegion = os.Getenv("AWS_REGION")
			if effectiveRegion == "" {
				effectiveRegion = cfg.Region
			}
			if effectiveRegion == "" {
				effectiveRegion = "us-east-1"
			}
		}

		crawler := ecs.NewCrawlerWithConfig(cfg, effectiveRegion)

		// 1. Find candidates
		candidates, err := crawler.FindReapCandidates(ctx, targetCluster, flagReapWindow, flagReapThreshold)
		if err != nil {
			return fmt.Errorf("failed to scan cluster %s: %w", targetCluster, err)
		}

		savedTasks := int32(0)
		reapedCount := 0

		// 2. Execute if requested
		for i := range candidates {
			c := &candidates[i]
			if c.ShouldReap {
				savedTasks += c.CurrentDesired
				if flagReapExecute {
					if err := crawler.ExecuteReap(ctx, *c); err != nil {
						fmt.Printf("Error scaling down service %s: %v\n", c.ServiceName, err)
						continue
					}
					reapedCount++
				}
			}
		}

		report := &model.ReapReport{
			ClusterName: targetCluster,
			Region:      effectiveRegion,
			TimeWindow:  flagReapWindow,
			Threshold:   flagReapThreshold,
			DryRun:      !flagReapExecute,
			Candidates:  candidates,
			ReapedCount: reapedCount,
			SavedTasks:  savedTasks,
		}

		return output.PrintReapReport(report, flagJSON)
	},
}

func init() {
	reapCmd.Flags().StringVarP(&flagReapCluster, "cluster", "c", "", "ECS cluster to inspect and reap (required)")
	reapCmd.Flags().IntVarP(&flagReapThreshold, "threshold", "t", 15, "Minimum stopped task restarts to qualify for scale-down")
	reapCmd.Flags().DurationVarP(&flagReapWindow, "window", "w", 24*time.Hour, "Time window to inspect stopped tasks (e.g. 6h, 12h, 24h)")
	reapCmd.Flags().BoolVar(&flagReapExecute, "execute", false, "Actually apply scale-down to 0 (default is safe dry-run)")
	reapCmd.Flags().BoolVar(&flagReapAllowProd, "allow-prod", false, "Explicit override required to run against clusters containing 'prod'")
	rootCmd.AddCommand(reapCmd)
}
