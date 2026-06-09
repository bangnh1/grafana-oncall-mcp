# Implementation Plan: Grafana OnCall MCP Server

**Branch**: `001-oncall-mcp-server` | **Date**: 2026-06-09 | **Spec**: [`spec.md`](./spec.md)
**Input**: Feature specification from `/specs/001-oncall-mcp-server/spec.md`
**Clarifications applied**:
- Session 2026-06-08 — added alert-group tools; dropped single-schedule detail tool
- Session 2026-06-09 — target plugin is the **legacy** `grafana-oncall-app`, **not** the rebranded `grafana-irm-app`

## Summary

Standalone Model Context Protocol server in Go that lets AI agents query and act
on Grafana OnCall data on a Grafana instance that hosts the legacy
`grafana-oncall-app` plugin. Built on `mark3labs/mcp-go` and the
`grafana/amixr-api-go-client` HTTP client (the same SDK upstream
`grafana/mcp-grafana` ships), but scoped exclusively to OnCall surfaces and
configured to target the legacy plugin (the upstream server only supports
`grafana-irm-app`). Exposes 11 tools: 7 reads (schedules, shifts, current
on-call, teams, users, alert groups), 4 writes (acknowledge / resolve / silence
/ unresolve alert groups), all gated by an opt-in read-only mode. The startup
plugin probe verifies `grafana-oncall-app` is installed and refuses to start
if it isn't, with a clear error pointing to the missing plugin.

## Technical Context

**Language/Version**: Go 1.26.x (`go 1.26.3` — match upstream `mcp-grafana`)
**Primary Dependencies**:
- `github.com/mark3labs/mcp-go v0.46.0` — MCP server SDK
- `github.com/grafana/amixr-api-go-client v0.0.28` — Go client for the legacy `grafana-oncall-app` HTTP API
- `github.com/prometheus/client_golang v1.20.5` — metrics
- `go.opentelemetry.io/otel v1.35.0` — tracing + OTLP exporter
- `github.com/stretchr/testify v1.11.1` — test framework
- `github.com/hashicorp/go-retryablehttp v0.7.8` — transitive; powers `amixr-api-go-client` retries
- `github.com/google/go-querystring v1.0.0` — transitive; powers `amixr-api-go-client` query encoding

**Storage**: N/A — the server is stateless; all state is fetched from Grafana on demand.
**Testing**: stdlib `testing` + `stretchr/testify`; three tiers (unit, contract, integration) — see `research.md` Decision 11.
**Target Platform**: Linux server (binary), macOS dev — multi-arch via BuildKit `BUILDPLATFORM`/`TARGETPLATFORM`.
**Project Type**: Standalone CLI / daemon binary exposing an MCP server.
**Performance Goals**:
- p95 read-tool latency ≤ 2 s under ≤ 10 concurrent requests (FR-040, SC-001, SC-003)
- p95 write-tool latency ≤ 3 s (constitution Principle IV)
- Per-request peak memory ≤ 100 MB (FR-043, constitution Principle IV)
- Outbound HTTP timeout 10 s default, exponential backoff + jitter on 429/5xx, max 3 retries (FR-041, constitution Principle IV)

**Constraints**:
- Credentials only via env vars or secrets manager; never on CLI, never logged (FR-002, FR-051, FR-052, constitution §Security)
- `GRAFANA_URL` MUST be `https://` for non-localhost (FR-052)
- Startup plugin probe MUST verify `grafana-oncall-app` is installed before accepting any tool call (FR-003, FR-004, FR-005, edge case at spec.md:160-163)
- Read-only mode MUST omit write tools from the tool list, not merely error on call (FR-024, SC-007)

