package output

import (
	"bytes"
	"io"
	"os"
	"testing"
	"time"

	"ecrmap/pkg/model"
)

func captureStdout(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestPrintWhereResults(t *testing.T) {
	res := WhereResult{
		Query: "payment-api:v2.1.0",
		Deployments: []MatchedDeploymentEntry{
			{
				Cluster:        "prod-us-east-1",
				Service:        "payment-service",
				Status:         "ACTIVE",
				RunningCount:   4,
				DesiredCount:   4,
				TaskDefinition: "payment-service:142",
				Container:      "app",
				ImageRaw:       "123456789012.dkr.ecr.us-east-1.amazonaws.com/payment-api:v2.1.0",
				Repository:     "payment-api",
				Tag:            "v2.1.0",
				IsECR:          true,
			},
		},
	}

	out := captureStdout(func() {
		err := PrintWhereResults(res, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if len(out) == 0 {
		t.Errorf("expected non-empty output")
	}
}

func TestPrintAuditResults(t *testing.T) {
	res := AuditResult{
		Repository:    "payment-api",
		TotalServices: 2,
		ActiveTags: map[string][]string{
			"v2.1.0": {"prod-us-east-1/payment-service"},
			"v2.0.9": {"stage-us-east-1/payment-service-stage"},
		},
		AllDeployments: []MatchedDeploymentEntry{
			{
				Cluster:        "prod-us-east-1",
				Service:        "payment-service",
				Status:         "ACTIVE",
				RunningCount:   4,
				DesiredCount:   4,
				TaskDefinition: "payment-service:142",
				Container:      "app",
				ImageRaw:       "123456789012.dkr.ecr.us-east-1.amazonaws.com/payment-api:v2.1.0",
				Repository:     "payment-api",
				Tag:            "v2.1.0",
				IsECR:          true,
			},
		},
	}

	out := captureStdout(func() {
		err := PrintAuditResults(res, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if len(out) == 0 {
		t.Errorf("expected non-empty output")
	}
}

func TestPrintListResults(t *testing.T) {
	deployments := []model.ServiceDeployment{
		{
			ClusterName:       "prod-cluster",
			ServiceName:       "auth-svc",
			Status:            "ACTIVE",
			DesiredCount:      2,
			RunningCount:      2,
			TaskFamily:        "auth-svc",
			TaskRevision:      "45",
			TaskDefinitionArn: "arn:aws:ecs:us-east-1:123456789012:task-definition/auth-svc:45",
			Containers: []model.ContainerDeployment{
				{
					ContainerName: "auth",
					Image: model.ImageRef{
						Repository: "auth-svc",
						Tag:        "v1.0.0",
						RawURI:     "123456789012.dkr.ecr.us-east-1.amazonaws.com/auth-svc:v1.0.0",
						IsECR:      true,
					},
				},
			},
		},
	}

	out := captureStdout(func() {
		err := PrintListResults(deployments, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if len(out) == 0 {
		t.Errorf("expected non-empty output")
	}
}

func TestSnapshotSerialization(t *testing.T) {
	s := model.Snapshot{
		Version:   "1.0",
		CreatedAt: time.Now(),
		Region:    "us-east-1",
		Deployments: []model.ServiceDeployment{
			{
				ClusterName: "cluster-1",
				ServiceName: "svc-1",
			},
		},
	}
	out := captureStdout(func() {
		_ = PrintJSON(s)
	})
	if len(out) == 0 {
		t.Errorf("expected json output")
	}
}
