# grafana-oncall-mcp

A dedicated Grafana OnCall MCP (Model Context Protocol) server written in Go.

## Overview

The upstream `mcp-grafana` project targets Grafana Cloud and no longer supports `grafana-oncall-app`, which is archived and deprecated upstream. This MCP server exists to maintain compatibility for **self-hosted Grafana OnCall** instances that still run `grafana-oncall-app` or its rebranded successor `grafana-irm-app`. If you are using **Grafana Cloud**, use the official [`mcp-grafana`](https://github.com/grafana/mcp-grafana) from Grafana instead.

## Quick Start (Recommended: `uvx`)

The fastest way to use this server is via `uvx`, which downloads a
small Python wrapper plus the pre-compiled Go binary from PyPI on
first invocation (~50MB total) and caches them. Requires [uv](https://docs.astral.sh/uv/).

```bash
# Install uv if you don't have it yet
curl -LsSf https://astral.sh/uv/install.sh | sh

# Smoke test
uvx --from grafana-oncall-mcp grafana-oncall-mcp -version
# grafana-oncall-mcp 0.1.0
```

## Prerequisites

- For `uvx`: [uv](https://docs.astral.sh/uv/) (Python package manager; handles the Python wrapper + Go binary).
- For Docker: a working Docker daemon.
- For source builds: Go 1.26.x.
- Access to a **self-hosted Grafana** instance with **at least one** supported OnCall plugin installed:
  - `grafana-oncall-app` (legacy, archived upstream) **or**
  - `grafana-irm-app` (rebranded successor)
- Grafana service account token with OnCall permissions.
- MCP client capable of stdio, SSE, or Streamable HTTP.

## Configuration

Use direct OnCall API mode to call the OnCall Application API directly, bypassing the Grafana plugin resource proxy. This is the recommended mode for self-hosted instances.

```bash
export GRAFANA_ONCALL_API_URL="https://oncall-prod-us-central-0.grafana.net/oncall"
export GRAFANA_ONCALL_API_KEY="oncall_user_scoped_key"
```

Get these values from `OnCall -> Settings`:
- `GRAFANA_ONCALL_API_URL`: the **OnCall Application endpoint**
- `GRAFANA_ONCALL_API_KEY`: the **user-scoped API key**

Alternatively, set the Grafana endpoint and service account token:

```bash
export GRAFANA_URL="https://example.grafana.net"
export GRAFANA_SERVICE_ACCOUNT_TOKEN="glsa_..."
```

`GRAFANA_API_KEY` may be used instead of `GRAFANA_SERVICE_ACCOUNT_TOKEN` for compatibility with upstream `mcp-grafana` configuration conventions.

Optional settings:

```bash
# Default is oncall-app; accepted values: oncall-app | irm | unset
export GRAFANA_ONCALL_PLUGIN_PREFERENCE="oncall-app"

export GRAFANA_ONCALL_READ_ONLY=true
export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4318"
```

## Build

```bash
go mod download
go build -o grafana-oncall-mcp ./cmd/oncall-mcp
```

### Speckit setup (contributors)

This project uses [Spec Kit](https://github.com/github/spec-kit) for spec-driven development. Spec Kit manages the feature spec, plan, research, tasks, and contract artifacts under `specs/`.

1. Install the Specify CLI (requires [uv](https://docs.astral.sh/uv/) and Python 3.11+):

   ```bash
   uv tool install specify-cli --from git+https://github.com/github/spec-kit.git@v0.10.0
   ```

2. Initialize Spec Kit in the project root (this project is already initialized; run only if cloning fresh):

   ```bash
   specify init . --force --integration copilot
   ```

   This creates `.specify/memory/constitution.md` (gitignored) and registers the Spec Kit slash-commands (`/speckit.constitution`, `/speckit.specify`, `/speckit.plan`, `/speckit.tasks`, `/speckit.implement`) in your AI coding agent.

3. For each new feature, run the SDD pipeline from a feature spec:

   ```bash
   /speckit.specify <feature description>
   /speckit.plan <tech stack and architecture choices>
   /speckit.tasks
   ```

   The generated artifacts (`plan.md`, `research.md`, `data-model.md`, `contracts/`, `tasks.md`) are committed alongside the spec in `specs/<feature-number>-<name>/`.

## Run

### Stdio (default)

```bash
./grafana-oncall-mcp -transport=stdio
```

### SSE

```bash
./grafana-oncall-mcp -transport=sse -address=:8000
```

Connect the MCP client to:

```text
http://localhost:8000/sse
```

### Streamable HTTP

```bash
./grafana-oncall-mcp -transport=streamable-http -address=:8000
```

Connect the MCP client to:

```text
http://localhost:8000/mcp
```

## Tools

Read tools:

- `list_oncall_schedules`
- `get_oncall_shift`
- `get_current_oncall_users`
- `list_oncall_teams`
- `list_oncall_users`
- `list_alert_groups`
- `get_alert_group`

Write tools (omitted in read-only mode):

- `acknowledge_alert_group`
- `resolve_alert_group`
- `silence_alert_group`
- `unresolve_alert_group`

## RBAC Notes

The token must have permission to:
- Read OnCall schedules, shifts, teams, users, and alert groups
- Write (acknowledge / resolve / silence / unresolve) alert groups

Service account tokens are preferred over API keys. Use read-only mode (`GRAFANA_ONCALL_READ_ONLY=true`) in environments where the agent must never mutate state.

## Running with Docker

A multi-profile `docker-compose.yml` is provided at the repo root. It supports three transports (stdio / SSE / streamable-HTTP) and reads the same `.env` file the binary would read directly.

```bash
# One-time setup
cp env.example .env
$EDITOR .env                                # fill in GRAFANA_URL + token
docker compose build mcp-stdio              # build the image once

# One-shot stdio (the default profile) — use this from an MCP client:
docker compose run --rm mcp-stdio

# Or run a long-lived HTTP server in the background:
docker compose --profile http up -d
docker compose --profile http logs -f       # check the dual-plugin probe

# Stop the HTTP server
docker compose --profile http down
```

## Coding Agent Setup

The server works with any MCP-aware client (opencode, VS Code Copilot Chat, Claude Desktop, Cursor, Windsurf, etc.). All clients need the same environment variables. The examples below use the recommended direct OnCall API mode (`GRAFANA_ONCALL_API_URL` + `GRAFANA_ONCALL_API_KEY`) with `GRAFANA_ONCALL_PLUGIN_PREFERENCE="oncall-app"` as the default.

Set the env vars in your shell before launching the client:

```bash
export GRAFANA_ONCALL_API_URL="https://oncall-<region>.grafana.net/oncall"
export GRAFANA_ONCALL_API_KEY="<OnCall user-scoped API key>"
export GRAFANA_ONCALL_PLUGIN_PREFERENCE="oncall-app"
```

### opencode

Add a server entry to your `opencode.json` (user-scoped at `~/.config/opencode/opencode.json`, or project-scoped at `./opencode.json`):

**Using `uvx` (recommended):**

```json
{
  "mcp": {
    "grafana-oncall": {
      "type": "local",
      "command": ["uvx", "grafana-oncall-mcp", "-transport=stdio"],
      "environment": {
        "GRAFANA_ONCALL_API_URL": "{{env.GRAFANA_ONCALL_API_URL}}",
        "GRAFANA_ONCALL_API_KEY": "{{env.GRAFANA_ONCALL_API_KEY}}",
        "GRAFANA_ONCALL_PLUGIN_PREFERENCE": "oncall-app"
      },
      "enabled": true
    }
  }
}
```

**Using Docker:**

```json
{
  "mcp": {
    "grafana-oncall": {
      "type": "local",
      "command": ["docker", "compose", "-f", "<repo-path>/docker-compose.yml", "--project-directory", "<repo-path>", "run", "--rm", "--no-deps", "--profile", "stdio", "mcp-stdio"],
      "environment": {
        "GRAFANA_ONCALL_API_URL": "{{env.GRAFANA_ONCALL_API_URL}}",
        "GRAFANA_ONCALL_API_KEY": "{{env.GRAFANA_ONCALL_API_KEY}}",
        "GRAFANA_ONCALL_PLUGIN_PREFERENCE": "oncall-app"
      },
      "enabled": true
    }
  }
}
```

Replace `<repo-path>` with the absolute path to this repository.

### VS Code (GitHub Copilot Chat)

Add to `.vscode/mcp.json` (workspace-scoped) or VS Code `settings.json`:

**Using `uvx` (recommended):**

```json
{
  "servers": {
    "grafana-oncall": {
      "type": "stdio",
      "command": "uvx",
      "args": ["grafana-oncall-mcp", "-transport=stdio"],
      "env": {
        "GRAFANA_ONCALL_API_URL": "${input:grafanaOncallApiUrl}",
        "GRAFANA_ONCALL_API_KEY": "${input:grafanaOncallApiKey}",
        "GRAFANA_ONCALL_PLUGIN_PREFERENCE": "oncall-app"
      }
    }
  },
  "inputs": [
    {
      "id": "grafanaOncallApiUrl",
      "type": "promptString",
      "description": "OnCall Application endpoint URL",
      "default": "https://oncall-prod-us-central-0.grafana.net/oncall"
    },
    {
      "id": "grafanaOncallApiKey",
      "type": "promptString",
      "description": "OnCall user-scoped API key",
      "password": true
    }
  ]
}
```

Restart the Copilot Chat session after editing: Command Palette > "Copilot: Restart Chat Session".

### Claude Desktop

Add to `~/.claude/claude_desktop_config.json` (user-scoped):

**Using `uvx` (recommended):**

```json
{
  "mcpServers": {
    "grafana-oncall": {
      "command": "uvx",
      "args": ["grafana-oncall-mcp", "-transport=stdio"],
      "env": {
        "GRAFANA_ONCALL_API_URL": "${GRAFANA_ONCALL_API_URL}",
        "GRAFANA_ONCALL_API_KEY": "${GRAFANA_ONCALL_API_KEY}",
        "GRAFANA_ONCALL_PLUGIN_PREFERENCE": "oncall-app"
      }
    }
  }
}
```

**Using Docker:**

```json
{
  "mcpServers": {
    "grafana-oncall": {
      "command": "docker",
      "args": [
        "compose",
        "-f", "<repo-path>/docker-compose.yml",
        "--project-directory", "<repo-path>",
        "run", "--rm", "--no-deps", "--profile", "stdio", "mcp-stdio"
      ],
      "env": {
        "GRAFANA_ONCALL_API_URL": "${GRAFANA_ONCALL_API_URL}",
        "GRAFANA_ONCALL_API_KEY": "${GRAFANA_ONCALL_API_KEY}",
        "GRAFANA_ONCALL_PLUGIN_PREFERENCE": "oncall-app"
      }
    }
  }
}
```

Restart Claude Desktop after editing the config.

### Verifying the connection

After configuring your client, ask the agent:

> "List the OnCall teams in my Grafana."

A successful response from `list_oncall_teams` confirms the connection is healthy.

## Release Process (for maintainers)

This repo ships a single Go source tree, but produces **two distribution channels** from one source:

1. **GitHub Releases** (`.tar.gz` / `.zip`) — built by GoReleaser from `.goreleaser.yaml` on every `v*` tag push.
2. **PyPI** (`uvx grafana-oncall-mcp`) — built by [go-to-wheel](https://github.com/simonw/go-to-wheel) which bundles the Go binary into a Python wrapper. Eight platform-specific wheels (linux/darwin/windows × amd64/arm64) are produced and uploaded via OIDC trusted publishing.

The CI wiring lives in `.github/workflows/release.yml`. The two jobs (`goreleaser` then `pypi`) run sequentially on every `v*` tag push.
