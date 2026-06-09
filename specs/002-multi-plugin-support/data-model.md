# Phase 1 — Data Model

**Branch**: `002-multi-plugin-support` · **Date**: 2026-06-09
**Source spec**: [`spec.md`](./spec.md) · **Plan**: [`plan.md`](./plan.md) · **Research**: [`research.md`](./research.md)
**Inherits**: [`specs/001-oncall-mcp-server/data-model.md`](../../001-oncall-mcp-server/data-model.md) (Schedule, Shift, User, Team, AlertGroup, Integration, Pagination envelope, Error envelope, Volume assumptions are unchanged from 001 — this feature adds two new entities related to plugin selection and does not alter the wire-format data the server exposes to MCP clients).

This document defines the **domain entities** the server exposes through MCP
tool I/O and the two new **plugin-selection entities** used internally
by the dual-plugin startup probe. All wire-format field names use
`snake_case` and follow the conventions agreed in
`specs/001-oncall-mcp-server/data-model.md` (which this document
inherits verbatim — see the **Inheritance** section below).

The server is stateless — the wire-format entities are **transport
DTOs** mapped from the `amixr-api-go-client` Go structs to
JSON-friendly wire shapes. The new plugin-selection entities
(`Plugin`, `PluginPreference`) are internal-only and never appear in
MCP tool I/O; they exist solely to express the dual-plugin startup
state.

---

## Inheritance

The following entities and conventions are inherited unchanged from
`specs/001-oncall-mcp-server/data-model.md`:

| Inherited entity / convention | Source section |
|---|---|
| Conventions (Identifier, Timestamp, Duration, Timezone, Cursor, Enum, Unknown) | "Conventions" |
| Schedule | "Entity: Schedule" |
| Shift | "Entity: Shift" |
| UserSummary (embedded) | "Entity: UserSummary" |
| User | "Entity: User" |
| Team | "Entity: Team" |
| AlertGroup | "Entity: AlertGroup" |
| AlertGroup state-transition table | "State transitions" |
| Integration (read-only, embedded) | "Entity: Integration" |
| Pagination envelope (`items`, `next_cursor`, `total_estimate`) | "Pagination envelope" |
| Volume / scale assumptions | "Volume / scale assumptions" |
| Conventions shared with 11 MCP tools (cursor pagination, error envelope) | "Tool contract and consistency" |

The **error envelope** is inherited with a single renumbering:
`IRM_PLUGIN_MISSING` is renamed to `ONCALL_PLUGIN_MISSING`; the
legacy code is retained in the JSON Schema's `enum` for one minor
release as a deprecated alias (see `research.md` Decision 13).

---

## Entity: Plugin (internal, not in MCP tool I/O)

The two Grafana OnCall plugin IDs the server supports, plus a short
operator-facing code used in env vars, flags, and log lines.

| Field | Type | Description |
|---|---|---|
| `id` | string | The Grafana plugin ID. One of `grafana-oncall-app` \| `grafana-irm-app`. |
| `short_code` | string | The operator-facing code. One of `oncall-app` \| `irm`. Used in `GRAFANA_ONCALL_PLUGIN_PREFERENCE`, the `--plugin-preference` flag value, and the `plugin=…` log-line attribute. |
| `oncall_api_url_template` | string | The template for the OnCall API base URL. Rendered with the configured `GRAFANA_URL`. See **OnCallAPIBase** below. |
| `json_field` | string | The JSON field in the plugin settings response that contains the OnCall API URL, if applicable. `onCallApiUrl` for IRM; not applicable (URL is templated) for `oncall-app`. |

**Validation rules**:
- Exactly one of the two `id` values is supported. Any other value
  is rejected as `INVALID_CONFIG` at the env-var / flag parser.
- The `short_code` values are part of the public CLI / env-var
  contract; renaming them is a breaking change.

**Relationships**:
- A **Client** has at most one **Plugin** selected at startup
  (1:1, immutable for the process lifetime per FR-009).
