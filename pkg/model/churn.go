package model

import "time"

// ServiceChurnStat contains churn and pull metrics for an ECS service.
type ServiceChurnStat struct {
	ClusterName         string         `json:"cluster_name"`
	ServiceName         string         `json:"service_name"`
	LaunchType          string         `json:"launch_type"`
	DesiredCount        int32          `json:"desired_count"`
	RunningCount        int32          `json:"running_count"`
	StoppedCount        int            `json:"stopped_count"`
	EstimatedPulls      int            `json:"estimated_pulls"`
	PullsPerHour        float64        `json:"pulls_per_hour"`
	HealthStatus        string         `json:"health_status"` // "CRASH-LOOPING", "HIGH_CHURN", "HEALTHY"
	PrimaryExitCode     int32          `json:"primary_exit_code,omitempty"`
	PrimaryReason       string         `json:"primary_reason,omitempty"`
	ExitCodeCounts      map[int32]int  `json:"exit_code_counts,omitempty"`
	Recommendations     []string       `json:"recommendations,omitempty"`
}

// RepoChurnReport summarizes pull churn for an ECR repository.
type RepoChurnReport struct {
	Repository          string             `json:"repository"`
	Window              time.Duration      `json:"window"`
	AnalyzedServices    int                `json:"analyzed_services"`
	TotalEstimatedPulls int                `json:"total_estimated_pulls"`
	Services            []ServiceChurnStat `json:"services"`
}
