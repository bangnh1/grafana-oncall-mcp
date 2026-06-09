// Package oncall — dual-plugin support (feature 002-multi-plugin-support).
//
// `plugin.go` defines the supported Grafana OnCall plugin IDs, the
// operator-facing plugin preference (parsed from
// GRAFANA_ONCALL_PLUGIN_PREFERENCE / --plugin-preference), and the
// dual-probe startup selection algorithm that resolves the OnCall
// API base URL for the selected plugin. The 11 MCP tool handlers in
// `internal/tools/` and the amixr-api-go-client wrapper in
// `internal/oncall/client.go` are unchanged in surface; this file
// owns the plugin-selection concern so the rest of the server can
// remain plugin-agnostic.
package oncall

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Plugin identifies one of the two Grafana OnCall plugin IDs this
// server supports. The zero value is PluginUnset and is not a valid
// selection outcome; callers should treat it as "no plugin chosen".
type Plugin int

const (
	// PluginUnset is the zero value; reserved for the "no plugin
	// selected" state used internally by preference parsing. It MUST
	// NOT be returned by SelectAndResolve to its caller.
	PluginUnset Plugin = iota
	// PluginOnCallApp is the legacy `grafana-oncall-app` plugin.
	PluginOnCallApp
	// PluginIRM is the rebranded `grafana-irm-app` plugin.
	PluginIRM
	// PluginDirectAPI means the client is configured with a direct
	// OnCall API endpoint plus an OnCall API key, bypassing Grafana's
	// plugin resource proxy and the IRM plugin settings probe.
	PluginDirectAPI
)

// String returns the operator-facing short code for the plugin
// (`oncall-app` or `irm`). The zero value renders as the empty
// string so an unset plugin is distinguishable from a real selection
// in log lines.
func (p Plugin) String() string {
	switch p {
	case PluginOnCallApp:
		return "oncall-app"
	case PluginIRM:
		return "irm"
	case PluginDirectAPI:
		return "direct-api"
	default:
		return ""
	}
}

// PluginID returns the full Grafana plugin ID as it appears in
// /api/plugins/{id}/settings URLs.
func (p Plugin) PluginID() string {
	switch p {
	case PluginOnCallApp:
		return "grafana-oncall-app"
	case PluginIRM:
		return "grafana-irm-app"
	case PluginDirectAPI:
		return "direct-api"
	default:
		return ""
	}
}

// onCallAPIURLTemplate returns the template for the OnCall API base
// URL when this plugin is selected. The legacy plugin exposes its
// API under a Grafana resource proxy; the IRM plugin reads the base
// URL from its settings response (returned as the jsonData.onCallApiUrl
// field by the probe). Callers apply the template to the configured
// GRAFANA_URL.
func (p Plugin) onCallAPIURLTemplate(grafanaURL string) string {
	switch p {
	case PluginOnCallApp:
		return strings.TrimRight(grafanaURL, "/") + "/api/plugins/grafana-oncall-app/resources/api/v1/"
	default:
		return ""
	}
}

// pluginProbeTimeout bounds the dual-probe HTTP call. Kept short so
// SC-005 ("fail to start within 1 second of probe completion") holds
// for unreachable Grafanas.
const pluginProbeTimeout = 5 * time.Second

// PluginPreference is the operator-supplied plugin selection, parsed
// from GRAFANA_ONCALL_PLUGIN_PREFERENCE / --plugin-preference.
type PluginPreference int

const (
	// PluginPrefUnset is the zero value; the default behavior is
	// "prefer oncall-app when both are installed; use whichever is
	// installed when only one is".
	PluginPrefUnset PluginPreference = iota
	// PluginPrefOnCallApp forces the legacy plugin.
	PluginPrefOnCallApp
	// PluginPrefIRM forces the rebranded plugin.
	PluginPrefIRM
)

