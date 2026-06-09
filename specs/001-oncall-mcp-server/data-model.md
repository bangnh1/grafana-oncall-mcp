# Phase 1 — Data Model

**Branch**: `001-oncall-mcp-server` · **Date**: 2026-06-08
**Source spec**: [`spec.md`](./spec.md) · **Plan**: [`plan.md`](./plan.md) · **Research**: [`research.md`](./research.md)

This document defines the **domain entities** the server exposes through MCP
tool I/O. All field names use `snake_case` and follow the conventions agreed
in `research.md` Decision 12–13 (cursor pagination, error envelope).

The server is stateless — these entities are **transport DTOs** mapped from
the `amixr-api-go-client` Go structs to JSON-friendly wire shapes. No
database, no ORM, no persistence layer.

---

## Conventions

| Concept | Type | Format |
|---|---|---|
| Identifier | `string` | Opaque, as returned by upstream OnCall API (never numeric — wrap if upstream returns int) |
| Timestamp | `string` | ISO 8601 UTC, e.g. `2026-06-08T14:30:00Z` |
| Duration | `string` | ISO 8601 duration, e.g. `PT15M` for 15 minutes |
| Timezone | `string` | IANA timezone name, e.g. `Europe/London`; `UTC` fallback if upstream is missing/invalid |
| Cursor | `string` | Opaque base64-encoded continuation token |
| Enum | `string` | Lowercase snake_case (e.g. `firing`, `acknowledged`) |
| Unknown / absent | `null` (output) or field omitted (input) | Never the string `"null"` |

---

## Entity: Schedule

A named on-call rotation owned by a team. Returned by
`list_oncall_schedules` and embedded in `get_oncall_shift` /
`get_current_oncall_users` responses.

| Field | Type | Required | Description |
|---|---|---|---|
| `id` | string | yes | OnCall schedule identifier |
| `name` | string | yes | Human-readable schedule name |
| `team_id` | string \| null | yes | Owning team; `null` for org-wide schedules |
| `type` | enum | yes | `web`, `calendar`, `ical` |
| `timezone` | string | yes | IANA TZ; `UTC` if upstream missing |
| `timezone_was_inferred` | bool | yes | `true` when upstream did not provide a TZ |
| `shift_ids` | string[] | yes | Stable IDs of shifts comprising this schedule (may be empty) |

**Lifecycle / state transitions**: none — schedules are configuration
artifacts; the server does not mutate them.

**Validation rules**:
- `id` MUST be non-empty.
- `name` MUST be non-empty.
- `timezone` MUST parse via Go's `time.LoadLocation`; if not, replace with
  `UTC` and set `timezone_was_inferred=true`.
- `type` MUST be one of the enum values; unknown upstream values map to
  `web` with a structured log warning.

**Relationships**:
- Belongs to at most one **Team** (`team_id` → `Team.id`).
- Has many **Shift**s (`shift_ids[]` → `Shift.id`).

---

## Entity: Shift

A time window inside a schedule with one or more on-call users.

| Field | Type | Required | Description |
|---|---|---|---|
| `id` | string | yes | Shift identifier |
| `schedule_id` | string | yes | Parent schedule |
| `name` | string \| null | yes | Optional display label |
| `type` | enum | yes | `single_event`, `recurrent_event`, `rolling_users` |
| `start_at` | string | yes | ISO 8601 UTC start time |
| `duration` | string | yes | ISO 8601 duration (e.g. `PT8H`) |
| `frequency` | enum \| null | yes | `daily`, `weekly`, `monthly`, or `null` for one-shot |
| `interval` | int \| null | yes | Repeat interval (frequency multiplier) |
| `users` | UserSummary[] | yes | Users on this shift (see UserSummary below) |
| `rotation_start` | string \| null | yes | When this rotation began (`null` for non-rolling) |

**Validation rules**:
- `start_at` MUST be a valid RFC 3339 timestamp.
- `duration` MUST parse as an ISO 8601 duration.
- `users` MAY be empty if the shift is a placeholder, but the field is
  always present.

**Relationships**:
- Belongs to one **Schedule**.
- References many **User**s via `UserSummary.id`.

---

## Entity: UserSummary (embedded)

Reduced projection of a User used inside Shift / current-on-call responses
to avoid duplicating full user records.

| Field | Type | Required | Description |
|---|---|---|---|
| `id` | string | yes | User identifier |
| `username` | string | yes | Login name |
| `display_name` | string \| null | yes | Human-friendly name |

---

## Entity: User

A Grafana OnCall user. Returned by `list_oncall_users` and embedded as
`UserSummary` in shift/coverage responses.

| Field | Type | Required | Description |
|---|---|---|---|
| `id` | string | yes | User identifier |
| `username` | string | yes | Login name |
| `email` | string \| null | yes | Contact email (may be null per RBAC) |
| `display_name` | string \| null | yes | Human-friendly name |
| `role` | enum \| null | yes | `admin`, `editor`, `viewer`, or `null` if unknown |
| `timezone` | string \| null | yes | IANA TZ |
| `team_ids` | string[] | yes | Teams this user belongs to |

**Validation rules**:
- `email` is the only PII field; redact in logs (handler-level rule).
- `role` unknown values map to `null`.

**Relationships**: Belongs to many **Team**s.

---

## Entity: Team

| Field | Type | Required | Description |
|---|---|---|---|
| `id` | string | yes | Team identifier |
| `name` | string | yes | Display name |
| `email` | string \| null | yes | Team contact email |
| `avatar_url` | string \| null | yes | Avatar URL if present |

