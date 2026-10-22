package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	awsecs "github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/spf13/cobra"

	"ecrmap/pkg/ecs"
	"ecrmap/pkg/output"
)

var (
	flagWindow time.Duration
)

var pullsCmd = &cobra.Command{
	Use:     "pulls <repository-name>",
	Aliases: []string{"churn", "flapping"},
	Short:   "Detect which ECS services are causing excessive ECR pull counts and task churn",
	Long: `Analyze an ECR repository to identify services causing high pull counts.

Detects silent crash-loops (OOM / exit code 1), uncached Fargate cold-pull thrashing,
and rapid restart churn across your ECS clusters.`,
	Example: `  # Analyze pull churn over the default 24h window:
  ecrmap pulls payment-service

  # Analyze over the last 6 hours:
  ecrmap pulls payment-service --window 6h

  # Output as JSON:
  ecrmap pulls payment-service --json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		repoName := args[0]
		ctx := cmd.Context()

		// 1. Get snapshot to locate candidate services
		snapshot, err := getSnapshot(ctx)
		if err != nil {
			return fmt.Errorf("failed to discover services: %w", err)
		}

		// 2. Load AWS Config for live task inspection
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
		}

		client := awsecs.NewFromConfig(cfg)
		crawler := ecs.NewCrawler(client, effectiveRegion)

		// 3. Run churn analysis
		report, err := crawler.AnalyzeRepoChurn(ctx, snapshot, repoName, flagWindow)
		if err != nil {
			return fmt.Errorf("failed to analyze churn: %w", err)
		}

		return output.PrintChurnReport(report, flagJSON)
	},
}

func init() {
	pullsCmd.Flags().DurationVarP(&flagWindow, "window", "w", 24*time.Hour, "Time window to inspect stopped tasks (e.g. 1h, 6h, 24h)")
	rootCmd.AddCommand(pullsCmd)
}