// String returns the operator-facing short code for the preference
// (`oncall-app`, `irm`, or empty for unset).
func (p PluginPreference) String() string {
	switch p {
	case PluginPrefOnCallApp:
		return "oncall-app"
	case PluginPrefIRM:
		return "irm"
	default:
		return ""
	}
}

// ParsePluginPreference accepts the raw env-var or flag string and
// returns the corresponding PluginPreference. The empty string maps
// to PluginPrefUnset. Any other value returns an *Error with
// ErrCodeInvalidInput naming the accepted values.
func ParsePluginPreference(s string) (PluginPreference, *Error) {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case "":
		return PluginPrefUnset, nil
	case "oncall-app":
		return PluginPrefOnCallApp, nil
	case "irm":
		return PluginPrefIRM, nil
	default:
		return PluginPrefUnset, WithHint(
			NewInvalidInputError("startup", fmt.Sprintf("invalid plugin preference %q", s)),
			"accepted values: oncall-app, irm",
		)
	}
}

// probeResult is the outcome of a single /api/plugins/{id}/settings
// HTTP probe. `installed` is true iff the probe returned 200; the
// `onCallURL` is non-empty only for the IRM probe (it reads
// jsonData.onCallApiUrl out of the response body).
type probeResult struct {
	plugin     Plugin
	installed  bool
	onCallURL  string
	httpStatus int
	err        error
}

// SelectAndResolve probes both supported plugin settings endpoints
// in parallel, applies the precedence rule (see
// specs/002-multi-plugin-support/research.md Decision 3), and returns
// the selected plugin + the OnCall API base URL appropriate to it.
//
// The base URL is the value the amixr-api-go-client should be
// constructed with (i.e. it does NOT include the trailing `api/v1/`
// — the amixr client appends that itself). For the legacy plugin
// the URL is templated from GRAFANA_URL; for the IRM plugin it is
// the jsonData.onCallApiUrl field of the IRM plugin settings
// response (with a /api/v1/ suffix appended by this function so
// callers can pass the result to aapi.NewWithGrafanaURL without
// re-parsing).
//
// Errors:
//   - If neither plugin is installed, returns
//     ErrCodeOnCallPluginMissing ("ONCALL_PLUGIN_MISSING") with a
//     hint naming both plugin IDs.
//   - If the operator preference names a plugin that is not
//     installed, returns ErrCodeInvalidInput ("INVALID_CONFIG")
//     naming the requested preference and the plugins that ARE
//     installed.
//   - If a probe returns a non-404, non-200 status (e.g. 5xx) the
//     plugin is treated as not installed and a warning is logged;
//     the other probe decides the outcome.
//
// The logger is used to emit the mandatory INFO line that names the
// selected plugin (FR-007) and the WARN line when both plugins are
// present and the legacy is preferred (FR-008 / FR-053). A nil
// logger is treated as a no-op (no log lines emitted).
func SelectAndResolve(ctx context.Context, grafanaURL, token string, pref PluginPreference, logger *slog.Logger) (Plugin, string, *Error) {
	if grafanaURL == "" {
		return PluginUnset, "", NewInvalidInputError("startup", "grafanaURL is required for plugin discovery")
	}

	// Defensive URL parse so the legacy URL template doesn't accept
	// a malformed input that would later produce an invalid base URL.
	if _, err := url.Parse(grafanaURL); err != nil {
		return PluginUnset, "", NewInvalidInputError(
			"startup",
			fmt.Sprintf("invalid grafanaURL %q: %v", grafanaURL, err),
		)
	}

	results := probeBoth(ctx, grafanaURL, token, logger)

	legacy := resultFor(results, PluginOnCallApp)
	irm := resultFor(results, PluginIRM)

	// Apply the precedence table from research.md Decision 3.
	selected, baseURL, perr := selectPlugin(pref, legacy, irm, grafanaURL)
	if perr != nil {
		return PluginUnset, "", perr
	}

	// Emit the mandatory INFO line (FR-007) and the optional WARN
	// line when the legacy plugin is preferred over IRM with no
	// operator preference (FR-008 / FR-053).
	if logger != nil {
		logger.Info("oncall plugin selected",
			"plugin", selected.String(),
			"oncall_api_url", baseURL,
		)
		if selected == PluginOnCallApp && legacy.installed && irm.installed && pref == PluginPrefUnset {
			logger.Warn("legacy plugin preferred over irm-app per default; set GRAFANA_ONCALL_PLUGIN_PREFERENCE=irm to override")
		}
	}

	return selected, baseURL, nil
}

