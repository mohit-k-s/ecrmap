package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"ecrmap/pkg/model"
	"ecrmap/pkg/output"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all active ECS services and their deployed container images",
	Example: `  # List all services across all clusters:
  ecrmap list

  # Filter by a specific cluster:
  ecrmap list --cluster prod-cluster

  # Output as JSON:
  ecrmap list --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		snapshot, err := getSnapshot(ctx)
		if err != nil {
			return fmt.Errorf("failed to discover ECS workloads: %w", err)
		}

		// If a cluster filter was provided on an already cached snapshot, filter it here
		deployments := snapshot.Deployments
		if flagCluster != "" {
			var filtered []model.ServiceDeployment
			for _, d := range deployments {
				if d.ClusterName == flagCluster || d.ClusterArn == flagCluster {
					filtered = append(filtered, d)
				}
			}
			deployments = filtered
		}

		return output.PrintListResults(deployments, flagJSON)
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
