# Phase 0 Research — Grafana OnCall MCP Server

**Branch**: `001-oncall-mcp-server` · **Date**: 2026-06-08 (updated 2026-06-09)
**Source spec**: [`spec.md`](./spec.md) · **Plan**: [`plan.md`](./plan.md)

This document records the research decisions that resolve every
"NEEDS CLARIFICATION" candidate from the Technical Context. The user
directive — *"dùng technical stack giống như mcp-grafana"* ("use the same
technical stack as `mcp-grafana`") — pins the bulk of the choices to the
upstream project; remaining choices are local concerns (project layout,
test wiring, OnCall write semantics). The 2026-06-09 update flips the
target plugin from `grafana-irm-app` to the legacy `grafana-oncall-app`
per the user clarification in `spec.md` Session 2026-06-09.

Inputs that informed these decisions:

- The `grafana/mcp-grafana` `go.mod`, `Makefile`, `.golangci.yaml`,
  `Dockerfile`, `cmd/mcp-grafana/main.go`, and `tools/oncall.go` (read directly
  from the upstream repo's `main` branch on 2026-06-08).
- The user's enumeration of the 5 OnCall tools the upstream project ships,
  plus the option-B clarification adding alert-group reads and writes.
- The 2026-06-09 clarification that the target plugin is the legacy
  `grafana-oncall-app`, not the rebranded `grafana-irm-app`.
- The project constitution at `.specify/memory/constitution.md` (v1.0.0).

---

## Decision 1 — Implementation language & runtime

**Decision**: Go 1.26.x (mirror `go 1.26.3` from upstream `go.mod`). Single
static binary, `CGO_ENABLED=0`, multi-arch via `BUILDPLATFORM`/`TARGETPLATFORM`
BuildKit args.

**Rationale**:
- Upstream `mcp-grafana` is Go, and the user explicitly asked us to mirror it.
- The Grafana OnCall HTTP client (`grafana/amixr-api-go-client`) and the
  best-maintained MCP server SDK with stdio + SSE + streamable-HTTP
  transports (`mark3labs/mcp-go`) are both Go.
- Static binaries deploy trivially as MCP servers (one command, no runtime).

**Alternatives considered**:
- **Python** (FastMCP / `mcp` SDK): faster to prototype, but would force a
  full reimplementation of the OnCall client and break the requested
  stack-parity. Rejected.
- **Node/TypeScript**: same parity problem; also weaker upstream OnCall
  tooling. Rejected.
- **Rust** (`rmcp`): viable but no benefit over Go here and no upstream
  parity. Rejected.

---

## Decision 2 — MCP server library

**Decision**: `github.com/mark3labs/mcp-go v0.46.0`.

**Rationale**:
- Exact version used by upstream `mcp-grafana`.
- Ships first-class implementations of the three MCP transports we need
  (`server.NewStdioServer`, `server.NewSSEServer`,
  `server.NewStreamableHTTPServer`), all wired the same way upstream does.
- Supports `server.WithHooks(...)` for cross-cutting concerns (slow-request
  logging, metrics, redaction) without intercepting every handler.

**Alternatives considered**:
- **`modelcontextprotocol/go-sdk`**: official-leaning but lower adoption and
  no parity with upstream; rejected.
- **Hand-rolled JSON-RPC over stdio**: rejected — duplicates well-tested
  code and complicates SSE/streamable-HTTP.

---

## Decision 3 — Grafana OnCall HTTP client

**Decision**: `github.com/grafana/amixr-api-go-client v0.0.28` for OnCall
resource calls. Plugin discovery (resolving the OnCall API URL from the
`grafana-oncall-app` plugin settings) uses an `http.Client` constructed by a
local `BuildTransport()` helper that mirrors the one in
`mcpgrafana.BuildTransport` (sets `User-Agent`, attaches the
Grafana service-account bearer token, applies the standard timeouts).

**Rationale**:
- The `amixr-api-go-client` is the official Go SDK for the legacy
  `grafana-oncall-app` HTTP API (its package history is "amixr" → "oncall" →
  "IRM" — the library's typed services predate the IRM rebrand and talk the
  legacy OnCall URL shape).
- The same library covers 100% of our v1 tool surface
  (`ScheduleService`, `OnCallShiftService`, `UserService`, `TeamService`,
  `AlertGroupService`).
- Library parity with the upstream `mcp-grafana` server (same module, same
  version) is preserved — only the **plugin ID** used for URL resolution
  differs, and that lives in a 30-line helper.

**Plugin discovery (legacy `grafana-oncall-app`)**:
- Probe URL: `GET {GRAFANA_URL}/api/plugins/grafana-oncall-app/settings`.
- The OnCall API for the legacy plugin is exposed as a Grafana resource
  proxy at `{GRAFANA_URL}/api/plugins/grafana-oncall-app/resources/api/v1/`.
  This is the URL passed as `base_url` to
  `aapi.NewWithGrafanaURL(baseURL, token, grafanaURL)`; the amixr client
  appends `api/v1/` itself, so the caller passes the prefix *without* a
  trailing `api/v1/`.
- If the probe returns 404 *and* a follow-up probe of
  `/api/plugins/grafana-irm-app/settings` returns 200, the server emits
  `ONCALL_PLUGIN_MISSING` and refuses to start (the operator has installed
  the rebranded plugin; we explicitly do not support it).
- If both probes 404, emit `ONCALL_PLUGIN_MISSING` with a hint to install
  `grafana-oncall-app`.

**Alternatives considered**:
- Direct HTTP calls with `net/http`: more code to maintain, no pagination
  helpers, no model structs. Rejected.
- `grafana/grafana-openapi-client-go` alone: covers Grafana core but not the
  OnCall plugin endpoints. Kept only for `BuildTransport()` parity.

---

## Decision 4 — Transports and CLI flag parsing

**Decision**: Support all three transports (stdio, SSE, streamable-HTTP)
behind a single `--transport` flag with `stdio` as the default. CLI parsing
uses the standard-library `flag` package (no cobra, no pflag) — exact match
with upstream.

**Rationale**:
- Spec FR-001 requires stdio at minimum; Assumptions §3 marks SSE/HTTP as
  nice-to-have. Implementing all three at v1 is cheap when `mark3labs/mcp-go`
  provides them and matches the operator expectations set by upstream.
- The stdlib `flag` package is enough — no subcommands needed; matches
  upstream style; one fewer dependency.

**Flag surface (v1)**:

| Flag | Default | Purpose |
|---|---|---|
| `-t`, `--transport` | `stdio` | `stdio` \| `sse` \| `streamable-http` |
| `--address` | `localhost:8000` | Listen address for sse/streamable-http |
| `--base-path` | `""` | SSE base path |
| `--endpoint-path` | `/` | Streamable-HTTP endpoint |
| `--read-only` | `false` | Suppress every write tool (FR-024) |
| `--log-level` | `info` | `debug` \| `info` \| `warn` \| `error` |
| `--debug` | `false` | Verbose HTTP request/response logging |
| `--metrics` | `false` | Enable `/metrics` endpoint |
| `--metrics-address` | `""` | Separate metrics listener (empty = same as main) |
| `--slow-request-threshold` | `0` | Log when a request exceeds this Go duration |
| `--slow-request-log-level` | `warn` | `info` or `warn` |
| `--session-idle-timeout-minutes` | `30` | SSE/streamable-http only |
| `-version` | n/a | Print version + exit |

**Alternatives considered**:
- **Cobra**: heavier, generates a help system we don't need at v1. Rejected.
- **`spf13/pflag`**: nicer long-flag handling but no parity benefit. Rejected.

---

## Decision 5 — Configuration & secrets handling

**Decision**: Configuration is read from environment variables only
(no CLI flags for secrets, no config file at v1). Required variables:

| Variable | Purpose | Required |
|---|---|---|
| `GRAFANA_URL` | Grafana base URL (`https://...`) | Yes |
| `GRAFANA_SERVICE_ACCOUNT_TOKEN` | Service-account token (preferred) | One of |
| `GRAFANA_API_KEY` | Legacy API key | One of |
| `GRAFANA_ONCALL_READ_ONLY` | `true`/`false` — same as `--read-only` | No |
| `GRAFANA_ONCALL_HTTP_TIMEOUT` | Go duration for outbound calls | No (default `10s`) |
| `GRAFANA_ONCALL_MAX_RETRIES` | int 0–5 | No (default `3`) |
| `OTEL_*` | Standard OTel exporter config | No |

The redacted `config.String()` implementation prints only the URL and a
fingerprint of the token (first 4 chars + length), never the full secret.

**Rationale**:
- Spec FR-002 forbids credentials on the CLI or in logs.
- Constitution §Security & Operational Constraints forbids committing or
  logging secrets.
- Upstream uses the same `GRAFANA_URL` / `GRAFANA_SERVICE_ACCOUNT_TOKEN`
  variable names — operators flipping between the two servers don't have to
  relearn anything.

**Alternatives considered**:
- A YAML/TOML config file: useful for many instances, but Assumptions §6
  pins us to one Grafana instance per server; adds parser surface. Deferred.
- Reading secrets from a file path (Docker/K8s secrets): can be done by the
  operator with `$(cat /run/secrets/token)` patterns; no special support
  needed at v1.

---

## Decision 6 — Plugin discovery & startup validation

**Decision**: On startup the server performs three checks in order, and
exits non-zero (with a structured error written to stderr) if any fails:

1. **Config sanity**: `GRAFANA_URL` set and HTTPS for non-local hosts
   (FR-052); exactly one of `GRAFANA_SERVICE_ACCOUNT_TOKEN` /
   `GRAFANA_API_KEY` present.
2. **OnCall plugin probe**:
   `GET {GRAFANA_URL}/api/plugins/grafana-oncall-app/settings`. The legacy
   plugin exposes its OnCall HTTP API at
   `{GRAFANA_URL}/api/plugins/grafana-oncall-app/resources/api/v1/`; that URL
   (without the trailing `api/v1/` — the amixr client appends it) is what
   the rest of the server uses. If the probe is 404 *and* a follow-up
   `GET {GRAFANA_URL}/api/plugins/grafana-irm-app/settings` returns 200, the
   error message explicitly says "the rebranded `grafana-irm-app` plugin is
   not supported — install `grafana-oncall-app`".
3. **OnCall API reachability**: `GET {onCallApiUrl}/api/v1/users/?perpage=1`
   with the configured token. Expect HTTP 200/204 within the configured
   timeout.

If all three pass, the server starts and serves tools. Subsequent OnCall API
failures do *not* re-trigger plugin probing; they are surfaced per-call via
the structured error envelope.

**Rationale**:
- Implements FR-003, FR-004, FR-005 with concrete checks rather than
  hand-wavy "verify on startup" language.
- Failing fast is operationally clearer than letting the first tool call
  fail mysteriously.
- The dual probe (`grafana-oncall-app` first, `grafana-irm-app` as a
  diagnostic follow-up) lets us produce an actionable error in both the
  "nothing installed" and "wrong plugin installed" cases.

**Alternatives considered**:
- Lazy probing on first tool call: violates "fail closed" (FR-005). Rejected.
- Periodic re-probing: unnecessary churn in a long-running server; the
  per-call error envelope already covers plugin-disappearance. Deferred.

---

## Decision 7 — Tool naming convention

**Decision**: Use upstream `mcp-grafana`'s `<verb>_<resource>` snake_case
naming for all 11 tools:

| Tool | Type | Source of authority |
|---|---|---|
| `list_oncall_schedules` | read | upstream parity (user-enumerated) |
| `get_oncall_shift` | read | upstream parity (user-enumerated) |
| `get_current_oncall_users` | read | upstream parity (user-enumerated) |
| `list_oncall_teams` | read | upstream parity (user-enumerated) |
| `list_oncall_users` | read | upstream parity (user-enumerated) |
| `list_alert_groups` | read | upstream parity |
| `get_alert_group` | read | upstream parity |
| `acknowledge_alert_group` | write | new (this server) |
| `resolve_alert_group` | write | new (this server) |
| `silence_alert_group` | write | new (this server) |
| `unresolve_alert_group` | write | new (this server) |

**Rationale**:
- User-quoted names are exactly the 5 upstream names; option B retained
  alert-group ops, so the alert-group tools must follow the same convention
  for cross-server prompt portability.
- Documented as an intentional deviation from constitution Principle III in
  the plan's Complexity Tracking, with a follow-up to amend the constitution.

**Alternatives considered**:
- **`<resource>_<verb>`** (constitution-compliant: `oncall_schedules_list`,
  `alert_group_acknowledge`): rejected — breaks the user contract on tool
  names and the parity directive.
- **Mixed** (reads `<verb>_<resource>`, writes `<resource>_<verb>`):
  rejected as worse than either pure rule.

---

## Decision 8 — Write tools: idempotency & semantics

**Decision**: Each write tool is an HTTP POST to the corresponding
`amixr-api-go-client` action endpoint and is treated as **idempotent at the
target state**. The contract for each:

| Tool | Maps to | Idempotency | On "already in target state" |
|---|---|---|---|
| `acknowledge_alert_group` | `POST /alert_groups/{id}/acknowledge/` | Yes | Returns success with `was_already_in_state=true` |
| `resolve_alert_group` | `POST /alert_groups/{id}/resolve/` | Yes | Same |
| `silence_alert_group` | `POST /alert_groups/{id}/silence/` (with `delay`) | No (delay restarts) | Returns updated silence end timestamp |
| `unresolve_alert_group` | `POST /alert_groups/{id}/unresolve/` | Yes | Same |

Each write tool's output includes the alert-group ID, the new state, the
acting user (resolved from token), and a timestamp. The tool description
explicitly tells the calling agent that **destructive confirmation is the
client's responsibility** (Assumptions §4).

**Rationale**:
- Idempotent acks/resolves mean agents can safely retry on transient
  network failures without double-acknowledging.
- `silence` is necessarily non-idempotent (the delay parameter changes the
  silence window) — the response makes that explicit so the agent can
  reason about it.

**Alternatives considered**:
- 409-on-already-acknowledged: more "REST-pure" but worse UX for agents
  retrying; user would have to special-case each transient retry.
- Single `set_alert_group_state` tool with a `state` parameter: would map
  poorly onto the OnCall HTTP API (separate endpoints per action) and
  conceals which write is happening from the agent. Rejected.

---

## Decision 9 — Read-only mode

**Decision**: Read-only mode is controlled by `--read-only` /
`GRAFANA_ONCALL_READ_ONLY=true`. When enabled, the four write tools
(`acknowledge_…`, `resolve_…`, `silence_…`, `unresolve_…`) are
**never registered** with the MCP server — they do not appear in `tools/list`
results.

**Rationale**:
- Spec FR-024 + SC-007 require "absent rather than present-but-erroring",
  which is naturally implemented by skipping registration.
- Mirrors how upstream `mcp-grafana` implements its `--disable-write` flag.

---

## Decision 10 — Observability stack

**Decision**: Structured logging via `log/slog` with a custom handler that
redacts known-secret keys (`token`, `apiKey`, `authorization`, `password`)
before emission. Tracing + metrics via OpenTelemetry (same package versions
as upstream). Metrics are off by default and surface on a separate
`/metrics` endpoint when `--metrics` is set (mirrors upstream).

Per-tool spans (`tool=<name>`, `duration_ms`, `outcome`, `error_code`)
satisfy FR-050 and feed the slow-request hook (`--slow-request-threshold`).

**Rationale**:
- Upstream parity (same OTel package set, same slog usage).
- `sloglint` (already in upstream `.golangci.yaml`) prevents accidental
  package-level logger use, which keeps redaction enforceable.

**Alternatives considered**:
- `zap` / `zerolog`: faster but not used upstream, and slog is now the
  Go-standard. Rejected.
- Logs-as-traces only: violates FR-050's structured-log requirement.

---

## Decision 11 — Testing tiers & coverage gating

**Decision**: Three test tiers (matching constitution Principle II):

1. **Unit** (`-tags unit`): mock the `amixr-api-go-client` services with
   stretchr/testify mocks. Cover happy paths + every error envelope branch.
2. **Contract**: JSON Schemas (one per tool input + output, generated via
   `invopop/jsonschema`) checked into `contracts/` and validated against
   live handler output in `tests/contract/jsonschema_test.go`. Also includes
   `error_envelope.schema.json` to guarantee FR-033 compliance.
3. **Integration** (`-tags integration`): `tests/integration/docker-compose.yaml`
   starts Grafana with `grafana-oncall-app` (legacy plugin) and a seeded
   dataset; tests exercise every tool against the real plugin.

Coverage is enforced at ≥80% on the `internal/` tree via a CI step:

```bash
go test -coverprofile=coverage.out -tags unit ./internal/...
go tool cover -func=coverage.out | awk '/total:/ {print $3}' | \
  sed 's/%//' | awk '{ if ($1 < 80) { exit 1 } }'
```

**Rationale**:
- Three tiers map 1:1 onto the constitution requirement.
- Generated contracts double as user-facing documentation of every tool's
  shape.

**Alternatives considered**:
- Skip contract tests: violates constitution. Rejected.
- Live-only integration (no docker-compose): non-portable for CI.

---

## Decision 12 — Pagination

**Decision**: Adopt a single, opaque `cursor` string parameter shared by all
list-style tools, plus a `limit` parameter (default 50, max 200). When the
upstream OnCall API returns a `next` URL, the server base64-encodes it as
the cursor; the next call decodes it and forwards. Empty cursor means
"first page". Responses include `next_cursor` (omitted when the list is
exhausted).

**Rationale**:
- Spec FR-032 + FR-034 require consistent shared parameter names.
- Opaque cursor insulates agents from changes in the upstream API's
  pagination shape.

**Alternatives considered**:
- Page-number pagination: simpler but breaks if the upstream switches to
  cursor-only later. Rejected.
- Stream all pages: violates the performance budget (FR-040) for large
  alert-group lists.

---

## Decision 13 — Error envelope

**Decision**: Every tool error returns:

```json
{
  "code": "STABLE_SNAKE_CASE",
  "message": "Human-readable explanation",
  "tool": "list_alert_groups",
  "hint": "Optional actionable hint, e.g. 'Token missing scope grafana-oncall-app.alert-groups:read'",
  "retryable": false
}
```

Stable codes (initial set): `INVALID_INPUT`, `NOT_FOUND`, `UNAUTHENTICATED`,
`FORBIDDEN`, `UPSTREAM_RATE_LIMITED`, `UPSTREAM_UNAVAILABLE`,
`UPSTREAM_TIMEOUT`, `ONCALL_PLUGIN_MISSING`, `INTERNAL`, `READ_ONLY_MODE`,
`STATE_TRANSITION_REJECTED`.

**Rationale**:
- Implements FR-033 and SC-006 with concrete codes that tests can assert
  against.
- `retryable` lets agents decide whether to back off vs. surface to the user
  immediately.

---

## Resolution of "NEEDS CLARIFICATION" candidates

There are no remaining "NEEDS CLARIFICATION" markers in the Technical
Context. The two spec-level questions that *could* have become markers —
"which write tools?" (answered in `/speckit.clarify` as option B) and
"which tech stack?" (answered by the current user input mirroring
`mcp-grafana`) — are both resolved.

**Outcome**: Phase 0 complete. Proceeding to Phase 1.