- An **OnCallAPIBase** is derived from a **Plugin** + the
  configured `GRAFANA_URL`.

---

## Entity: PluginPreference (internal, not in MCP tool I/O)

The operator's explicit plugin selection, parsed from the
`GRAFANA_ONCALL_PLUGIN_PREFERENCE` env var or the
`--plugin-preference` flag.

| Field | Type | Description |
|---|---|---|
| `value` | enum | `oncall-app` \| `irm` \| `unset` |
| `source` | enum | `flag` \| `env` \| `default` — which input the value came from; for `unset` it is always `default` |

**Validation rules**:
- The parser accepts only the three values above; any other input
  produces an `INVALID_CONFIG` error naming the accepted values.
- The flag wins over the env var when both are set (consistent
  with 001's `--read-only` / `GRAFANA_ONCALL_READ_ONLY` rule).
- The `unset` value is the zero value of the Go enum; a process
  started without either input uses the "legacy preferred" default
  behavior (see `research.md` Decision 3 table).

**Relationships**:
- Combined with the **Plugin** probe results by the selection
  algorithm in `research.md` Decision 3 to produce a
  **Client.SelectedPlugin**.

---

## Entity: OnCallAPIBase (internal, not in MCP tool I/O)

The resolved HTTP base URL the server routes OnCall calls through.
Lives on the `oncall.Client` struct; computed once at startup and
immutable for the process lifetime per FR-009.

| Field | Type | Description |
|---|---|---|
| `url` | string | The fully resolved OnCall API base URL (without the trailing `api/v1/` — the amixr client appends it). |
| `plugin_id` | string | The **Plugin** this URL was derived from. |
| `resolved_at` | string (ISO 8601 UTC) | When the URL was resolved. Diagnostic only; not exposed in MCP tool I/O. |

**Per-plugin resolution rules**:

| Selected plugin | `url` template |
|---|---|
| `oncall-app` | `{GRAFANA_URL}/api/plugins/grafana-oncall-app/resources/api/v1/` (passed without the trailing `api/v1/`) |
| `irm` | `jsonData.onCallApiUrl` from `GET {GRAFANA_URL}/api/plugins/grafana-irm-app/settings`, with `api/v1/` appended by the amixr client at request time |

**Validation rules**:
- `url` MUST parse via `net/url.Parse`; the startup probe
  re-validates by issuing a `GET {url}/api/v1/users/?perpage=1`
  and expecting a 2xx response.
- A non-2xx response at this step surfaces as
  `UPSTREAM_UNAVAILABLE` with the selected plugin ID in the hint.

**Relationships**:
- Exactly one **OnCallAPIBase** per **Client** (1:1, immutable).
- Used to construct the `aapi.NewWithGrafanaURL(baseURL, token, grafanaURL)`
  call at startup.

---

## Error envelope (revised)

The error envelope is inherited from
`specs/001-oncall-mcp-server/data-model.md` (and the matching JSON
Schema in `contracts/error_envelope.schema.json`) with the
following change:

| Field | Change |
|---|---|
| `code` enum | `IRM_PLUGIN_MISSING` **renamed** to `ONCALL_PLUGIN_MISSING`. The legacy `IRM_PLUGIN_MISSING` value is retained in the JSON Schema's `enum` for one minor release as a deprecated alias. New tests assert both values pass validation. The server NEVER emits the legacy name. |
| `hint` (when `code=ONCALL_PLUGIN_MISSING`) | The hint MUST name both `grafana-oncall-app` and `grafana-irm-app` and point the operator to install at least one. |

No other error-code changes are introduced by this feature.

---

## Volume / scale assumptions

Inherited from 001 without change. The dual-plugin startup adds at
most one extra `GET /settings` round-trip (~50–150 ms) at startup
and zero per-request overhead, so the per-request cardinality
budgets (Schedule: 1–500, Shift: 1–10 000, User: 1–5 000, Team:
1–500, AlertGroup: up to 100 000 paginated) are unaffected.
