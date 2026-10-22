package output

import (
	"fmt"
	"os"
	"text/tabwriter"

	"ecrmap/pkg/model"
)

// PrintReapReport formats and prints the results of a reap candidate evaluation or execution.
func PrintReapReport(report *model.ReapReport, jsonFormat bool) error {
	if jsonFormat {
		return PrintJSON(report)
	}

	modeStr := fmt.Sprintf("%s🔍 DRY-RUN (Pass --execute to apply changes)%s", colorYellow, colorReset)
	if !report.DryRun {
		modeStr = fmt.Sprintf("%s⚡ EXECUTED (Services scaled to 0)%s", colorGreen, colorReset)
	}

	fmt.Printf("\n%sECS Service Reaper%s | Cluster: %s%s%s (Region: %s)\n",
		colorBold, colorReset, colorCyan, report.ClusterName, colorReset, report.Region)
	fmt.Printf("Window: last %s | Threshold: >= %d restarts | Mode: %s\n\n",
		report.TimeWindow.String(), report.Threshold, modeStr)

	candidateCount := 0
	for _, c := range report.Candidates {
		if c.ShouldReap {
			candidateCount++
		}
	}

	if candidateCount == 0 {
		fmt.Printf("%sNo crash-looping services exceeded the threshold (%d restarts). Cluster is healthy!%s\n\n",
			colorGreen, report.Threshold, colorReset)
		return nil
	}

	fmt.Printf("%sCANDIDATE SERVICES FOR SCALE-DOWN (CRASH-LOOPING):%s\n", colorBold, colorReset)

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 3, ' ', 0)
	fmt.Fprintf(w, "%sSERVICE\tLAUNCH\tDESIRED\tSTOPPED\tPRIMARY CAUSE\tACTION%s\n", colorBold, colorReset)

	for _, c := range report.Candidates {
		if !c.ShouldReap {
			continue
		}

		actionStr := fmt.Sprintf("%sWOULD SCALE TO 0%s", colorYellow, colorReset)
		if !report.DryRun {
			actionStr = fmt.Sprintf("%sSCALED TO 0%s", colorGreen, colorReset)
		}

		fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%s\t%s\n",
			c.ServiceName,
			c.LaunchType,
			c.CurrentDesired,
			c.StoppedCount,
			c.ReapReason,
			actionStr,
		)
	}
	w.Flush()
	fmt.Println()

	if report.DryRun {
		fmt.Printf("💡 %s%d candidate service(s)%s would be scaled to desiredCount=0 (saving %d active tasks).\n",
			colorBold, candidateCount, colorReset, report.SavedTasks)
		fmt.Printf("   Run with %s--execute%s to apply scale-down and save compute/pull costs.\n\n",
			colorCyan, colorReset)
	} else {
		fmt.Printf("✅ %sSuccessfully scaled down %d service(s)%s (saved %d tasks).\n",
			colorGreen, report.ReapedCount, colorReset, report.SavedTasks)
		fmt.Printf("   Original counts are tagged on each service. Run %secrmap restore <service> --cluster %s%s to restore anytime.\n\n",
			colorCyan, report.ClusterName, colorReset)
	}

	return nil
}

// PrintRestoreResult formats the output when a service is restored.
func PrintRestoreResult(serviceName, clusterName string, restoredCount int32, jsonFormat bool) error {
	if jsonFormat {
		return PrintJSON(map[string]any{
			"service":        serviceName,
			"cluster":        clusterName,
			"restored_count": restoredCount,
			"status":         "restored",
		})
	}

	fmt.Printf("\n%s✔ Successfully restored service '%s' in cluster '%s' to desiredCount = %d%s\n\n",
		colorGreen, serviceName, clusterName, restoredCount, colorReset)
	return nil
}
