package oncall

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- T051: ParsePluginPreference table tests --------------------------------

func TestParsePluginPreference(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		in      string
		want    PluginPreference
		wantErr ErrorCode
	}{
		{name: "empty -> Unset", in: "", want: PluginPrefUnset, wantErr: ""},
		{name: "oncall-app lower", in: "oncall-app", want: PluginPrefOnCallApp, wantErr: ""},
		{name: "oncall-app upper", in: "ONCALL-APP", want: PluginPrefOnCallApp, wantErr: ""},
		{name: "oncall-app mixed case with spaces", in: "  OnCall-App  ", want: PluginPrefOnCallApp, wantErr: ""},
		{name: "irm lower", in: "irm", want: PluginPrefIRM, wantErr: ""},
		{name: "irm upper", in: "IRM", want: PluginPrefIRM, wantErr: ""},
		{name: "invalid value -> INVALID_INPUT", in: "foo", want: PluginPrefUnset, wantErr: ErrCodeInvalidInput},
		{name: "whitespace only -> Unset (treated as empty)", in: "  ", want: PluginPrefUnset, wantErr: ""},
		{name: "grafana-oncall-app full name -> INVALID_INPUT (operator must use short code)", in: "grafana-oncall-app", want: PluginPrefUnset, wantErr: ErrCodeInvalidInput},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, perr := ParsePluginPreference(c.in)
			if c.wantErr == "" {
				require.Nil(t, perr, "unexpected error for %q", c.in)
				assert.Equal(t, c.want, got)
			} else {
				require.NotNil(t, perr, "expected error for %q", c.in)
				assert.Equal(t, c.wantErr, perr.Code)
				assert.Equal(t, "startup", perr.Tool)
			}
		})
	}
}

// --- T051 / T052 / T058: SelectAndResolve precedence table -------------------

// startFakeGrafana returns an httptest.Server that responds to
// /api/plugins/{id}/settings with the configured (status, body) pair
// per plugin. The probe order is the parallel order in
// SelectAndResolve, so this is safe to use with a real (parallel)
// client. The returned server URL is suitable for use as the
// `grafanaURL` argument to SelectAndResolve.
func startFakeGrafana(t *testing.T, legacyStatus int, legacyBody string, irmStatus int, irmBody string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/plugins/grafana-oncall-app/settings", func(w http.ResponseWriter, r *http.Request) {
		if legacyStatus == http.StatusOK {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(legacyStatus)
		_, _ = io.WriteString(w, legacyBody)
	})
	mux.HandleFunc("/api/plugins/grafana-irm-app/settings", func(w http.ResponseWriter, r *http.Request) {
		if irmStatus == http.StatusOK {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(irmStatus)
		_, _ = io.WriteString(w, irmBody)
	})
	return httptest.NewServer(mux)
}

