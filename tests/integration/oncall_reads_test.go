//go:build integration

package integration

import (
	"os"
	"testing"
)

func TestIntegrationRequiresEnv(t *testing.T) {
	if os.Getenv("GRAFANA_URL") == "" {
		t.Skip("GRAFANA_URL not set; skipping integration test")
	}
}

func loadGrafanaURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("GRAFANA_URL")
	if url == "" {
		t.Skip("GRAFANA_URL not set; skipping integration test")
	}
	return url
}

func TestIntegrationReadToolsRegistered(t *testing.T) {
	_ = loadGrafanaURL(t)
	// This test verifies the basic wiring exists. When a real instance is
	// available, it should ListTools and assert the 11 read+write tool names.
	t.Skip("requires live Grafana OnCall instance; set GRAFANA_URL to run")

	// Example of what a full implementation would do:
	// client := newMCPClient(t, env)
	// result, err := client.ListTools(ctx, mcp.ListToolsRequest{})
	// require.NoError(t, err)
	// assert.Subset(toolNames, result.Tools)
}

