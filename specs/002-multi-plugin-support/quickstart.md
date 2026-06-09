# Quickstart: Grafana OnCall MCP Server (Multi-Plugin)

This guide validates the planned implementation for the dual-plugin Grafana OnCall MCP server.
The server auto-detects whether the target Grafana instance hosts the
legacy `grafana-oncall-app` plugin or the rebranded `grafana-irm-app`
plugin and serves the same 11 tools in either case.

## Prerequisites

- Go 1.26.x
- Access to a Grafana Cloud stack or Grafana instance hosting **at
  least one** of the supported OnCall plugins:
  - `grafana-oncall-app` (the legacy plugin, archived upstream) **OR**
  - `grafana-irm-app` (the rebranded plugin)
- A Grafana service account token with OnCall permissions
- MCP client capable of stdio, SSE, or Streamable HTTP

If the target Grafana hosts **both** plugins, the server picks the
legacy `grafana-oncall-app` by default; use
`GRAFANA_ONCALL_PLUGIN_PREFERENCE=irm` (or `--plugin-preference=irm`)
to force the rebranded plugin.

## Configuration

Set the Grafana endpoint and one token variable:

```bash
export GRAFANA_URL="https://example.grafana.net"
export GRAFANA_SERVICE_ACCOUNT_TOKEN="glsa_..."
```

`GRAFANA_API_KEY` may be used instead of `GRAFANA_SERVICE_ACCOUNT_TOKEN` for compatibility with upstream `mcp-grafana` configuration conventions.

Optional settings:

```bash
# Force plugin selection when both are installed.
# Accepts: oncall-app | irm. Default: unset (= prefer oncall-app).
export GRAFANA_ONCALL_PLUGIN_PREFERENCE="oncall-app"

export GRAFANA_ONCALL_READ_ONLY=true
export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4318"
```

When read-only mode is enabled, write tools are not registered:

- `acknowledge_alert_group`
- `resolve_alert_group`
- `silence_alert_group`
- `unresolve_alert_group`

## Build

```bash
go mod download
go build -o grafana-oncall-mcp ./cmd/oncall-mcp
```

## Run With Stdio

```bash
./grafana-oncall-mcp -transport=stdio
```

The startup log includes one `INFO` line naming the selected plugin
(`plugin=oncall-app` or `plugin=irm`) and, when both plugins are
installed and the legacy is preferred, one `WARN` line explaining
the choice. The resolved OnCall API base URL is also logged with the
token redacted.

Expected MCP capabilities include these read tools (identical for
both plugin paths):

- `list_oncall_schedules`
- `get_oncall_shift`
- `get_current_oncall_users`
- `list_oncall_teams`
- `list_oncall_users`
- `list_alert_groups`
- `get_alert_group`

Expected MCP capabilities also include write tools unless read-only mode is enabled:

- `acknowledge_alert_group`
- `resolve_alert_group`
- `silence_alert_group`
- `unresolve_alert_group`

## Run With SSE

```bash
./grafana-oncall-mcp -transport=sse -addr=:8000
```

Connect the MCP client to:

```text
http://localhost:8000/sse
```

## Run With Streamable HTTP

```bash
./grafana-oncall-mcp -transport=streamable-http -addr=:8000
```

Connect the MCP client to:

```text
http://localhost:8000/mcp
```

## Smoke Tests

Run the unit and contract tests:

```bash
go test ./...
```

Run linting:

```bash
golangci-lint run
```

Verify read behavior from an MCP client (the same smoke flow works
against both plugin paths):

1. List teams with `list_oncall_teams`.
2. List schedules with `list_oncall_schedules`.
3. Pick a schedule and call `get_current_oncall_users`.
4. List active alert groups with `list_alert_groups` and `state=firing`.
5. Pick one alert group and call `get_alert_group`.

Verify read-only behavior:

```bash
GRAFANA_ONCALL_READ_ONLY=true ./grafana-oncall-mcp -transport=stdio
```

The MCP tool list must omit all write tools.

Verify write behavior in a non-production or drill environment:

1. Call `acknowledge_alert_group` for a firing alert group.
2. Call `resolve_alert_group` for the same alert group.
3. Call `unresolve_alert_group` and confirm the group returns to firing or acknowledged state according to upstream OnCall behavior.
4. Call `silence_alert_group` with a short future `until` timestamp and confirm `silenced_until` is returned.

## Dual-Plugin Specific Tests

Verify the auto-detection + preference logic by running the
server against three fixtures (see `tests/integration/docker-compose.yaml`):

| Fixture | Installed plugins | Operator preference | Expected selected plugin |
|---|---|---|---|
| `legacy-only` | `grafana-oncall-app` | unset | `oncall-app` (INFO) |
| `irm-only` | `grafana-irm-app` | unset | `irm` (INFO) |
| `both-installed` | `grafana-oncall-app` + `grafana-irm-app` | unset | `oncall-app` (INFO + WARN) |
| `both-installed` | both | `GRAFANA_ONCALL_PLUGIN_PREFERENCE=irm` | `irm` (INFO) |
| `both-installed` | both | `GRAFANA_ONCALL_PLUGIN_PREFERENCE=oncall-app` | `oncall-app` (INFO) |
| `legacy-only` | `grafana-oncall-app` | `GRAFANA_ONCALL_PLUGIN_PREFERENCE=irm` | startup fails with `INVALID_CONFIG` |
| `none-installed` | (neither) | unset | startup fails with `ONCALL_PLUGIN_MISSING` |

## Expected Error Handling

All tool failures return the shared `error_envelope.schema.json` shape.

Common cases:

- Missing token returns `UNAUTHENTICATED`.
- Neither supported OnCall plugin is installed returns
  `ONCALL_PLUGIN_MISSING`; the hint names both `grafana-oncall-app`
  and `grafana-irm-app`.
- Operator preference names a plugin that is not installed
  returns `INVALID_CONFIG`; the hint names the requested
  preference and the installed plugins.
- Invalid IDs return `NOT_FOUND`.
- Upstream 429 returns `UPSTREAM_RATE_LIMITED` and is marked
  retryable.
- Write attempts unavailable in read-only mode are prevented by
  tool registration; if reached internally, return `READ_ONLY_MODE`.

## Performance Acceptance

For a healthy Grafana Cloud region and a page size of 50:

- p95 read-tool latency should stay below 2 seconds on either
  plugin path, excluding upstream outages.
- p95 write-tool latency should stay below 3 seconds on either
  plugin path, excluding upstream outages.
- The dual-plugin startup probe adds no more than 100 ms of
  overhead vs. the single-plugin build (SC-003).
- Pagination must avoid unbounded upstream fetches.