func TestSelectAndResolve_Precedence(t *testing.T) {
	t.Parallel()
	irmOKBody := `{"jsonData":{"onCallApiUrl":"https://example.invalid/oncall"}}`

	cases := []struct {
		name        string
		legacyStat  int
		legacyBody  string
		irmStat     int
		irmBody     string
		pref        PluginPreference
		wantPlugin  Plugin
		wantBaseURL func(srvURL string) string
		wantCode    ErrorCode // "" if no error
	}{
		// T051: 9-row precedence table from research.md Decision 3
		{
			name:        "unset + both installed -> legacy",
			legacyStat:  http.StatusOK,
			legacyBody:  "",
			irmStat:     http.StatusOK,
			irmBody:     irmOKBody,
			pref:        PluginPrefUnset,
			wantPlugin:  PluginOnCallApp,
			wantBaseURL: func(s string) string { return s + "/api/plugins/grafana-oncall-app/resources/api/v1/" },
		},
		{
			name:        "unset + legacy only -> legacy",
			legacyStat:  http.StatusOK,
			legacyBody:  "",
			irmStat:     http.StatusNotFound,
			irmBody:     "",
			pref:        PluginPrefUnset,
			wantPlugin:  PluginOnCallApp,
			wantBaseURL: func(s string) string { return s + "/api/plugins/grafana-oncall-app/resources/api/v1/" },
		},
		{
			name:        "unset + irm only -> irm",
			legacyStat:  http.StatusNotFound,
			legacyBody:  "",
			irmStat:     http.StatusOK,
			irmBody:     irmOKBody,
			pref:        PluginPrefUnset,
			wantPlugin:  PluginIRM,
			wantBaseURL: func(s string) string { return "https://example.invalid/oncall/api/v1/" },
		},
		{
			name:       "unset + neither installed -> ONCALL_PLUGIN_MISSING",
			legacyStat: http.StatusNotFound,
			legacyBody: "",
			irmStat:    http.StatusNotFound,
			irmBody:    "",
			pref:       PluginPrefUnset,
			wantCode:   ErrCodeOnCallPluginMissing,
		},
		{
			name:        "pref=oncall-app + legacy installed (any irm) -> legacy",
			legacyStat:  http.StatusOK,
			legacyBody:  "",
			irmStat:     http.StatusInternalServerError,
			irmBody:     "boom",
			pref:        PluginPrefOnCallApp,
			wantPlugin:  PluginOnCallApp,
			wantBaseURL: func(s string) string { return s + "/api/plugins/grafana-oncall-app/resources/api/v1/" },
		},
		{
			name:       "pref=oncall-app + legacy NOT installed -> INVALID_CONFIG",
			legacyStat: http.StatusNotFound,
			legacyBody: "",
			irmStat:    http.StatusOK,
			irmBody:    irmOKBody,
			pref:       PluginPrefOnCallApp,
			wantCode:   ErrCodeInvalidInput,
		},
		{
			name:        "pref=irm + irm installed (any legacy) -> irm",
			legacyStat:  http.StatusInternalServerError,
			legacyBody:  "boom",
			irmStat:     http.StatusOK,
			irmBody:     irmOKBody,
			pref:        PluginPrefIRM,
			wantPlugin:  PluginIRM,
			wantBaseURL: func(s string) string { return "https://example.invalid/oncall/api/v1/" },
		},
		{
			name:       "pref=irm + irm NOT installed -> INVALID_CONFIG",
			legacyStat: http.StatusOK,
			legacyBody: "",
			irmStat:    http.StatusNotFound,
			irmBody:    "",
			pref:       PluginPrefIRM,
			wantCode:   ErrCodeInvalidInput,
		},
		// T058: legacy-only case is the "unset + legacy only" row,
		// already covered above. Add one more row that exercises the
		// explicit case the task asks for: legacy-only with IRM
		// returning 500.
		{
			name:        "legacy-only with IRM 5xx still selects legacy",
			legacyStat:  http.StatusOK,
			legacyBody:  "",
			irmStat:     http.StatusInternalServerError,
			irmBody:     "boom",
			pref:        PluginPrefUnset,
			wantPlugin:  PluginOnCallApp,
			wantBaseURL: func(s string) string { return s + "/api/plugins/grafana-oncall-app/resources/api/v1/" },
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			srv := startFakeGrafana(t, c.legacyStat, c.legacyBody, c.irmStat, c.irmBody)
			defer srv.Close()

			gotPlugin, gotBase, perr := SelectAndResolve(context.Background(), srv.URL, "test-token", c.pref, nil)

			if c.wantCode != "" {
				require.NotNil(t, perr, "expected error code %s", c.wantCode)
				assert.Equal(t, c.wantCode, perr.Code)
				assert.NotNil(t, perr.Hint, "preference/installation errors must carry a hint naming the installed plugins")
				return
			}
			require.Nil(t, perr, "unexpected error: %v", perr)
			assert.Equal(t, c.wantPlugin, gotPlugin)
			assert.Equal(t, c.wantBaseURL(srv.URL), gotBase)
		})
	}
}

// --- T052 / T058: per-plugin URL resolution --------------------------------

func TestSelectAndResolve_LegacyURLTemplate(t *testing.T) {
	t.Parallel()
	srv := startFakeGrafana(t, http.StatusOK, "", http.StatusNotFound, "")
	defer srv.Close()

	plugin, base, perr := SelectAndResolve(context.Background(), srv.URL, "tok", PluginPrefUnset, nil)
	require.Nil(t, perr)
	assert.Equal(t, PluginOnCallApp, plugin)
	assert.Equal(t,
		srv.URL+"/api/plugins/grafana-oncall-app/resources/api/v1/",
		base,
	)
}

func TestSelectAndResolve_IRMBaseURLNormalisation(t *testing.T) {
	t.Parallel()
	// IRM plugin with onCallApiUrl that ALREADY ends in /api/v1/ ->
	// must not be double-suffixed.
	cases := []struct {
		name     string
		raw      string
		wantBase string
	}{
		{name: "no path no trailing slash", raw: "https://oncall.invalid", wantBase: "https://oncall.invalid/api/v1/"},
		{name: "with trailing slash", raw: "https://oncall.invalid/", wantBase: "https://oncall.invalid/api/v1/"},
		{name: "with /api/v1", raw: "https://oncall.invalid/api/v1", wantBase: "https://oncall.invalid/api/v1/"},
		{name: "with /api/v1/", raw: "https://oncall.invalid/api/v1/", wantBase: "https://oncall.invalid/api/v1/"},
		{name: "with subpath", raw: "https://oncall.invalid/some/path/", wantBase: "https://oncall.invalid/some/path/api/v1/"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			srv := startFakeGrafana(t, http.StatusNotFound, "", http.StatusOK, `{"jsonData":{"onCallApiUrl":"`+c.raw+"\"}}")
			defer srv.Close()
			_, got, perr := SelectAndResolve(context.Background(), srv.URL, "tok", PluginPrefUnset, nil)
			require.Nil(t, perr)
			assert.Equal(t, c.wantBase, got)
		})
	}
}

// --- T059: log capture (INFO selected plugin, WARN when both installed) ---

