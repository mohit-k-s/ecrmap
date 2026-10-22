package output

import (
	"fmt"
	"os"
	"text/tabwriter"

	"ecrmap/pkg/model"
)

// PrintChurnReport renders the task churn and ECR pull analysis report.
func PrintChurnReport(report *model.RepoChurnReport, jsonFormat bool) error {
	if jsonFormat {
		return PrintJSON(report)
	}

	if report.AnalyzedServices == 0 {
		fmt.Printf("\n%sNo active services found running images from repository '%s'%s\n\n",
			colorYellow, report.Repository, colorReset)
		return nil
	}

	fmt.Printf("\n%sECR Pull Churn Analysis: %s%s%s\n", colorBold, colorCyan, report.Repository, colorReset)
	fmt.Printf("Analyzed %d active service(s) | Window: last %s | Total Estimated Pulls: %s%d%s\n\n",
		report.AnalyzedServices, report.Window.String(), colorBold, report.TotalEstimatedPulls, colorReset)

	fmt.Printf("%sCULPRIT RANKING (SERVICES DRIVING HIGHEST PULL CHURN):%s\n", colorBold, colorReset)

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 3, ' ', 0)
	fmt.Fprintf(w, "%sRANK\tSERVICE\tCLUSTER\tLAUNCH\tRUNNING\tSTOPPED\tEST. PULLS\tRATE\tHEALTH STATUS%s\n", colorBold, colorReset)

	for i, s := range report.Services {
		statusColor := colorGreen
		statusIcon := "🟢"
		switch s.HealthStatus {
		case "CRASH-LOOPING":
			statusColor = colorRed
			statusIcon = "🔴"
		case "HIGH_CHURN":
			statusColor = colorYellow
			statusIcon = "🟡"
		}

		rateStr := fmt.Sprintf("%.1f/hr", s.PullsPerHour)

		fmt.Fprintf(w, "#%d\t%s\t%s\t%s\t%d\t%d\t~%d\t%s\t%s %s%s%s\n",
			i+1,
			s.ServiceName,
			s.ClusterName,
			s.LaunchType,
			s.RunningCount,
			s.StoppedCount,
			s.EstimatedPulls,
			rateStr,
			statusIcon, statusColor, s.HealthStatus, colorReset,
		)
	}
	w.Flush()
	fmt.Println()

	// Diagnostic breakdown of problem services
	hasIssues := false
	for _, s := range report.Services {
		if s.HealthStatus != "HEALTHY" || len(s.Recommendations) > 0 {
			if !hasIssues {
				fmt.Printf("%sDIAGNOSTIC INSIGHTS & ROOT CAUSES:%s\n", colorBold, colorReset)
				hasIssues = true
			}

			fmt.Printf("\n  %s⚠️  Service '%s' in '%s'%s (Estimated pulls: ~%d, Stopped: %d):\n",
				colorYellow, s.ServiceName, s.ClusterName, colorReset, s.EstimatedPulls, s.StoppedCount)

			if s.PrimaryReason != "" {
				fmt.Printf("     • Dominant Stop Reason: %s\n", s.PrimaryReason)
			}

			for _, rec := range s.Recommendations {
				fmt.Printf("     • %s%s%s\n", colorCyan, rec, colorReset)
			}
		}
	}

	if hasIssues {
		fmt.Println()
	} else {
		fmt.Printf("%sAll active services appear stable with no crash-looping or excessive pull churn.%s\n\n",
			colorGreen, colorReset)
	}

	return nil
}