**Validation rules**: `id` and `name` MUST be non-empty.

**Relationships**: Has many **Schedule**s and many **User**s.

---

## Entity: AlertGroup

A correlated set of alerts presented as one unit. Returned by
`list_alert_groups` (summary view) and `get_alert_group` (full view).

| Field | Type | Required | View | Description |
|---|---|---|---|---|
| `id` | string | yes | summary, full | Alert group identifier |
| `title` | string | yes | summary, full | Headline (typically derived from the first alert) |
| `state` | enum | yes | summary, full | `firing`, `acknowledged`, `resolved`, `silenced` |
| `severity` | string \| null | yes | summary, full | Label-derived severity (e.g. `critical`) |
| `integration_id` | string | yes | summary, full | Originating integration |
| `integration_name` | string | yes | summary, full | Denormalized for agent convenience |
| `route_id` | string \| null | yes | summary, full | Route that matched this alert group |
| `team_id` | string \| null | yes | summary, full | Owning team |
| `created_at` | string | yes | summary, full | ISO 8601 UTC |
| `updated_at` | string | yes | summary, full | ISO 8601 UTC |
| `resolved_at` | string \| null | yes | summary, full | ISO 8601 UTC, present iff `state=resolved` |
| `acknowledged_at` | string \| null | yes | summary, full | ISO 8601 UTC, present iff currently acked |
| `acknowledged_by` | UserSummary \| null | yes | summary, full | The acking user |
| `silenced_until` | string \| null | yes | summary, full | ISO 8601 UTC end of silence, if any |
| `alerts_count` | int | yes | summary, full | Total member alerts |
| `labels` | object | yes | full only | `{key: value}` map of normalized labels |
| `permalink` | string | yes | full only | Direct URL to the alert group in Grafana OnCall |

### State transitions

The server reflects upstream state. Allowed transitions per the OnCall API:

```
                +----------------+
                |    firing      |
                +----------------+
                  ^   |   |   ^
                  |   v   v   |
       +----------------+   +-----------+
       | acknowledged   |<->| silenced  |
       +----------------+   +-----------+
                  ^   |
                  |   v
                +----------------+
                |   resolved     |
                +----------------+
```

| From | Tool | To | Notes |
|---|---|---|---|
| `firing` | `acknowledge_alert_group` | `acknowledged` | Idempotent — if already acknowledged, returns `was_already_in_state=true` |
| `firing` | `resolve_alert_group` | `resolved` | Idempotent |
| `firing` | `silence_alert_group` | `silenced` | Non-idempotent (delay restarts) |
| `acknowledged` | `resolve_alert_group` | `resolved` | Idempotent |
| `acknowledged` | `silence_alert_group` | `silenced` | Non-idempotent |
| `silenced` | `acknowledge_alert_group` | `acknowledged` | Idempotent |
| `silenced` | `resolve_alert_group` | `resolved` | Idempotent |
| `resolved` | `unresolve_alert_group` | `firing` | Idempotent (if already firing, no-op) |
| `resolved` | `acknowledge_alert_group` | — | Rejected with `STATE_TRANSITION_REJECTED` |
| `resolved` | `silence_alert_group` | — | Rejected with `STATE_TRANSITION_REJECTED` |
| `firing`/`acknowledged`/`silenced` | `unresolve_alert_group` | — | Rejected with `STATE_TRANSITION_REJECTED` |

**Validation rules**:
- `state` MUST be one of the four enum values; unknown upstream states are
  surfaced as `INTERNAL` errors with a log entry — the contract is closed
  on purpose.
- All timestamps MUST be UTC.
- `labels` MUST be a JSON object of `string -> string`; coerce upstream
  scalar non-strings to strings (`true` → `"true"`).

**Relationships**:
- Belongs to one **Integration** (`integration_id`).
- Optionally belongs to one **Team** (`team_id`).
- Optionally references one **UserSummary** (`acknowledged_by`).

---

## Entity: Integration (read-only, embedded)

Currently surfaced only via `integration_id` + `integration_name` inside
**AlertGroup** for v1. No standalone tool. Listed here so future expansion
has a place to record the schema without renumbering.

| Field | Type | Description |
|---|---|---|
| `id` | string | Integration identifier |
| `name` | string | Display name |
| `type` | string | e.g. `grafana_alerting`, `webhook` |

---

## Pagination envelope

Used by every list tool's output:

| Field | Type | Required | Description |
|---|---|---|---|
| `items` | `T[]` | yes | Page of results; empty array, never `null` |
| `next_cursor` | string \| null | yes | Opaque cursor; `null` when exhausted |
| `total_estimate` | int \| null | yes | Best-effort total count from upstream, `null` when unknown |

---

## Error envelope

Used by **every** tool error response. Identical to the schema in
`contracts/error_envelope.schema.json`.

| Field | Type | Required | Description |
|---|---|---|---|
| `code` | string | yes | Stable code from the enumerated set (research.md Decision 13) |
| `message` | string | yes | Human-readable explanation |
| `tool` | string | yes | The failing tool name |
| `hint` | string \| null | yes | Optional actionable hint |
| `retryable` | bool | yes | Whether the agent should retry |

---

## Volume / scale assumptions

| Entity | Expected v1 cardinality (per Grafana instance) |
|---|---|
| Schedule | 1–500 |
| Shift | 1–10 000 |
| User | 1–5 000 |
| Team | 1–500 |
| AlertGroup | new: 0–100/hour; queryable: up to 100 000 (paginated) |

These targets inform the pagination defaults (`limit=50`, `max=200`) and the
performance budgets in the spec (FR-040, FR-043).
