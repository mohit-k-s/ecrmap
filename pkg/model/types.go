package model

import "time"

// ImageRef represents a parsed container image reference.
type ImageRef struct {
	RawURI     string `json:"raw_uri"`
	Registry   string `json:"registry"`
	AccountID  string `json:"account_id,omitempty"`
	Region     string `json:"region,omitempty"`
	Repository string `json:"repository"`
	Tag        string `json:"tag,omitempty"`
	Digest     string `json:"digest,omitempty"`
	IsECR      bool   `json:"is_ecr"`
}

// ContainerDeployment describes a container inside a task definition.
type ContainerDeployment struct {
	ContainerName string   `json:"container_name"`
	Image         ImageRef `json:"image"`
	Essential     bool     `json:"essential"`
}

// ServiceDeployment represents an active ECS service and its deployed task definition.
type ServiceDeployment struct {
	Region             string                `json:"region"`
	ClusterName        string                `json:"cluster_name"`
	ClusterArn         string                `json:"cluster_arn"`
	ServiceName        string                `json:"service_name"`
	ServiceArn         string                `json:"service_arn"`
	Status             string                `json:"status"`
	DesiredCount       int32                 `json:"desired_count"`
	RunningCount       int32                 `json:"running_count"`
	TaskDefinitionArn  string                `json:"task_definition_arn"`
	TaskFamily         string                `json:"task_family"`
	TaskRevision       string                `json:"task_revision"`
	Containers         []ContainerDeployment `json:"containers"`
}

// Snapshot is the cached state of all active ECS service deployments.
type Snapshot struct {
	Version     string              `json:"version"`
	CreatedAt   time.Time           `json:"created_at"`
	Regions     []string            `json:"regions"`
	Deployments []ServiceDeployment `json:"deployments"`
}