// captureLogs returns a slog.Logger that writes JSON records into the
// returned bytes.Buffer (one record per line). The logger is safe for
// concurrent use by the parallel probes in SelectAndResolve.
func captureLogs() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	h := slog.NewJSONHandler(&syncBuf{Buffer: &buf}, &slog.HandlerOptions{Level: slog.LevelDebug})
	return slog.New(h), &buf
}

type syncBuf struct {
	*bytes.Buffer
	mu atomic.Int32
}

func (s *syncBuf) Write(p []byte) (int, error) {
	for !s.mu.CompareAndSwap(0, 1) {
	}
	defer s.mu.Store(0)
	return s.Buffer.Write(p)
}

func TestSelectAndResolve_LogsInfoOnSelection(t *testing.T) {
	t.Parallel()
	srv := startFakeGrafana(t, http.StatusNotFound, "", http.StatusOK, `{"jsonData":{"onCallApiUrl":"https://x.example/api/v1"}}`)
	defer srv.Close()
	logger, buf := captureLogs()
	plugin, _, perr := SelectAndResolve(context.Background(), srv.URL, "tok", PluginPrefUnset, logger)
	require.Nil(t, perr)
	assert.Equal(t, PluginIRM, plugin)

	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &m))
		records = append(records, m)
	}
	require.NotEmpty(t, records)
	found := false
	for _, r := range records {
		if r["msg"] == "oncall plugin selected" {
			assert.Equal(t, "INFO", r["level"])
			assert.Equal(t, "irm", r["plugin"])
			found = true
		}
	}
	assert.True(t, found, "expected an INFO 'oncall plugin selected' log line, got: %v", records)
}

func TestSelectAndResolve_LogsWarnWhenLegacyPreferredOverIRM(t *testing.T) {
	t.Parallel()
	srv := startFakeGrafana(t, http.StatusOK, "", http.StatusOK, `{"jsonData":{"onCallApiUrl":"https://x.example/api/v1"}}`)
	defer srv.Close()
	logger, buf := captureLogs()
	plugin, _, perr := SelectAndResolve(context.Background(), srv.URL, "tok", PluginPrefUnset, logger)
	require.Nil(t, perr)
	assert.Equal(t, PluginOnCallApp, plugin)

	var sawInfo, sawWarn bool
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &m))
		if m["msg"] == "oncall plugin selected" {
			sawInfo = true
		}
		if strings.Contains(fmt.Sprint(m["msg"]), "legacy plugin preferred") {
			sawWarn = true
			assert.Equal(t, "WARN", m["level"])
		}
	}
	assert.True(t, sawInfo, "expected INFO line when both installed")
	assert.True(t, sawWarn, "expected WARN line when both installed and no preference is set")
}

func TestSelectAndResolve_NoWarnWhenPreferenceForcesIRM(t *testing.T) {
	t.Parallel()
	srv := startFakeGrafana(t, http.StatusOK, "", http.StatusOK, `{"jsonData":{"onCallApiUrl":"https://x.example/api/v1"}}`)
	defer srv.Close()
	logger, buf := captureLogs()
	plugin, _, perr := SelectAndResolve(context.Background(), srv.URL, "tok", PluginPrefIRM, logger)
	require.Nil(t, perr)
	assert.Equal(t, PluginIRM, plugin)
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &m))
		assert.NotEqual(t, "WARN", m["level"], "operator-forced IRM must not emit a WARN about legacy-preferred")
	}
}

// --- T051: invalid input ---------------------------------------------------

func TestSelectAndResolve_EmptyGrafanaURLIsInvalidInput(t *testing.T) {
	t.Parallel()
	_, _, perr := SelectAndResolve(context.Background(), "", "tok", PluginPrefUnset, nil)
	require.NotNil(t, perr)
	assert.Equal(t, ErrCodeInvalidInput, perr.Code)
}

func TestSelectAndResolve_MalformedGrafanaURLIsInvalidInput(t *testing.T) {
	t.Parallel()
	_, _, perr := SelectAndResolve(context.Background(), "://not a url", "tok", PluginPrefUnset, nil)
	require.NotNil(t, perr)
	assert.Equal(t, ErrCodeInvalidInput, perr.Code)
}

// --- T051: ONCALL_PLUGIN_MISSING hint ---------------------------------------

func TestSelectAndResolve_NeitherInstalled_HintMentionsBothPlugins(t *testing.T) {
	t.Parallel()
	srv := startFakeGrafana(t, http.StatusNotFound, "", http.StatusNotFound, "")
	defer srv.Close()
	_, _, perr := SelectAndResolve(context.Background(), srv.URL, "tok", PluginPrefUnset, nil)
	require.NotNil(t, perr)
	assert.Equal(t, ErrCodeOnCallPluginMissing, perr.Code)
	require.NotNil(t, perr.Hint)
	assert.Contains(t, *perr.Hint, "grafana-oncall-app")
	assert.Contains(t, *perr.Hint, "grafana-irm-app")
}