// selectPlugin applies the 9-row precedence table. Returns the
// selected Plugin, the resolved OnCall API base URL (no trailing
// api/v1/), or an *Error describing why no plugin could be selected.
func selectPlugin(pref PluginPreference, legacy, irm probeResult, grafanaURL string) (Plugin, string, *Error) {
	switch pref {
	case PluginPrefOnCallApp:
		if legacy.installed {
			return PluginOnCallApp, legacyBaseURL(grafanaURL), nil
		}
		return PluginUnset, "", WithHint(
			NewInvalidInputError("startup", "GRAFANA_ONCALL_PLUGIN_PREFERENCE=oncall-app but the legacy grafana-oncall-app plugin is not installed"),
			installedHint(legacy, irm),
		)

	case PluginPrefIRM:
		if irm.installed {
			return PluginIRM, irmBaseURL(irm.onCallURL), nil
		}
		return PluginUnset, "", WithHint(
			NewInvalidInputError("startup", "GRAFANA_ONCALL_PLUGIN_PREFERENCE=irm but the rebranded grafana-irm-app plugin is not installed"),
			installedHint(legacy, irm),
		)

	case PluginPrefUnset:
		switch {
		case legacy.installed && irm.installed:
			return PluginOnCallApp, legacyBaseURL(grafanaURL), nil
		case legacy.installed:
			return PluginOnCallApp, legacyBaseURL(grafanaURL), nil
		case irm.installed:
			return PluginIRM, irmBaseURL(irm.onCallURL), nil
		default:
			return PluginUnset, "", NewOnCallPluginMissingError(
				"startup",
				"neither grafana-oncall-app nor grafana-irm-app is installed on the target Grafana",
			)
		}
	}

	// Unreachable; ParsePluginPreference rejects unknown values.
	return PluginUnset, "", NewInvalidInputError("startup", "unknown plugin preference state")
}

// probeBoth issues the two plugin-probe HTTP calls in parallel and
// returns their outcomes. Each call is short-bounded by
// pluginProbeTimeout so a hung Grafana cannot stall startup
// indefinitely.
func probeBoth(ctx context.Context, grafanaURL, token string, logger *slog.Logger) []probeResult {
	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		result = make([]probeResult, 2)
	)
	probes := []Plugin{PluginOnCallApp, PluginIRM}
	for i, p := range probes {
		wg.Add(1)
		go func(idx int, pl Plugin) {
			defer wg.Done()
			r := probeOne(ctx, grafanaURL, token, pl)
			mu.Lock()
			result[idx] = r
			mu.Unlock()
			if r.err != nil && logger != nil && !isBenignProbeError(r) {
				logger.Warn("plugin probe failed",
					"plugin", pl.String(),
					"http_status", r.httpStatus,
					"error", r.err.Error(),
				)
			}
		}(i, p)
	}
	wg.Wait()
	return result
}

