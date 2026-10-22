package parser

import (
	"testing"
)

func TestParseImageURI(t *testing.T) {
	tests := []struct {
		name         string
		uri          string
		wantRepo     string
		wantTag      string
		wantDigest   string
		wantAccount  string
		wantRegion   string
		wantIsECR    bool
	}{
		{
			name:        "Standard ECR with tag",
			uri:         "123456789012.dkr.ecr.us-east-1.amazonaws.com/payment-service:v1.2.3",
			wantRepo:    "payment-service",
			wantTag:     "v1.2.3",
			wantAccount: "123456789012",
			wantRegion:  "us-east-1",
			wantIsECR:   true,
		},
		{
			name:        "ECR with nested repository and tag",
			uri:         "123456789012.dkr.ecr.ap-south-1.amazonaws.com/org/payments/api:latest",
			wantRepo:    "org/payments/api",
			wantTag:     "latest",
			wantAccount: "123456789012",
			wantRegion:  "ap-south-1",
			wantIsECR:   true,
		},
		{
			name:        "ECR with digest",
			uri:         "123456789012.dkr.ecr.eu-west-1.amazonaws.com/auth-svc@sha256:4f53cda18c2baa0c0354bb5f9a3ecbe5ed12ab4d8e11ba873c65049ef21d6e53",
			wantRepo:    "auth-svc",
			wantTag:     "",
			wantDigest:  "sha256:4f53cda18c2baa0c0354bb5f9a3ecbe5ed12ab4d8e11ba873c65049ef21d6e53",
			wantAccount: "123456789012",
			wantRegion:  "eu-west-1",
			wantIsECR:   true,
		},
		{
			name:        "ECR with tag AND digest",
			uri:         "123456789012.dkr.ecr.us-west-2.amazonaws.com/worker:v2.0@sha256:abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234",
			wantRepo:    "worker",
			wantTag:     "v2.0",
			wantDigest:  "sha256:abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234",
			wantAccount: "123456789012",
			wantRegion:  "us-west-2",
			wantIsECR:   true,
		},
		{
			name:       "DockerHub official",
			uri:        "nginx:alpine",
			wantRepo:   "nginx",
			wantTag:    "alpine",
			wantIsECR:  false,
		},
		{
			name:       "Quay.io repository",
			uri:        "quay.io/coreos/etcd:v3.5.0",
			wantRepo:   "coreos/etcd",
			wantTag:    "v3.5.0",
			wantIsECR:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseImageURI(tt.uri)
			if got.Repository != tt.wantRepo {
				t.Errorf("Repository = %q, want %q", got.Repository, tt.wantRepo)
			}
			if got.Tag != tt.wantTag {
				t.Errorf("Tag = %q, want %q", got.Tag, tt.wantTag)
			}
			if got.Digest != tt.wantDigest {
				t.Errorf("Digest = %q, want %q", got.Digest, tt.wantDigest)
			}
			if got.AccountID != tt.wantAccount {
				t.Errorf("AccountID = %q, want %q", got.AccountID, tt.wantAccount)
			}
			if got.Region != tt.wantRegion {
				t.Errorf("Region = %q, want %q", got.Region, tt.wantRegion)
			}
			if got.IsECR != tt.wantIsECR {
				t.Errorf("IsECR = %v, want %v", got.IsECR, tt.wantIsECR)
			}
		})
	}
}

func TestMatches(t *testing.T) {
	ref := ParseImageURI("123456789012.dkr.ecr.us-east-1.amazonaws.com/org/payments/api:v1.2.3@sha256:4f53cda18c2b")

	cases := []struct {
		query string
		match bool
	}{
		{"v1.2.3", false}, // Tag alone without repo shouldn't match repo:tag unless specified as query
		{"api:v1.2.3", true},
		{"org/payments/api:v1.2.3", true},
		{"api", true},
		{"org/payments/api", true},
		{"4f53cda1", true},
		{"sha256:4f53cda1", true},
		{"123456789012.dkr.ecr.us-east-1.amazonaws.com/org/payments/api:v1.2.3@sha256:4f53cda18c2b", true},
		{"unrelated-service", false},
	}

	for _, c := range cases {
		got := Matches(ref, c.query)
		if got != c.match {
			t.Errorf("Matches(%q) = %v; want %v", c.query, got, c.match)
		}
	}
}
