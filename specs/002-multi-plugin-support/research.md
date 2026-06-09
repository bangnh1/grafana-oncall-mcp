# Phase 0 Research — Multi-Plugin OnCall Support

**Branch**: `002-multi-plugin-support` · **Date**: 2026-06-09
**Source spec**: [`spec.md`](./spec.md) · **Plan**: [`plan.md`](./plan.md)
**Inherits**: [`specs/001-oncall-mcp-server/research.md`](../../001-oncall-mcp-server/research.md) (Decisions 1, 2, 4, 5, 7, 8, 9, 10, 11, 12, and parts of 13 carry over unchanged; Decisions 3, 6, and 13 are revised below).

This document records the research decisions that resolve every
"NEEDS CLARIFICATION" candidate from the Technical Context for the
dual-plugin feature. The user clarification in `spec.md` Session
2026-06-09 — *"support cả grafana-irm-app và grafana-oncall-app"* —
pins the bulk of the work to the prior single-plugin implementation;
remaining choices are local concerns (plugin selection precedence,
preference naming, error-code deprecation policy, dual-probe
fallback behavior).

Inputs that informed these decisions:

- The 2026-06-09 spec clarification (support both plugins).
- The 001-oncall-mcp-server research.md (Decisions 1–13), which
  remain the source of truth for unchanged concerns.
