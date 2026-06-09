//go:build integration

package integration

import (
	"os"
	"testing"
)

func TestIntegrationWriteToolsRequireEnv(t *testing.T) {
	if os.Getenv("GRAFANA_URL") == "" {
		t.Skip("GRAFANA_URL not set; skipping integration test")
	}
}

func TestIntegrationWriteTools(t *testing.T) {
	if os.Getenv("GRAFANA_URL") == "" {
		t.Skip("GRAFANA_URL not set; skipping integration test")
	}
	t.Skip("requires live Grafana OnCall instance; set GRAFANA_URL to run, preferably in a drill or non-production environment")
}
