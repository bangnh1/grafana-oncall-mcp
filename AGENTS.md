# grafana-oncall-mcp Development Guidelines

Auto-generated from all feature plans. Last updated: 2026-06-09

## Active Technologies

- Go 1.26.x (`go 1.26.3` — match upstream `mcp-grafana`) (001-oncall-mcp-server)
- `github.com/mark3labs/mcp-go` v0.46.0 (MCP server SDK; stdio + SSE + streamable-HTTP) (001-oncall-mcp-server)
- `github.com/grafana/amixr-api-go-client` v0.0.28 (Go client; works against both `grafana-oncall-app` and `grafana-irm-app` HTTP surface) (001-oncall-mcp-server)
- `github.com/prometheus/client_golang` v1.20.5 + `go.opentelemetry.io/otel` v1.35.0 (metrics + tracing) (001-oncall-mcp-server)
- `github.com/stretchr/testify` v1.11.1 (test framework) (001-oncall-mcp-server)

## Project Structure

```text
cmd/oncall-mcp/      # main entrypoint + dual-plugin startup probe
internal/config/     # env-var loader + validation + redaction (incl. plugin preference)
internal/obs/        # slog logger + redaction handler + OTel setup
internal/oncall/     # amixr-api-go-client wrapper + plugin discovery (dual) + retry helpers
internal/server/     # mcp-go server wrapper + read-only mode + slow-request middleware
internal/tools/      # MCP tool registration (schedules, teams, users, alert groups)
tests/contract/      # JSON-schema contract tests (every tool input/output)
tests/integration/   # docker-compose matrix (legacy-only / irm-only / both-installed) + live tests
tests/e2e/           # end-to-end MCP client tests
```

## Commands

```bash
# Build
go build -o bin/grafana-oncall-mcp ./cmd/oncall-mcp

# Unit + contract tests
go test ./internal/... ./tests/contract/...

# Integration tests (require running Grafana + supported OnCall plugin via docker-compose)
go test -tags integration ./tests/integration/...

# Lint + format
golangci-lint run

# Vet
go vet ./...

# Coverage gate (>=80% on internal/)
go test -coverprofile=coverage.out ./internal/...
go tool cover -func=coverage.out | awk '/total:/ {print $3}' | sed 's/%//' | awk '{ if ($1 < 80) exit 1 }'
```

## Code Style

Go 1.26.x (`go 1.26.3` — match upstream `mcp-grafana`): Follow standard conventions
- All exported functions, MCP tool handlers, and structs MUST have doc comments.
- Tool names MUST follow `<verb>_<resource>` snake_case (e.g., `list_oncall_schedules`, `acknowledge_alert_group`) to preserve parity with upstream `mcp-grafana`.
- Error returns to MCP clients MUST use the shared `error_envelope.schema.json` shape with a stable `code` from the enumerated set (current set: `INVALID_INPUT`, `NOT_FOUND`, `UNAUTHENTICATED`, `FORBIDDEN`, `UPSTREAM_RATE_LIMITED`, `UPSTREAM_UNAVAILABLE`, `UPSTREAM_TIMEOUT`, `ONCALL_PLUGIN_MISSING`, `IRM_PLUGIN_MISSING` (deprecated alias), `INTERNAL`, `READ_ONLY_MODE`, `STATE_TRANSITION_REJECTED`).
- Secrets MUST never appear in logs, error messages, or response bodies; redaction happens at the logging boundary in `internal/obs`.
- The startup plugin probe MUST verify **at least one** of `grafana-oncall-app` or `grafana-irm-app` is installed; the server MUST refuse to start otherwise.
- Plugin selection is a startup decision only; the resolved OnCall API base URL MUST be immutable for the process lifetime.
- The new env-var `GRAFANA_ONCALL_PLUGIN_PREFERENCE` and flag `--plugin-preference` accept `oncall-app` | `irm` | unset (default `oncall-app`); the flag wins over the env var.

## Recent Changes

- 002-multi-plugin-support: Server now supports BOTH `grafana-oncall-app` (legacy) AND `grafana-irm-app` (rebranded); auto-detects at startup, honors `GRAFANA_ONCALL_PLUGIN_PREFERENCE` / `--plugin-preference`; error code renamed `IRM_PLUGIN_MISSING` → `ONCALL_PLUGIN_MISSING` (old name kept as a one-minor-release deprecated alias)
- 001-oncall-mcp-server: Added Go 1.26.x (`go 1.26.3` — match upstream `mcp-grafana`)
- 001-oncall-mcp-server: Target plugin is the legacy `grafana-oncall-app` (via `amixr-api-go-client`); 11 MCP tools (7 read + 4 write) gated by `GRAFANA_ONCALL_READ_ONLY` / `--read-only`

<!-- MANUAL ADDITIONS START -->
<!-- MANUAL ADDITIONS END -->