**Scale/Scope**:
- Schemas: 1–500 schedules, 1–10 000 shifts, 1–5 000 users, 1–500 teams, up to 100 000 paginated alert groups per Grafana instance (data-model.md §Volume)
- 11 MCP tools total (7 read + 4 write)
- Token format: single Grafana service-account token OR legacy API key

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. Code Quality** — PASS. `golangci-lint` (per `.golangci.yaml`) covers lint + format; `go vet` runs in CI; all exported functions, MCP tool handlers, and DTO structs carry doc comments (per `data-model.md` and `internal/` layout); no `TODO` comments in source.
- **II. Testing Standards** — PASS. Three tiers: unit (`internal/**/*_test.go` + `go test ./internal/...`), contract (`tests/contract/jsonschema_test.go` validates every tool input/output JSON Schema from `contracts/`), integration (`tests/integration/docker-compose.yaml` + `go test -tags integration ./tests/integration/...`); ≥80% coverage on `internal/` enforced via `go test -coverprofile=...` + `go tool cover -func=...` gate (constitution §Testing, `AGENTS.md` commands).
- **III. UX Consistency** — PASS with documented deviation. Tool names follow `<verb>_<resource>` snake_case (e.g., `list_oncall_schedules`, `acknowledge_alert_group`) — preserves upstream `mcp-grafana` parity; tracked as intentional deviation from the constitution's `<resource>_<verb>` default under Complexity Tracking. Schemas share `id`, `cursor`, `limit`, `started_at`/`ended_at` (ISO 8601 UTC), shared error envelope (`contracts/error_envelope.schema.json`).
- **IV. Performance** — PASS. Read p95 ≤ 2 s (FR-040, SC-003); per-request peak memory ≤ 100 MB (FR-043); 10 s outbound timeout + exponential backoff + jitter, max 3 retries (FR-041); 429 surfaced as `UPSTREAM_RATE_LIMITED` with `retryable=true` (FR-042); benchmark suite for top-traffic tools is a follow-up task (deferred to a later feature — see Complexity Tracking).
- **Security & Operational Constraints** — PASS. Tokens read from `GRAFANA_SERVICE_ACCOUNT_TOKEN` / `GRAFANA_API_KEY` env vars (FR-002, config.go); `https://` enforced for non-localhost (FR-052); logging redaction in `internal/obs/logging.go` strips `token`, `apiKey`, `authorization`, `password`; deps scanned via `govulncheck` in CI (constitution §Security).

No FAIL gates. One intentional deviation (tool-naming order) tracked below.

## Project Structure

### Documentation (this feature)

```text
specs/001-oncall-mcp-server/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output (one input/output schema per tool)
│   ├── _defs.schema.json
│   ├── error_envelope.schema.json
│   └── <tool>.{input,output}.schema.json (×11)
├── checklists/          # Quality checklist output
└── tasks.md             # Phase 2 output (NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
cmd/oncall-mcp/
└── main.go                       # CLI entrypoint, transport selection, startup plugin probe

internal/
├── config/
│   └── config.go                 # env-var loader + validation + redaction
├── obs/
│   ├── logging.go                # slog handler with redaction
│   └── otel.go                   # OTel tracer/meter setup
├── oncall/
│   ├── client.go                 # amixr-api-go-client wrapper + plugin discovery (grafana-oncall-app)
│   ├── dtos.go                   # Wire DTOs (Schedule, Shift, User, Team, AlertGroup, …)
│   ├── errors.go                 # Stable error codes + envelope mapping
│   └── retry.go                  # Retry/backoff helpers on top of amixr client
├── server/
│   ├── server.go                 # mcp-go server wrapper + slow-request middleware
│   ├── readonly.go               # Read-only mode gate
│   └── transport.go              # stdio / SSE / streamable-HTTP plumbing
└── tools/
    ├── tools.go                  # ToolRegistry interface + AddOnCallTools entry point
    ├── schedules.go              # list_oncall_schedules, get_oncall_shift, get_current_oncall_users
    ├── teams.go                  # list_oncall_teams
    ├── users.go                  # list_oncall_users
    ├── alert_groups_read.go      # list_alert_groups, get_alert_group
    └── alert_groups_write.go     # acknowledge, resolve, silence, unresolve_alert_group

tests/
├── contract/
│   └── jsonschema_test.go        # Validates every tool input/output against contracts/*.json
├── integration/
│   ├── docker-compose.yaml       # Grafana + grafana-oncall-app + seeded dataset
│   ├── oncall_reads_test.go
│   └── oncall_writes_test.go
└── e2e/
    └── mcp_client_test.go        # End-to-end MCP client driving the server
```

