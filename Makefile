build:
	go build -o grafana-oncall-mcp ./cmd/oncall-mcp

test:
	go test ./...

test-unit:
	go test -tags unit ./...

test-integration:
	go test -tags integration ./...

# Dual-plugin integration tests run against the IRM-only, legacy-only,
# and both-installed docker-compose matrices (spec 002-multi-plugin-support).
# The legacy and both matrices are skipped if the corresponding plugin
# is not present on the host's Grafana install; the IRM matrix is the
# default since it matches the most common production setup.
test-integration-irm:
	GRAFANA_URL=$${GRAFANA_URL:-http://localhost:3000} docker compose -f tests/integration/docker-compose.yaml up -d --build
	GRAFANA_URL=http://localhost:3000 go test -tags integration ./tests/integration/... -run TestDualPlugin -count=1
	docker compose -f tests/integration/docker-compose.yaml down

test-integration-legacy:
	GRAFANA_URL=$${GRAFANA_URL:-http://localhost:3000} docker compose -f tests/integration/docker-compose.legacy.yaml up -d --build
	GRAFANA_URL=http://localhost:3000 GRAFANA_ONCALL_PLUGIN_PREFERENCE=oncall-app go test -tags integration ./tests/integration/... -run TestDualPlugin -count=1
	docker compose -f tests/integration/docker-compose.legacy.yaml down

test-integration-both:
	GRAFANA_URL=$${GRAFANA_URL:-http://localhost:3000} docker compose -f tests/integration/docker-compose.both.yaml up -d --build
	GRAFANA_URL=http://localhost:3000 go test -tags integration ./tests/integration/... -run TestDualPlugin -count=1
	docker compose -f tests/integration/docker-compose.both.yaml down

lint:
	golangci-lint run

run:
	./grafana-oncall-mcp -transport=stdio

run-sse:
	./grafana-oncall-mcp -transport=sse -address=:8000

build-image:
	docker build -t grafana-oncall-mcp:latest .

# -----------------------------------------------------------------------------
# Docker Compose wrappers (used by MCP clients — see opencode.json,
# .vscode/mcp.json, .claude/claude_desktop_config.json, and
# docs/mcp-clients.md).
# -----------------------------------------------------------------------------

# Build the docker-compose image once so the first MCP client
# invocation doesn't pay the Go build cost.
compose-build:
	docker compose build mcp-stdio

# Run the stdio service as a one-shot. The container exits when
# the caller disconnects stdin/stdout; this is the typical pattern
# for editor-integrated MCP clients.
compose-run-stdio:
	docker compose run --rm mcp-stdio

# Bring the streamable-HTTP server up in the background; useful
# when multiple MCP clients (or multiple opencode sessions) want
# to share a single MCP server process.
compose-up:
	docker compose --profile http up -d

compose-down:
	docker compose --profile http down

compose-logs:
	docker compose --profile http logs -f

# Bring up the SSE server (legacy client compatibility).
compose-up-sse:
	docker compose --profile sse up -d

compose-down-sse:
	docker compose --profile sse down

.PHONY: build test test-unit test-integration test-integration-irm test-integration-legacy test-integration-both lint run run-sse build-image compose-build compose-run-stdio compose-up compose-down compose-logs compose-up-sse compose-down-sse
