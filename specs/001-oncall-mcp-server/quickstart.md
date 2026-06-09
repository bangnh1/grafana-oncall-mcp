# Quickstart: Grafana OnCall MCP Server

This guide validates the planned implementation for a dedicated Grafana OnCall MCP server.

## Prerequisites

- Go 1.26.x
- Access to a Grafana Cloud stack or Grafana instance with the legacy
  `grafana-oncall-app` plugin installed
- Grafana service account token with OnCall permissions
- MCP client capable of stdio, SSE, or Streamable HTTP

The rebranded `grafana-irm-app` plugin is intentionally unsupported.
Servers pointed at an instance that only hosts `grafana-irm-app` will
refuse to start with an `ONCALL_PLUGIN_MISSING` error.

## Configuration

Set the Grafana endpoint and one token variable:

```bash
export GRAFANA_URL="https://example.grafana.net"
export GRAFANA_SERVICE_ACCOUNT_TOKEN="glsa_..."
```

`GRAFANA_API_KEY` may be used instead of `GRAFANA_SERVICE_ACCOUNT_TOKEN` for compatibility with upstream `mcp-grafana` configuration conventions.

Optional settings:

```bash
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

Expected MCP capabilities include these read tools:

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

Verify read behavior from an MCP client:

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

## Expected Error Handling

All tool failures return the shared `error_envelope.schema.json` shape.

Common cases:

- Missing token returns `UNAUTHENTICATED`.
- Missing `grafana-oncall-app` plugin (or an instance that only has
  `grafana-irm-app`) returns `ONCALL_PLUGIN_MISSING`.
- Invalid IDs return `NOT_FOUND`.
- Upstream 429 returns `UPSTREAM_RATE_LIMITED` and is marked retryable.
- Write attempts unavailable in read-only mode are prevented by tool registration; if reached internally, return `READ_ONLY_MODE`.

## Performance Acceptance

For a healthy Grafana Cloud region and a page size of 50:

- p95 read-tool latency should stay below 2 seconds, excluding upstream outages.
- p95 write-tool latency should stay below 3 seconds, excluding upstream outages.
- Pagination must avoid unbounded upstream fetches.
