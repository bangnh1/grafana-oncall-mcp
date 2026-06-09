//go:build e2e

package e2e

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildBinary(t *testing.T) string {
	t.Helper()
	_, self, _, _ := runtime.Caller(0)
	root := filepath.Join(self, "..", "..", "..")
	bin := filepath.Join(os.TempDir(), "grafana-oncall-mcp-e2e-"+t.Name())
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/oncall-mcp")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	return bin
}

func TestE2EListTools(t *testing.T) {
	bin := buildBinary(t)
	defer os.Remove(bin)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	env := []string{
		"GRAFANA_URL=https://example.invalid",
	}

	// We only test that the binary starts and can list tools when upstream is unreachable.
	// A real E2E test would point to a test instance.
	cli, err := client.NewStdioMCPClient(bin, env, "-transport=stdio", "-log-level=error")
	require.NoError(t, err)
	defer cli.Close()

	initRes, err := cli.Initialize(ctx, mcp.InitializeRequest{})
	require.NoError(t, err, "client initialization failed: the server should start even when upstream is unreachable at tool registration time")
	assert.Equal(t, "grafana-oncall-mcp", initRes.ServerInfo.Name)
	assert.Equal(t, "0.1.0", initRes.ServerInfo.Version)

	toolsRes, err := cli.ListTools(ctx, mcp.ListToolsRequest{})
	require.NoError(t, err)
	assert.NotEmpty(t, toolsRes.Tools, "expected registered tools")
}
