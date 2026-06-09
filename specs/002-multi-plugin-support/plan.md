# Implementation Plan: Multi-Plugin OnCall Support

**Branch**: `002-multi-plugin-support` (current git branch still `001-oncall-mcp-server`; see Complexity Tracking) | **Date**: 2026-06-09 | **Spec**: [`spec.md`](./spec.md)
**Input**: Feature specification from `/specs/002-multi-plugin-support/spec.md`
**Builds on**: [`specs/001-oncall-mcp-server/`](../../001-oncall-mcp-server/) — this feature extends the single-plugin server to a dual-plugin server.
**Clarifications applied** (from 002 spec Clarifications section):
- Target both `grafana-oncall-app` AND `grafana-irm-app`; auto-detect at startup.
- New operator override `GRAFANA_ONCALL_PLUGIN_PREFERENCE` / `--plugin-preference` with values `oncall-app` | `irm` | unset.
- Default preference is `oncall-app` (legacy); when both plugins are present and no preference is set, the server picks the legacy plugin and logs a `WARN` line.
- Error code renamed `IRM_PLUGIN_MISSING` → `ONCALL_PLUGIN_MISSING`; hint names both plugin IDs.
- Plugin selection is a startup decision only; mid-session re-probe is forbidden.

## Summary

Extend the single-plugin Grafana OnCall MCP server (from feature 001) to
auto-detect and operate against either the legacy `grafana-oncall-app`
plugin or the rebranded `grafana-irm-app` plugin on a given Grafana
instance. The startup probe is rewritten to query both plugin
settings endpoints, select exactly one plugin per an explicit precedence
rule (operator preference > default legacy), and resolve the
OnCall API base URL appropriate to the selected plugin. The 11
MCP tools (7 read + 4 write) are unchanged; the data model and JSON
schemas are unchanged. The 10-line plugin probe and the
plugin-preference config surface are the only new operator-facing
deltas.

## Technical Context

**Language/Version**: Go 1.26.x (`go 1.26.3` — match upstream `mcp-grafana`). Inherited from 001.
**Primary Dependencies**: Identical to 001.
- `github.com/mark3labs/mcp-go v0.46.0` — MCP server SDK
- `github.com/grafana/amixr-api-go-client v0.0.28` — Go client; works against both plugins' HTTP surface
- `github.com/prometheus/client_golang v1.20.5` — metrics
- `go.opentelemetry.io/otel v1.35.0` — tracing
- `github.com/stretchr/testify v1.11.1` — tests

