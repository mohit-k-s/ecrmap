package ecs

import (
	"testing"
)

func TestIsProductionCluster(t *testing.T) {
	tests := []struct {
		cluster string
		isProd  bool
	}{
		{"prod-useast1", true},
		{"production-cluster", true},
		{"PROD_BACKEND", true},
		{"payment-production", true},
		{"dev-cluster", false},
		{"staging-useast1", false},
		{"qa-tests", false},
	}

	for _, tt := range tests {
		got := IsProductionCluster(tt.cluster)
		if got != tt.isProd {
			t.Errorf("IsProductionCluster(%q) = %v, want %v", tt.cluster, got, tt.isProd)
		}
	}
}
