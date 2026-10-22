package output

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"ecrmap/pkg/model"
)

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorRed    = "\033[31m"
)

// PrintJSON prints any value as formatted JSON.
func PrintJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// WhereResult holds the search matches for an image query.
type WhereResult struct {
	Query       string                   `json:"query"`
	Deployments []MatchedDeploymentEntry `json:"deployments"`
}

// MatchedDeploymentEntry holds details of an image match in an ECS service.
type MatchedDeploymentEntry struct {
	Region         string `json:"region"`
	Cluster        string `json:"cluster"`
	Service        string `json:"service"`
	Status         string `json:"status"`
	RunningCount   int32  `json:"running_count"`
	DesiredCount   int32  `json:"desired_count"`
	TaskDefinition string `json:"task_definition"`
	Container      string `json:"container"`
	ImageRaw       string `json:"image_raw"`
	Repository     string `json:"repository"`
	Tag            string `json:"tag,omitempty"`
	Digest         string `json:"digest,omitempty"`
	IsECR          bool   `json:"is_ecr"`
}

// PrintWhereResults displays matches for an image lookup.
func PrintWhereResults(results WhereResult, jsonFormat bool) error {
	if jsonFormat {
		return PrintJSON(results)
	}

	if len(results.Deployments) == 0 {
		fmt.Printf("%sNo active ECS services found matching %s%s%s%s\n", colorYellow, colorBold, results.Query, colorReset, colorYellow)
		return nil
	}

	fmt.Printf("\n%sFound %d active container instance(s) matching %s%s%s:\n\n",
		colorBold, len(results.Deployments), colorCyan, results.Query, colorReset)

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 3, ' ', 0)
	fmt.Fprintf(w, "%sREGION\tCLUSTER\tSERVICE\tSTATUS\tCOUNT\tTASK DEF\tCONTAINER\tIMAGE REF%s\n", colorBold, colorReset)

	for _, d := range results.Deployments {
		countStr := fmt.Sprintf("%d/%d", d.RunningCount, d.DesiredCount)
		imageRef := d.Tag
		if imageRef == "" && d.Digest != "" {
			imageRef = shortDigest(d.Digest)
		} else if imageRef != "" && d.Digest != "" {
			imageRef = fmt.Sprintf("%s (%s)", d.Tag, shortDigest(d.Digest))
		}
		if imageRef == "" {
			imageRef = d.ImageRaw
		}

		statusColor := colorGreen
		if d.RunningCount == 0 && d.DesiredCount > 0 {
			statusColor = colorRed
		} else if d.RunningCount < d.DesiredCount {
			statusColor = colorYellow
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s%s%s\t%s\t%s\t%s\t%s%s%s\n",
			d.Region,
			d.Cluster,
			d.Service,
			statusColor, d.Status, colorReset,
			countStr,
			d.TaskDefinition,
			d.Container,
			colorCyan, imageRef, colorReset,
		)
	}
	w.Flush()
	fmt.Println()
	return nil
}

// PrintListResults displays all discovered services and their images.
func PrintListResults(deployments []model.ServiceDeployment, jsonFormat bool) error {
	if jsonFormat {
		return PrintJSON(deployments)
	}

	if len(deployments) == 0 {
		fmt.Println("No active ECS services found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 3, ' ', 0)
	fmt.Fprintf(w, "%sREGION\tCLUSTER\tSERVICE\tDESIRED/RUNNING\tTASK DEFINITION\tCONTAINER\tIMAGE%s\n", colorBold, colorReset)

	for _, d := range deployments {
		counts := fmt.Sprintf("%d/%d", d.DesiredCount, d.RunningCount)
		taskDefShort := d.TaskFamily + ":" + d.TaskRevision

		for i, c := range d.Containers {
			regionCol := d.Region
			clusterCol := d.ClusterName
			svcCol := d.ServiceName
			countCol := counts
			tdCol := taskDefShort

			// For secondary containers, keep parent columns clean
			if i > 0 {
				regionCol = ""
				clusterCol = ""
				svcCol = ""
				countCol = ""
				tdCol = ""
			}

			tagOrDigest := c.Image.Tag
			if tagOrDigest == "" {
				tagOrDigest = shortDigest(c.Image.Digest)
			}
			repoDisplay := c.Image.Repository + ":" + tagOrDigest

			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				regionCol, clusterCol, svcCol, countCol, tdCol, c.ContainerName, repoDisplay)
		}
	}
	w.Flush()
	fmt.Println()
	return nil
}

// AuditResult models image usage per repository.
type AuditResult struct {
	Repository     string                   `json:"repository"`
	TotalServices  int                      `json:"total_services"`
	ActiveTags     map[string][]string      `json:"active_tags"` // tag -> list of "cluster/service"
	AllDeployments []MatchedDeploymentEntry `json:"deployments"`
}

// PrintAuditResults displays repository-level audit information.
func PrintAuditResults(res AuditResult, jsonFormat bool) error {
	if jsonFormat {
		return PrintJSON(res)
	}

	fmt.Printf("\n%sRepository Audit: %s%s%s\n", colorBold, colorCyan, res.Repository, colorReset)
	fmt.Printf("Active in %d service(s) across %d unique tag(s)\n\n", res.TotalServices, len(res.ActiveTags))

	if len(res.ActiveTags) == 0 {
		fmt.Printf("%sNo active services currently deployed with images from repository '%s'%s\n\n",
			colorYellow, res.Repository, colorReset)
		return nil
	}

	fmt.Printf("%sTag / Version Distribution:%s\n", colorBold, colorReset)
	for tag, svcs := range res.ActiveTags {
		fmt.Printf("  • %s%-20s%s → %s (%d service(s))\n",
			colorGreen, tag, colorReset, strings.Join(svcs, ", "), len(svcs))
	}
	fmt.Println()

	// Detailed table: SERVICE, REGION, CLUSTER, IMAGE VERSION, TASK DEF, STATUS, COUNT
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 3, ' ', 0)
	fmt.Fprintf(w, "%sSERVICE\tREGION\tCLUSTER\tIMAGE VERSION\tTASK DEF\tSTATUS\tCOUNT%s\n", colorBold, colorReset)

	for _, d := range res.AllDeployments {
		versionStr := d.Tag
		if versionStr == "" && d.Digest != "" {
			versionStr = shortDigest(d.Digest)
		} else if versionStr != "" && d.Digest != "" {
			versionStr = fmt.Sprintf("%s (%s)", d.Tag, shortDigest(d.Digest))
		}

		statusColor := colorGreen
		if d.RunningCount == 0 && d.DesiredCount > 0 {
			statusColor = colorRed
		} else if d.RunningCount < d.DesiredCount {
			statusColor = colorYellow
		}

		counts := fmt.Sprintf("%d/%d", d.RunningCount, d.DesiredCount)

		fmt.Fprintf(w, "%s\t%s\t%s\t%s%s%s\t%s\t%s%s%s\t%s\n",
			d.Service,
			d.Region,
			d.Cluster,
			colorCyan, versionStr, colorReset,
			d.TaskDefinition,
			statusColor, d.Status, colorReset,
			counts,
		)
	}
	w.Flush()
	fmt.Println()
	return nil
}

func shortDigest(digest string) string {
	if len(digest) > 19 {
		// sha256:123456789012...
		return digest[:19] + "…"
	}
	return digest
}
