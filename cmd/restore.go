package cmd

import (
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/spf13/cobra"

	"ecrmap/pkg/ecs"
	"ecrmap/pkg/output"
)

var (
	flagRestoreCluster string
)

var restoreCmd = &cobra.Command{
	Use:   "restore <service-name>",
	Short: "Restore a previously reaped ECS service back to its original desired count",
	Long: `Restores a service that was auto-scaled to 0 by ecrmap reap.
Reads the original desired count stored in the 'ecrmap:original-desired-count' tag.`,
	Example: `  # Restore a service in a dev cluster:
  ecrmap restore payment-worker --cluster dev-cluster`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		serviceName := args[0]
		targetCluster := flagRestoreCluster
		if targetCluster == "" {
			targetCluster = flagCluster
		}
		if targetCluster == "" {
			return fmt.Errorf("cluster is required for restore command. Use --cluster <cluster-name>")
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

		restoredCount, err := crawler.RestoreService(ctx, targetCluster, serviceName)
		if err != nil {
			return err
		}

		return output.PrintRestoreResult(serviceName, targetCluster, restoredCount, flagJSON)
	},
}

func init() {
	restoreCmd.Flags().StringVarP(&flagRestoreCluster, "cluster", "c", "", "ECS cluster containing the service (required)")
	rootCmd.AddCommand(restoreCmd)
}