- The `grafana/amixr-api-go-client` v0.0.28 source (re-audited for
  this feature; the library's `Client.NewWithGrafanaURL(baseURL,
  token, grafanaURL)` accepts a base URL that may point at either
  plugin's resource proxy — there is no per-plugin code path in the
  client itself).
- The Grafana plugin URL conventions: the legacy `grafana-oncall-app`
  exposes its OnCall HTTP API at
  `{GRAFANA_URL}/api/plugins/grafana-oncall-app/resources/api/v1/`
  (a Grafana resource proxy); the rebranded `grafana-irm-app`
  exposes it at `jsonData.onCallApiUrl` (an arbitrary backend URL
  configured in the plugin settings, with `api/v1/` appended by
  the amixr client convention).
- The project constitution at `.specify/memory/constitution.md` (v1.0.0).

---

## Decision 3 (revised) — Grafana OnCall HTTP client and plugin selection

**Decision**: The `github.com/grafana/amixr-api-go-client v0.0.28`
client is used unchanged. Plugin selection is performed by a new
`internal/oncall/plugin.go` helper that exposes a
`func SelectAndResolve(ctx, grafanaURL, token, preference) (Plugin, string, error)`
function. The helper probes both `/api/plugins/grafana-oncall-app/settings`
and `/api/plugins/grafana-irm-app/settings` in parallel, applies the
precedence rule (see Decision 6), resolves the OnCall API base URL
appropriate to the selected plugin, and returns the selected plugin
ID + base URL. The returned base URL is then passed to
`aapi.NewWithGrafanaURL(baseURL, token, grafanaURL)` exactly as in
001; the amixr client appends `api/v1/` itself.

**Rationale**:
- The amixr client already speaks both URL shapes; the only
  per-plugin logic is the URL-resolution step. Centralising that
  step in `plugin.go` keeps the `client.go` HTTP plumbing
  identical to 001.
- Parallel probes (two `GET /settings` calls in goroutines) keep
  the dual-plugin startup latency at ~1 RTT (the slower of the
  two), well under SC-003's 100 ms overhead budget.
- Returning a `(Plugin, baseURL)` pair from a single function lets
  `client.go` stay agnostic to the plugin selection logic and lets
  the caller (the startup probe) log the selected plugin at the
  right level (INFO / WARN) with a single value.

**Plugin-resolution table**:

| Preference | `grafana-oncall-app` probe | `grafana-irm-app` probe | Outcome |
|---|---|---|---|
| `unset` | 200 | 200 | Use `oncall-app`; log `INFO plugin=oncall-app` and `WARN legacy plugin preferred over irm-app per default` |
| `unset` | 200 | 404 | Use `oncall-app`; log `INFO plugin=oncall-app` |
| `unset` | 404 | 200 | Use `irm`; log `INFO plugin=irm` |
| `unset` | 404 | 404 | `ONCALL_PLUGIN_MISSING`; hint names both plugin IDs |
| `oncall-app` | 200 | any | Use `oncall-app`; log `INFO plugin=oncall-app` |
| `oncall-app` | 404 | any | `INVALID_CONFIG`; hint names the requested preference and explains the plugin is not installed |
| `irm` | any | 200 | Use `irm`; log `INFO plugin=irm` |
| `irm` | any | 404 | `INVALID_CONFIG`; hint names the requested preference and explains the plugin is not installed |
| any other value | n/a | n/a | `INVALID_CONFIG`; hint names the accepted values (`oncall-app`, `irm`) |

**Alternatives considered**:
- **Sequential probes (legacy first, IRM second)**: simpler to
  reason about but adds an extra RTT in the both-installed case.
  Rejected — parallel probes are a one-line `errgroup` and SC-003
  binds the overhead.
- **Lazy selection on first tool call**: violates FR-005 ("fail
  closed") and would let the first tool call fail with a confusing
  error if neither plugin is installed. Rejected.
- **Per-call plugin negotiation**: violates FR-009 (selection is a
  startup decision; no mid-session re-probe). Rejected.

---

## Decision 4 (revised) — Transports, CLI flags, and the new `--plugin-preference`

**Decision**: Inherit the 001 CLI surface. Add one new flag:

| Flag | Env var | Default | Purpose |
|---|---|---|---|
| `--plugin-preference` | `GRAFANA_ONCALL_PLUGIN_PREFERENCE` | `""` (unset) | Force plugin selection: `oncall-app` \| `irm` \| unset |

If both the flag and the env var are set, the **flag wins** (matches
001's flag-wins-over-env convention used for `--read-only` /
`GRAFANA_ONCALL_READ_ONLY`).

**Rationale**: Naming follows the existing kebab-case / env-var
convention. Default unset (= `oncall-app`) makes the legacy plugin
the implicit choice, matching US3's "both installed → prefer legacy"
scenario without forcing operators to opt in.

**Alternatives considered**:
- **`--plugin` (no `preference` suffix)**: shorter but ambiguous
  ("plugin" is a Grafana concept, not an OnCall one). Rejected.
- **Boolean `--use-irm`**: insufficient (doesn't capture the
  "neither installed" diagnostic). Rejected.

---

## Decision 5 (revised) — Configuration & secrets handling

**Decision**: Inherit the 001 env-var surface. Add the new
`GRAFANA_ONCALL_PLUGIN_PREFERENCE` (see Decision 4). The
`config.PluginPreference` field is a typed enum
(`PluginOnCallApp` | `PluginIRM` | `PluginUnset`); its `Unset`
zero-value is mapped to "no preference" so the default behavior is
the both-installed case in 001's "legacy first" order.

**Rationale**: A typed enum (vs. a raw string) makes invalid values
unrepresentable inside the Go process; only the env-var / flag
parser is exposed to user input. The parser returns
`INVALID_CONFIG` on any value outside the enumerated set.

**No changes** to the secret-handling rules; `GRAFANA_ONCALL_PLUGIN_PREFERENCE`
is not a secret.

---

## Decision 6 (revised) — Plugin discovery & startup validation

**Decision**: Replace the single-probe 001 startup check with the
two-probe flow in Decision 3. The probe order is:

1. **Config sanity** (FR-002, FR-052, FR-006): all 001 checks plus
   the new preference parser.
2. **Parallel plugin probes**:
   - `GET {GRAFANA_URL}/api/plugins/grafana-oncall-app/settings`
   - `GET {GRAFANA_URL}/api/plugins/grafana-irm-app/settings`
3. **Plugin selection** (Decision 3 table).
4. **OnCall API reachability** (inherited from 001): once a plugin
   is selected and its base URL resolved, issue
   `GET {onCallApiUrl}/api/v1/users/?perpage=1` to confirm the
   resolved base URL responds.

The new dual-probe adds at most one extra HTTP round-trip vs. the
001 single-probe; SC-003 anchors this overhead at ≤ 100 ms.

**Rationale**:
- Implements FR-003 (revised), FR-004 (revised), FR-005, FR-006,
  FR-007, FR-008, FR-009 with concrete, testable behavior.
- Parallel probes (vs. sequential) keep the worst-case startup
  latency at ~1 RTT, not ~2.
- The preference-wins-over-default rule is deterministic and
  trivial to test (table-driven unit tests over the 9-row matrix
  in Decision 3).

**Alternatives considered**:
- **Lazy probing on first tool call**: violates FR-005. Rejected.
- **Periodic re-probing**: would violate FR-009 (no mid-session
  re-selection) and adds churn. Rejected.
- **Hard-code IRM as the default**: would surprise operators on
  legacy-only Grafanas (the dominant self-hosted case). Rejected.

---

## Decision 13 (revised) — Error envelope

**Decision**: Inherit the 001 error envelope structure. **Rename**
`IRM_PLUGIN_MISSING` to `ONCALL_PLUGIN_MISSING`. The legacy code
remains in the JSON Schema's `enum` for one minor release (the
"deprecation window" mandated by constitution Principle III) and
maps to the same internal error in `internal/oncall/errors.go` —
the server never emits the old name, but the schema accepts it so
old clients that have pinned the value do not break.

Add no new error codes for this feature: the existing
`INVALID_INPUT` (re-used for the "preference names a plugin that
isn't installed" case), `UPSTREAM_UNAVAILABLE` (re-used for any
non-200/non-404 probe response), and `ONCALL_PLUGIN_MISSING` (re-named
from `IRM_PLUGIN_MISSING`) cover the new surface.

**Rationale**:
- The rename is a breaking change but the one-minor-release alias
  keeps it within the constitution's deprecation window.
- The error-hint text for `ONCALL_PLUGIN_MISSING` explicitly names
  both `grafana-oncall-app` and `grafana-irm-app` so an operator
  who sees the error knows both options.

---

## Resolution of "NEEDS CLARIFICATION" candidates

There are no remaining "NEEDS CLARIFICATION" markers in the Technical
Context. The 2026-06-09 user clarification ("support both plugins")
is the only material change vs. 001 and is already resolved in the
spec's Clarifications section; downstream decisions (preference
naming, default value, error code rename) are answered with
reasonable defaults documented above and in the Assumptions section
of `spec.md`.

**Outcome**: Phase 0 complete. Proceeding to Phase 1.