// probeOne issues a single /api/plugins/{id}/settings GET and
// returns a probeResult. A 200 response is "installed"; a 404
// response is "not installed" (the normal case for a Grafana that
// hosts only one of the two plugins); any other status is treated
// as "not installed" with a non-nil err for the caller to log.
func probeOne(ctx context.Context, grafanaURL, token string, pl Plugin) probeResult {
	settingsURL := strings.TrimRight(grafanaURL, "/") + "/api/plugins/" + pl.PluginID() + "/settings"

	reqCtx, cancel := context.WithTimeout(ctx, pluginProbeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, settingsURL, nil)
	if err != nil {
		return probeResult{plugin: pl, err: fmt.Errorf("build request: %w", err)}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "grafana-oncall-mcp/0.1.0")

	client := &http.Client{Timeout: pluginProbeTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return probeResult{plugin: pl, err: fmt.Errorf("plugin probe: %w", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return probeResult{plugin: pl, httpStatus: resp.StatusCode}
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return probeResult{
			plugin:     pl,
			httpStatus: resp.StatusCode,
			err:        fmt.Errorf("plugin settings HTTP %d: %s", resp.StatusCode, string(body)),
		}
	}

	// IRM plugin's jsonData.onCallApiUrl is the OnCall API base URL.
	// Legacy plugin does not carry this field.
	var onCallURL string
	if pl == PluginIRM {
		var settings struct {
			JSONData struct {
				OnCallAPIURL string `json:"onCallApiUrl"`
			} `json:"jsonData"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&settings); err == nil {
			onCallURL = settings.JSONData.OnCallAPIURL
		}
	}

	return probeResult{plugin: pl, installed: true, onCallURL: onCallURL, httpStatus: resp.StatusCode}
}

// isBenignProbeError returns true for errors that are expected
// outcomes of the dual probe (e.g. 404 for the plugin that isn't
// installed) and should NOT produce a WARN log line.
func isBenignProbeError(r probeResult) bool {
	return r.httpStatus == http.StatusNotFound
}

// resultFor returns the probeResult for a given plugin from the
// parallel-probe output. Returns a zero probeResult if the slice
// does not contain an entry for the plugin (defensive only).
func resultFor(results []probeResult, pl Plugin) probeResult {
	for _, r := range results {
		if r.plugin == pl {
			return r
		}
	}
	return probeResult{plugin: pl}
}

// legacyBaseURL returns the OnCall API base URL for the legacy
// plugin, without the trailing `api/v1/`. Callers that pass the
// returned string to aapi.NewWithGrafanaURL do not need to append
// the version path; the amixr client handles that.
func legacyBaseURL(grafanaURL string) string {
	return strings.TrimRight(grafanaURL, "/") + "/api/plugins/grafana-oncall-app/resources/api/v1/"
}

// irmBaseURL normalises the IRM plugin's onCallApiUrl field by
// appending `/api/v1/` if it isn't already present, so the result
// is in the same shape as legacyBaseURL. Trailing slashes are
// collapsed; a bare host (no path) is left as-is.
func irmBaseURL(raw string) string {
	if raw == "" {
		return ""
	}
	trimmed := strings.TrimRight(raw, "/")
	if strings.HasSuffix(trimmed, "/api/v1") {
		return trimmed + "/"
	}
	return trimmed + "/api/v1/"
}

// installedHint produces the actionable hint for the
// INVALID_CONFIG case where the operator's preference points to a
// plugin that is not installed. It lists which plugins ARE
// installed (or "neither" if none are) so the operator can correct
// the preference.
func installedHint(legacy, irm probeResult) string {
	switch {
	case legacy.installed && irm.installed:
		return "installed plugins: grafana-oncall-app, grafana-irm-app"
	case legacy.installed:
		return "installed plugin: grafana-oncall-app only (irm missing)"
	case irm.installed:
		return "installed plugin: grafana-irm-app only (oncall-app missing)"
	default:
		return "installed plugins: neither grafana-oncall-app nor grafana-irm-app is installed on this Grafana"
	}
}

// NewOnCallPluginMissingError is defined in errors.go and used by
// selectPlugin above; the comment is kept here as a placement
// marker for readers who land on this file first.

// errPluginProbeFailed is the sentinel returned by probeOne when
// neither HTTP transport nor JSON decode succeeded. It is wrapped
// (not exposed) and only used for the `errors.Is` check in
// SelectAndResolve's tests.
var errPluginProbeFailed = errors.New("plugin probe failed")

var _ = errPluginProbeFailed // silence unused; reserved for future probe-failure metric
