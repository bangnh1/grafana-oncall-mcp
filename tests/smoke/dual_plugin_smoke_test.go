//go:build smoke

// Package smoke_test exercises the dual-plugin startup probe
// end-to-end against an in-process fake Grafana and a real
// `grafana-oncall-mcp` binary. It is gated by the `smoke` build
// tag so it does not run as part of the unit or integration test
// matrix; invoke it via
//
//	go test -tags smoke ./tests/smoke
//
// to validate the quickstart scenarios from
// specs/002-multi-plugin-support/quickstart.md against the local
// implementation.
package smoke_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDualPluginSmoke(t *testing.T) {
	// Build the binary once for the entire table; each sub-test
	// only spins up its own fake Grafana.
	bin := buildBinary(t)

	// fixture selects which plugins the fake Grafana reports as
	// installed; preference is the value of
	// GRAFANA_ONCALL_PLUGIN_PREFERENCE the binary is started with.
	type fixture struct {
		name            string
		legacyInstalled bool
		irmInstalled    bool
		irmURL          string
		preference      string
		wantSelected    string
		wantErrSubstr   string
	}

	cases := []fixture{
		{name: "legacy-only, no pref", legacyInstalled: true, irmInstalled: false, wantSelected: "oncall-app"},
		{name: "legacy-only, pref=oncall-app", legacyInstalled: true, irmInstalled: false, preference: "oncall-app", wantSelected: "oncall-app"},
		{name: "irm-only, no pref", irmInstalled: true, irmURL: "https://example.invalid/oncall", wantSelected: "irm"},
		{name: "irm-only, pref=irm", irmInstalled: true, irmURL: "https://example.invalid/oncall", preference: "irm", wantSelected: "irm"},
		{name: "both, no pref -> legacy preferred (WARN)", legacyInstalled: true, irmInstalled: true, irmURL: "https://example.invalid/oncall", wantSelected: "oncall-app", wantErrSubstr: "WARN"},
		{name: "both, pref=irm", legacyInstalled: true, irmInstalled: true, irmURL: "https://example.invalid/oncall", preference: "irm", wantSelected: "irm"},
		{name: "irm-only, pref=oncall-app -> INVALID_INPUT", irmInstalled: true, irmURL: "https://example.invalid/oncall", preference: "oncall-app", wantErrSubstr: "INVALID_INPUT"},
		{name: "legacy-only, pref=irm -> INVALID_INPUT", legacyInstalled: true, irmInstalled: false, preference: "irm", wantErrSubstr: "INVALID_INPUT"},
		{name: "neither, no pref -> ONCALL_PLUGIN_MISSING", wantErrSubstr: "ONCALL_PLUGIN_MISSING"},
		{name: "preference=bogus -> INVALID_CONFIG", legacyInstalled: true, irmInstalled: false, preference: "bogus", wantErrSubstr: "INVALID_CONFIG"},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			srv := newFakeGrafana(t, c.legacyInstalled, c.irmInstalled, c.irmURL)
			defer srv.Close()

			env := append(os.Environ(),
				"GRAFANA_URL="+srv.URL,
				"GRAFANA_SERVICE_ACCOUNT_TOKEN=smoke-test-token",
				"PATH="+os.Getenv("PATH"),
			)
			if c.preference != "" {
				env = append(env, "GRAFANA_ONCALL_PLUGIN_PREFERENCE="+c.preference)
			}

			// A short probe — the server should fail to start in
			// <1s on a misconfigured fixture; on a valid fixture
			// it just runs and we kill it after the probe
			// completes.
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, bin, "-transport=stdio")
			cmd.Env = env
			stdout, _ := cmd.StdoutPipe()
			stderr, _ := cmd.StderrPipe()

			startErr := cmd.Start()
			if startErr != nil {
				t.Fatalf("start binary: %v", startErr)
			}

			// Read stderr in a goroutine; the binary writes
			// structured slog records there.
			stderrBytes := make(chan []byte, 1)
			go func() {
				b, _ := io.ReadAll(stderr)
				stderrBytes <- b
			}()

			// Give the probe time to complete; on success the
			// binary waits for stdin (stdio), so we just close
			// it after a short window. On failure the binary
			// exits non-zero within the probe window.
			select {
			case <-time.After(800 * time.Millisecond):
				_ = cmd.Process.Kill()
			case <-ctx.Done():
			}
			_ = cmd.Wait()
			_ = stdout.Close()

			output, _ := io.ReadAll(strings.NewReader(""))
			_ = output
			out := <-stderrBytes

			if c.wantErrSubstr != "" {
				if !strings.Contains(string(out), c.wantErrSubstr) {
					t.Fatalf("expected stderr to contain %q, got:\n%s", c.wantErrSubstr, string(out))
				}
				return
			}

			want := fmt.Sprintf(`"plugin":"%s"`, c.wantSelected)
			if !strings.Contains(string(out), want) {
				t.Fatalf("expected stderr to contain %q, got:\n%s", want, string(out))
			}
		})
	}
}

// buildBinary compiles the oncall-mcp binary into a temp file and
// returns its absolute path. Subsequent test runs reuse the cached
// binary if it is newer than the source files.
func buildBinary(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "oncall-mcp-smoke-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	bin := filepath.Join(dir, "grafana-oncall-mcp")
	// Walk up to the repo root: tests/smoke/X -> tests/smoke -> tests -> repo root.
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/oncall-mcp")
	cmd.Dir = root
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build: %v", err)
	}
	return bin
}

func newFakeGrafana(t *testing.T, legacy, irm bool, irmURL string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/plugins/grafana-oncall-app/settings", func(w http.ResponseWriter, r *http.Request) {
		if !legacy {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"grafana-oncall-app","enabled":true}`))
	})
	mux.HandleFunc("/api/plugins/grafana-irm-app/settings", func(w http.ResponseWriter, r *http.Request) {
		if !irm {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		body, _ := json.Marshal(map[string]interface{}{
			"id": "grafana-irm-app", "enabled": true,
			"jsonData": map[string]interface{}{"onCallApiUrl": irmURL},
		})
		_, _ = w.Write(body)
	})
	return httptest.NewServer(mux)
}

var _ = log.Print
