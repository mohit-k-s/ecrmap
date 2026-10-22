package model

import "time"

// ReapCandidate represents an ECS service that is flapping and candidate for auto-scale down.
type ReapCandidate struct {
	Region          string `json:"region"`
	ClusterName     string `json:"cluster_name"`
	ClusterArn      string `json:"cluster_arn"`
	ServiceName     string `json:"service_name"`
	ServiceArn      string `json:"service_arn"`
	LaunchType      string `json:"launch_type"`
	CurrentDesired  int32  `json:"current_desired"`
	CurrentRunning  int32  `json:"current_running"`
	StoppedCount    int    `json:"stopped_count"`
	PrimaryReason   string `json:"primary_reason"`
	PrimaryExitCode int32  `json:"primary_exit_code"`
	ShouldReap      bool   `json:"should_reap"`
	ReapReason      string `json:"reap_reason"`
}

// ReapReport summarizes the analysis and execution of a reap operation.
type ReapReport struct {
	ClusterName  string          `json:"cluster_name"`
	Region       string          `json:"region"`
	TimeWindow   time.Duration   `json:"time_window"`
	Threshold    int             `json:"threshold"`
	DryRun       bool            `json:"dry_run"`
	Candidates   []ReapCandidate `json:"candidates"`
	ReapedCount  int             `json:"reaped_count"`
	SavedTasks   int32           `json:"saved_tasks"`
}
