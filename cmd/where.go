package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"ecrmap/pkg/output"
	"ecrmap/pkg/parser"
)

var whereCmd = &cobra.Command{
	Use:   "where <image-name | repo:tag | digest>",
	Short: "Find which ECS clusters and services are actively running an image",
	Example: `  # Find by repository and tag:
  ecrmap where payment-api:v2.1.0

  # Find by repository only (shows all running tags of that repo):
  ecrmap where payment-api

  # Find by image digest:
  ecrmap where sha256:4f53cda18c2b...

  # Force fresh live scan without cache:
  ecrmap where payment-api --fresh`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := args[0]
		ctx := cmd.Context()

		snapshot, err := getSnapshot(ctx)
		if err != nil {
			return fmt.Errorf("failed to discover ECS workloads: %w", err)
		}

		var matches []output.MatchedDeploymentEntry

		for _, dep := range snapshot.Deployments {
			for _, c := range dep.Containers {
				if parser.Matches(c.Image, query) {
					taskDefShort := dep.TaskFamily + ":" + dep.TaskRevision
					matches = append(matches, output.MatchedDeploymentEntry{
						Cluster:        dep.ClusterName,
						Service:        dep.ServiceName,
						Status:         dep.Status,
						RunningCount:   dep.RunningCount,
						DesiredCount:   dep.DesiredCount,
						TaskDefinition: taskDefShort,
						Container:      c.ContainerName,
						ImageRaw:       c.Image.RawURI,
						Repository:     c.Image.Repository,
						Tag:            c.Image.Tag,
						Digest:         c.Image.Digest,
						IsECR:          c.Image.IsECR,
					})
				}
			}
		}

		result := output.WhereResult{
			Query:       query,
			Deployments: matches,
		}

		return output.PrintWhereResults(result, flagJSON)
	},
}

func init() {
	rootCmd.AddCommand(whereCmd)
}