**Structure Decision**: Single Go project (Option 1) with a `cmd/` + `internal/` Go-standard layout. Source root is the repository root, not a `src/` subdir, because Go's `cmd/`/`internal/` convention is canonical and matches upstream `mcp-grafana` (AGENTS.md, `go.mod`). Tests live in `tests/{contract,integration,e2e}/`; unit tests are colocated (`*_test.go`) inside the `internal/` packages they cover.

## Plugin discovery (critical path)

The user clarification flipped the target plugin from `grafana-irm-app` to
`grafana-oncall-app`. The startup probe and the in-process client URL
resolution must be updated accordingly:

- **Probe URL**: `GET {GRAFANA_URL}/api/plugins/grafana-oncall-app/settings` (legacy plugin id).
- **OnCall API base URL**: The legacy `grafana-oncall-app` plugin proxies its
  OnCall HTTP API under `/api/plugins/grafana-oncall-app/resources/api/v1/`
  (i.e. `{GRAFANA_URL}/api/plugins/grafana-oncall-app/resources/api/v1/`).
  Pass this as the `base_url` argument to
  `aapi.NewWithGrafanaURL(baseURL, token, grafanaURL)`; the amixr client
  appends `api/v1/` to its own base, so pass the prefix *without* the trailing
  `api/v1/` and let the client concatenate.
- **Failure mode**: If the probe returns 404 *and* the `grafana-irm-app` plugin
  is detected (probe `grafana-irm-app/settings` returns 200), the startup
  error message MUST explicitly say the server targets the legacy
  `grafana-oncall-app` and the rebranded `grafana-irm-app` is unsupported.
- **Implementation note**: The current `internal/oncall/client.go::resolveOnCallURL`
  and `cmd/oncall-mcp/main.go::startUpHealthCheck` probe `grafana-irm-app` and
  extract `jsonData.onCallApiUrl`. Both must be inverted (probe
  `grafana-oncall-app`, build the resource-proxy URL, and detect IRM to
  surface the "wrong plugin" error).

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

| Violation | Why Needed | Simpler Alternative Rejected Because |
|---|---|---|
| Tool names use `<verb>_<resource>` (e.g., `list_oncall_schedules`, `acknowledge_alert_group`) instead of the constitution's `<resource>_<verb>` default (e.g., `oncall_schedules_list`, `alert_group_acknowledge`). | Preserves naming parity with upstream `grafana/mcp-grafana` so the same prompt invoking the same tool name works against either server. Operators who already use `mcp-grafana` get identical tool names. | Renaming tools to `<resource>_<verb>` would (a) break the user-quoted contract (the 5 tool names the user enumerated), (b) diverge from the upstream server the user explicitly mirrored, and (c) split the prompt-portability goal across the two servers. Follow-up: amend constitution Principle III to permit `<verb>_<resource>` when preserving upstream parity. |
| Benchmark suite for top-traffic tools deferred to a follow-up feature (constitution Principle IV "benchmark suite MUST cover top-traffic tools"). | Scope of v1 is 11 tools with no production telemetry yet; benchmark baselines can only be set against real traffic, and CI-only benchmarks would be unstable. | Inlining benchmarks in this feature would consume task budget without producing actionable signal; will be added in a follow-up once SC-003 / SC-006 are measured against a real Grafana instance. |
| `go.mod` currently declares `go 1.23.0`; constitution and plan target `go 1.26.3` to match upstream `mcp-grafana`. | Upstream parity directive (`dùng technical stack giống như mcp-grafana`). | Bumping to a non-existent toolchain version is blocked by the user's local Go install; resolved by a one-line edit to `go.mod` and `toolchain go1.26.3` in the first implementation task. |