No new dependencies are required. The `amixr-api-go-client` library
already speaks the OnCall URL shape used by both the legacy
`grafana-oncall-app` resource proxy and the rebranded
`grafana-irm-app` (per the upstream `client.go` we audited during
001's planning).

**Storage**: N/A — stateless server; the selected plugin ID + resolved
OnCall API base URL are kept in process memory only. Inherited from 001.
**Testing**: stdlib `testing` + `stretchr/testify`; three tiers (unit,
contract, integration). Integration tier grows from one docker-compose
matrix to a parameterized matrix of three rows (legacy-only, IRM-only,
both-installed). Inherited from 001.
**Target Platform**: Linux server binary, macOS dev. Inherited from 001.
**Project Type**: Standalone CLI / daemon binary exposing an MCP
server. Inherited from 001.

**Performance Goals** (inherited from 001, restated for this feature):
- p95 read-tool latency ≤ 2 s under ≤ 10 concurrent requests (FR-040, SC-003)
- p95 write-tool latency ≤ 3 s
- Per-request peak memory ≤ 100 MB (FR-043)
- Outbound HTTP timeout 10 s default, exponential backoff + jitter, max 3 retries (FR-041)
- **NEW (this feature)**: Dual-plugin detection adds ≤ 100 ms of startup latency vs. the single-plugin build (SC-003).

**Constraints** (inherited from 001, with new this-feature constraints):
- Credentials only via env vars (FR-002, FR-051, FR-052).
- Startup plugin probe MUST verify **at least one** of the two supported
  plugins is installed; the server MUST refuse to start otherwise
  (FR-003 revised, FR-004 revised, FR-005).
- Read-only mode MUST omit write tools from the tool list, not merely
  error on call (FR-024).
- **NEW (this feature)**: Plugin selection is decided exactly once at
  startup; the resolved OnCall API base URL MUST be immutable for the
  lifetime of the process (FR-009).
- **NEW (this feature)**: The error code for "neither plugin installed"
  is `ONCALL_PLUGIN_MISSING` (FR-035) — the prior `IRM_PLUGIN_MISSING`
  code is deprecated; the contract is updated.
- **NEW (this feature)**: The startup log MUST include the selected
  plugin ID at `INFO` and the legacy-preferred-over-IRM warning at
  `WARN` (FR-007, FR-053).

**Scale/Scope**: Inherited from 001 (11 MCP tools; same data volumes).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. Code Quality** — PASS. New public surface is small: a
  `Plugin` enum (`OnCallApp` | `IRM`), a `SelectedPlugin` field on the
  existing `oncall.Client`, two new env-var / flag handlers
  (`GRAFANA_ONCALL_PLUGIN_PREFERENCE`, `--plugin-preference`), and a
  refactored `resolveOnCallURL` that probes two endpoints instead of
  one. All carry doc comments; `golangci-lint` + `go vet` cover
  formatting and static checks; the prior `internal/oncall/client.go`
  doc comments are extended (not replaced) so review history is
  preserved.
- **II. Testing Standards** — PASS. The three-tier test layout
  (unit / contract / integration) is inherited from 001. New unit
  tests cover the dual-probe logic and the preference validator; the
  integration test fixture grows from one docker-compose matrix to
  three parameterized rows. ≥80% coverage on `internal/` remains the
  gate. No new behavior is added to existing tools, so existing
  contract tests for the 11 tools remain valid as-is.
- **III. UX Consistency** — PASS. Tool names and JSON schemas are
  unchanged from 001 (US4 explicitly asserts operator prompts need no
  rewrite). The new `GRAFANA_ONCALL_PLUGIN_PREFERENCE` env-var name
  follows the existing `GRAFANA_ONCALL_*` prefix convention; the
  `--plugin-preference` flag uses the same `--kebab-case` style as
  `--read-only`. The error code change `IRM_PLUGIN_MISSING` →
  `ONCALL_PLUGIN_MISSING` is a breaking change to the error envelope;
  the prior code remains a valid alias in the schema for one minor
  release (deprecation window per constitution Principle III).
- **IV. Performance** — PASS. p95 ≤ 2 s, <100 MB/request, 10 s
  timeout, exponential backoff + jitter, max 3 retries all inherited
  from 001. The dual-probe adds at most one extra `GET /settings`
  round-trip (~50–150 ms) at startup, which is not in the per-request
  hot path. SC-003 anchors the additional overhead to ≤ 100 ms. The
  benchmark-suite gap is inherited (deferred to a follow-up feature).
- **Security & Operational Constraints** — PASS. No new secret
  surface; the new `GRAFANA_ONCALL_PLUGIN_PREFERENCE` value is a
  non-secret enum (`oncall-app` | `irm`). Logging redaction and
  dependency scanning remain covered by the existing infrastructure.

No FAIL gates. One intentional UX deviation (error-code breaking
change with deprecation alias) is tracked below; the previous
deviation (tool-naming order) is inherited.

## Project Structure

### Documentation (this feature)

```text
specs/002-multi-plugin-support/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output (one input/output schema per tool)
│   ├── _defs.schema.json
│   ├── error_envelope.schema.json (updated: ONCALL_PLUGIN_MISSING + legacy IRM_PLUGIN_MISSING alias)
│   └── <tool>.{input,output}.schema.json (×11, unchanged from 001)
├── checklists/
│   └── requirements.md
└── tasks.md             # Phase 2 output (NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
cmd/oncall-mcp/
└── main.go                       # + startUpHealthCheck rewritten for dual probe
                                  # + --plugin-preference flag

internal/
├── config/
│   └── config.go                 # + GRAFANA_ONCALL_PLUGIN_PREFERENCE parser
│                                  # + PluginPreference type
├── obs/
│   ├── logging.go                # (unchanged)
│   └── otel.go                   # (unchanged)
├── oncall/
│   ├── client.go                 # !! rewrite resolveOnCallURL → probeBoth; introduce
│                                  #    SelectedPlugin (enum) on Client; keep
│                                  #    onCallClient field unchanged in type
│   ├── plugin.go (NEW)           #   - Plugin enum, PluginPreference parser,
│                                  #     preference validator, precedence logic
│   ├── dtos.go                   # (unchanged)
│   ├── errors.go                 # ! ONCALL_PLUGIN_MISSING replaces IRM_PLUGIN_MISSING;
│                                  #   legacy code kept as a deprecated alias
│   └── retry.go                  # (unchanged)
├── server/
│   ├── server.go                 # (unchanged)
│   ├── readonly.go               # (unchanged)
│   └── transport.go              # (unchanged)
└── tools/                        # (unchanged — same 11 tool files from 001)

tests/
├── contract/
│   └── jsonschema_test.go        # ! updated: error envelope enum now includes
│                                  #   ONCALL_PLUGIN_MISSING; the legacy code is
│                                  #   asserted as a still-accepted alias
├── integration/
│   ├── docker-compose.yaml       # ! parameterized: 3 service matrices
│   ├── oncall_reads_test.go      # ! matrix-driven: 3 sub-tests
│   └── oncall_writes_test.go     # ! matrix-driven: 3 sub-tests
└── e2e/                          # (unchanged)
```

**Structure Decision**: Single Go project (Option 1). The dual-plugin
support is implemented as a small additive change inside the existing
`internal/oncall/` package (new `plugin.go`, modified `client.go`) plus
a new env-var and flag in `internal/config/`. No new top-level
directories.

## Plugin discovery (critical path)

The new dual-plugin probe flow:

```
                  ┌──────────────────────────────────────┐
                  │  startUpHealthCheck (main.go)         │
                  │  + resolveOnCallURL (oncall/client.go)│
                  └──────────────────────────────────────┘
                                    │
                                    ▼
        ┌────────────────────────────────────────────────────┐
        │  1. Parse GRAFANA_ONCALL_PLUGIN_PREFERENCE         │
        │     → oncall-app | irm | unset (default oncall-app)│
        └────────────────────────────────────────────────────┘
                                    │
                                    ▼
        ┌────────────────────────────────────────────────────┐
        │  2. Probe grafana-oncall-app/settings (HTTP GET)    │
        │     200 → legacy plugin is installed               │
        │     404 → legacy plugin is NOT installed           │
        │     other → UPSTREAM_UNAVAILABLE                   │
        └────────────────────────────────────────────────────┘
                                    │
                                    ▼
        ┌────────────────────────────────────────────────────┐
        │  3. Probe grafana-irm-app/settings (HTTP GET)      │
        │     200 → IRM plugin is installed                  │
        │     404 → IRM plugin is NOT installed              │
        │     other → UPSTREAM_UNAVAILABLE                   │
        └────────────────────────────────────────────────────┘
                                    │
                                    ▼
        ┌────────────────────────────────────────────────────┐
        │  4. Select one plugin per precedence:              │
        │     a. If preference is set and the matching       │
        │        plugin is installed → use that plugin.      │
        │     b. If preference is set and the matching       │
        │        plugin is NOT installed → INVALID_CONFIG.   │
        │     c. If preference is unset and both are          │
        │        installed → prefer oncall-app, log WARN.    │
        │     d. If preference is unset and only one is      │
        │        installed → use that one, log INFO.         │
        │     e. If neither is installed → ONCALL_PLUGIN_    │
        │        MISSING with hint naming both plugin IDs.   │
        └────────────────────────────────────────────────────┘
                                    │
                                    ▼
        ┌────────────────────────────────────────────────────┐
        │  5. Resolve OnCall API base URL per the selected   │
        │     plugin:                                        │
        │       oncall-app → {GRAFANA_URL}/api/plugins/      │
        │                     grafana-oncall-app/resources/   │
        │                     api/v1/                        │
        │       irm       → jsonData.onCallApiUrl from       │
        │                     /api/plugins/grafana-irm-app/  │
        │                     settings, with api/v1/         │
        │                     appended                       │
        └────────────────────────────────────────────────────┘
                                    │
                                    ▼
        ┌────────────────────────────────────────────────────┐
        │  6. aapi.NewWithGrafanaURL(baseURL, token,         │
        │     grafanaURL) — pass the resolved base URL;      │
        │     the amixr client appends `api/v1/` itself      │
        │     so pass WITHOUT the trailing `api/v1/`.        │
        └────────────────────────────────────────────────────┘
                                    │
                                    ▼
        ┌────────────────────────────────────────────────────┐
        │  7. Log selected plugin at INFO; if "both present, │
        │     legacy preferred" log a WARN line.             │
        └────────────────────────────────────────────────────┘
```

The implementation lives in `internal/oncall/plugin.go` (new file) and
`internal/oncall/client.go::resolveOnCallURL` (rewritten to call into
`plugin.go`).

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

| Violation | Why Needed | Simpler Alternative Rejected Because |
|---|---|---|
| Error code `IRM_PLUGIN_MISSING` → `ONCALL_PLUGIN_MISSING` is a breaking change to the error envelope (constitution Principle III requires a deprecation window of at least one minor release). | The new code name is the only one that survives a future where neither plugin is hard-coded; the old name would lie about the contract. | Keeping `IRM_PLUGIN_MISSING` would force the server to emit the old name when only `grafana-oncall-app` is installed, which contradicts the spec's FR-003/FR-004/FR-035 and would surprise operators. Mitigation: `IRM_PLUGIN_MISSING` is accepted as a deprecated alias in the JSON Schema for one minor release; new tests assert both names pass validation. |
| Tool names use `<verb>_<resource>` (e.g., `list_oncall_schedules`, `acknowledge_alert_group`) instead of the constitution's `<resource>_<verb>` default. *(inherited from 001.)* | Preserves naming parity with upstream `grafana/mcp-grafana` so the same prompt invoking the same tool name works against either server. | Renaming tools to `<resource>_<verb>` would (a) break the user-quoted contract (the 5 tool names the user enumerated), (b) diverge from the upstream server the user explicitly mirrored, and (c) split the prompt-portability goal across the two servers. |
| Benchmark suite for top-traffic tools deferred to a follow-up feature. *(inherited from 001.)* | Scope of v1 is 11 tools with no production telemetry yet; benchmark baselines can only be set against real traffic. | Inlining benchmarks in this feature would consume task budget without producing actionable signal. |
| `go.mod` currently declares `go 1.23.0`; constitution and plan target `go 1.26.3` to match upstream `mcp-grafana`. *(inherited from 001; carried over to 002.)* | Upstream parity directive. | Will be resolved in a one-line edit to `go.mod` and `toolchain go1.26.3` in the first implementation task. |
| `git branch` is still `001-oncall-mcp-server` even though the spec directory is `002-multi-plugin-support` (a `before_plan` hook would normally create a fresh branch). | The user invoked `/speckit.plan` directly without a `before_specify` / `before_plan` hook firing, so no new branch was created. | The branch / directory decoupling is documented in `.specify/feature.json`; downstream `/speckit.tasks` will continue to operate against the spec dir, not the branch. The branch can be renamed (`git branch -m 002-multi-plugin-support`) as part of the first implementation task if the operator prefers aligned names. |
