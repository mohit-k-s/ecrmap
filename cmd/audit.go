package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"ecrmap/pkg/output"
)

var auditCmd = &cobra.Command{
	Use:     "audit <repository-name>",
	Aliases: []string{"repo", "repository"},
	Short:   "Find which services are using an ECR repository and at what image version",
	Example: `  # Look up an ECR repository:
  ecrmap repo payment-service

  # Or using audit:
  ecrmap audit payment-service

  # Nested repository path:
  ecrmap repo org/payments/api`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		repoQuery := strings.TrimSpace(args[0])
		ctx := cmd.Context()

		snapshot, err := getSnapshot(ctx)
		if err != nil {
			return fmt.Errorf("failed to discover ECS workloads: %w", err)
		}

		activeTags := make(map[string][]string)
		var matches []output.MatchedDeploymentEntry
		totalServices := 0

		for _, dep := range snapshot.Deployments {
			serviceMatched := false
			for _, c := range dep.Containers {
				// Match repository name (exact or short suffix)
				isMatch := strings.EqualFold(c.Image.Repository, repoQuery)
				if !isMatch {
					parts := strings.Split(c.Image.Repository, "/")
					if len(parts) > 1 && strings.EqualFold(parts[len(parts)-1], repoQuery) {
						isMatch = true
					}
				}

				if isMatch {
					serviceMatched = true
					tagKey := c.Image.Tag
					if tagKey == "" {
						tagKey = c.Image.Digest
					}

					svcIdentifier := fmt.Sprintf("%s/%s", dep.ClusterName, dep.ServiceName)
					activeTags[tagKey] = append(activeTags[tagKey], svcIdentifier)

					taskDefShort := dep.TaskFamily + ":" + dep.TaskRevision
					matches = append(matches, output.MatchedDeploymentEntry{
						Region:         dep.Region,
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
			if serviceMatched {
				totalServices++
			}
		}

		result := output.AuditResult{
			Repository:     repoQuery,
			TotalServices:  totalServices,
			ActiveTags:     activeTags,
			AllDeployments: matches,
		}

		return output.PrintAuditResults(result, flagJSON)
	},
}

func init() {
	rootCmd.AddCommand(auditCmd)
}
