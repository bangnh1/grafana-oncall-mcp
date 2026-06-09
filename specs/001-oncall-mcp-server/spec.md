# Feature Specification: Grafana OnCall MCP Server

**Feature Branch**: `001-oncall-mcp-server`
**Created**: 2026-06-08
**Status**: Draft
**Input**: User description: "create a grafana on-call mcp server helps AI models can contact directly with grafana oncall. (https://github.com/grafana/mcp-grafana) support grafana oncall but only support grafana-irm-app because grafana-oncall-app have been archived, so we need a specific grafana on-call mcp server"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Inspect on-call coverage during an incident (Priority: P1)

A site reliability engineer chatting with an AI assistant during a live
incident asks "who's on call for the database team right now and what's their
escalation path?" The assistant uses the MCP server to retrieve the relevant
schedule, the current on-call user, and the team's escalation contacts, and
replies with names, handles, and shift end times — without the engineer
opening the Grafana OnCall UI.

**Why this priority**: This is the single most common and time-critical
question asked of an on-call system. Solving it first delivers immediate value
even if no other tool is implemented.

**Independent Test**: With a populated test Grafana OnCall instance, an AI
agent prompts "who's on call for team X right now?". The agent receives a
correct, current answer that matches what the OnCall UI shows for the same
moment.

**Acceptance Scenarios**:

1. **Given** a configured MCP server and a schedule with a known on-call user,
   **When** the assistant asks who is on call for that schedule,
   **Then** the response identifies the correct user, their handle, and the
   shift end time in the user's timezone.
2. **Given** a team with multiple schedules,
   **When** the assistant asks who is on call for that team,
   **Then** all currently-on-call users across the team's schedules are
   returned.
3. **Given** an empty schedule (no one currently on call),
   **When** the assistant asks who is on call,
   **Then** the response clearly states no one is currently on call rather
   than returning an empty result the assistant might misinterpret.

---

### User Story 2 - Triage and act on active alert groups (Priority: P1)

An engineer pastes an alert link or alert group ID into chat and asks the
assistant to "summarize this alert group and acknowledge it on my behalf."
The assistant fetches the alert group's title, status, severity, count of
firing alerts, current resolver, and recent activity, presents a summary, and
— with the user's confirmation — acknowledges the group through the MCP
server. The engineer never leaves the chat surface.

**Why this priority**: Triage is the second-highest-volume on-call workflow
after coverage lookups, and the ability to act (not just read) is what makes
the integration feel like a force multiplier rather than a read-only viewer.

**Independent Test**: With a known alert group in a test instance, an agent
fetches its details, then issues an acknowledge action; the OnCall UI
reflects the new state within seconds and attributes the action to the
correct identity.

**Acceptance Scenarios**:

1. **Given** an alert group ID,
   **When** the assistant requests its details,
   **Then** it receives the title, state, severity, integration name,
   creation time, last update, and counts of acknowledged/resolved/silenced
   members.
2. **Given** a "new" alert group,
   **When** the assistant acknowledges it,
   **Then** the alert group transitions to "acknowledged" in Grafana OnCall
   and the activity log records the action attributed to the configured
   identity.
3. **Given** an already-resolved alert group,
   **When** the assistant attempts to acknowledge it,
   **Then** the response is a clear, actionable error explaining that
   resolved groups cannot be acknowledged.

---

### User Story 3 - Discover schedules, teams, and users (Priority: P2)

A new team member asks the assistant "what on-call schedules exist for the
platform org, and who's on each rotation?" The assistant lists the available
schedules with names, timezones, and team membership, then drills into one
to show the upcoming rotation order.

**Why this priority**: Discovery is foundational for less-frequent but
important workflows (onboarding, planning swaps, audits). It depends on the
same underlying API surface as P1/P2 but is invoked less often.

**Independent Test**: An agent enumerates schedules filtered by team and
receives a list whose names, IDs, and member counts match the OnCall UI.

**Acceptance Scenarios**:

1. **Given** a Grafana OnCall instance with multiple teams,
   **When** the assistant lists teams,
   **Then** all teams the configured identity can see are returned with
   stable IDs and names.
2. **Given** a team,
   **When** the assistant lists that team's schedules,
   **Then** only schedules belonging to that team are returned.
3. **Given** a schedule,
   **When** the assistant requests its rotation,
   **Then** the response includes the schedule's shift IDs (retrieved via
    the shift lookup tool) — note that a dedicated single-schedule detail
    tool is intentionally not provided; callers compose `list_oncall_schedules`
    results with `get_oncall_shift` for per-shift detail.

---

### User Story 4 - Filter and search alert groups (Priority: P2)

An engineer asks "show me all unacknowledged critical alerts from the
payments integration in the last 24 hours." The assistant queries the MCP
server with the appropriate filters (state, integration, severity label,
time range) and returns a concise list.

**Why this priority**: Targeted querying is essential for retrospectives and
broad triage, but the single-group lookup (US2) covers the most urgent path.

**Independent Test**: Given known alert groups across states, integrations,
labels, and times, the agent receives only groups matching the requested
filters.

**Acceptance Scenarios**:

1. **Given** the same parameter names used across all list-style tools,
   **When** the assistant filters by state, integration, label, or time
   range,
   **Then** results are scoped accordingly and pagination cursors are
   returned when results exceed the page size.
2. **Given** a filter that matches no alert groups,
   **When** the assistant queries,
   **Then** an empty list with a clear "no matches" indication is returned
   (not an error).

---

### Edge Cases

- The Grafana OnCall API is temporarily unreachable (network error, 5xx) —
  the server retries with backoff and ultimately returns a structured error
  the agent can surface and reason about, rather than hanging.
- The configured API token is invalid or has been revoked — the server
  returns an authentication error on the first failing call and the agent
  can prompt the user to refresh credentials.
- The configured token lacks the scope required for a specific tool — the
  server returns a permission-denied error naming the missing scope.
- The user asks about a schedule, team, alert group, or user ID that does
  not exist — the server returns a structured "not found" error instead of a
  generic failure.
- A timezone is missing or invalid on a schedule — times are normalized to
  UTC and the absence is flagged in the response.
- A list operation returns more results than fit in one page — the response
  includes a pagination cursor and the agent can request subsequent pages
  with identical filter semantics.
- The Grafana instance hosts the rebranded `grafana-irm-app` plugin (not
  the targeted `grafana-oncall-app`) — the server detects this at startup
  and refuses to serve, with an actionable error pointing to the unsupported
  configuration.
- A long-running action (e.g., bulk silence) cannot complete within the read
  latency budget — it is rejected with an explanation rather than blocking.

## Clarifications

### Session 2026-06-08

- Q: Should the server expose alert-group tools and a single-schedule detail
  tool in addition to the 5 OnCall tools enumerated in the original request
  (list_oncall_schedules, get_oncall_shift, get_current_oncall_users,
  list_oncall_teams, list_oncall_users)? → A: Yes for alert-group support
  (list, get, acknowledge, resolve, silence, unacknowledge/unresolve) plus
  the read-only mode toggle; no for a single-schedule detail tool (FR-011
  and its corresponding US3 acceptance scenario are dropped). Other Grafana
  surfaces outside OnCall (dashboards, datasources, alerting rules, etc.)
  remain out of scope.

### Session 2026-06-09

- Q: Which Grafana OnCall plugin should the server target — the legacy
  `grafana-oncall-app` or the rebranded `grafana-irm-app`? → A: Target the
  legacy `grafana-oncall-app` plugin; the rebranded `grafana-irm-app` is
  explicitly out of scope. The startup plugin check, FR-003/FR-004 wording,
  the corresponding edge case, and the Assumptions section have been
  updated to reflect this target.

## Requirements *(mandatory)*

### Functional Requirements

#### Server lifecycle and configuration

- **FR-001**: The server MUST run as a standalone Model Context Protocol
  server that an MCP-compatible AI client can connect to over the standard
  MCP transport (stdio at minimum).
- **FR-002**: The server MUST accept configuration for the Grafana base URL
  and an authentication credential (token) exclusively through environment
  variables or an equivalent secrets mechanism; credentials MUST NOT be
  required as command-line arguments or written to logs.
- **FR-003**: The server MUST target the Grafana OnCall plugin
  (`grafana-oncall-app`) API surface; the rebranded `grafana-irm-app` plugin
  MUST NOT be a supported target.
- **FR-004**: On startup, the server MUST verify that the configured Grafana
  instance has the supported OnCall plugin available, and MUST refuse to serve
  requests with a clear, actionable error if it does not.
- **FR-005**: The server MUST fail closed: missing required configuration or
  failed startup checks MUST prevent the server from accepting tool calls
  rather than degrading silently.

#### Tool surface — reads (required)

- **FR-010**: The server MUST expose a tool to list on-call schedules,
  filterable by team.
- **FR-012**: The server MUST expose a tool to retrieve the user(s)
  currently on call for a given schedule, including each user's identifier,
  display name, and shift end time.
- **FR-013**: The server MUST expose a tool to retrieve a single on-call
  shift by ID.
- **FR-014**: The server MUST expose a tool to list teams.
- **FR-015**: The server MUST expose a tool to list users, filterable by
  username.
- **FR-016**: The server MUST expose a tool to list alert groups with
  filters for state, integration, route, team, time range, labels, and free
  text.
- **FR-017**: The server MUST expose a tool to retrieve a single alert group
  by ID, including its current state, severity, integration, creation time,
  and member alert counts.

#### Tool surface — writes (required)

- **FR-020**: The server MUST expose a tool to acknowledge an alert group.
- **FR-021**: The server MUST expose a tool to resolve an alert group.
- **FR-022**: The server MUST expose a tool to silence an alert group for a
  caller-specified duration.
- **FR-023**: The server MUST expose a tool to unacknowledge or unresolve an
  alert group where the underlying API supports it.
- **FR-024**: The server MUST support a read-only mode (toggled via
  configuration) that disables every write tool; in this mode write tools
  MUST be absent from the tool list rather than present-but-erroring.

#### Tool contract and consistency

- **FR-030**: All tool names MUST follow a single, documented naming
  convention (`<resource>_<verb>`, snake_case).
- **FR-031**: All tools MUST declare and validate input and output JSON
  schemas; unknown or malformed inputs MUST be rejected with a structured
  validation error before any outbound call is made.
- **FR-032**: All tools MUST use identical parameter names, casing, and
  units for the same concept (timestamps as ISO 8601 UTC strings, durations
  as ISO 8601, IDs as strings, pagination via a shared cursor parameter).
- **FR-033**: Every error response MUST include a stable error code, a
  human-readable message, the failing tool name, and — where applicable — an
  actionable hint (e.g., the missing scope, the invalid field).
- **FR-034**: List-style tools MUST return a pagination cursor when more
  results exist and MUST accept that cursor on subsequent calls.

#### Reliability and performance

- **FR-040**: Read-only tool calls MUST respond within 2 seconds at p95
  under nominal load (up to 10 concurrent requests).
- **FR-041**: Every outbound call to the Grafana OnCall API MUST have an
  explicit timeout (default 10 seconds) and MUST apply exponential backoff
  with jitter on retryable failures (HTTP 429 and 5xx), up to 3 retries.
- **FR-042**: The server MUST surface upstream rate-limit signals (HTTP 429
  with `Retry-After`) to the caller in a structured form instead of silently
  blocking.
- **FR-043**: Per-request peak memory MUST remain under 100 MB.

#### Observability and security

- **FR-050**: The server MUST emit structured logs for every tool
  invocation, including tool name, duration, outcome (success / error code),
  and a correlation identifier — without leaking secrets, full bearer
  tokens, or alert payload contents that could contain PII.
- **FR-051**: The server MUST redact credentials in any error or debug
  output.
- **FR-052**: The server MUST refuse to start if the configured base URL
  uses an unencrypted scheme against a non-local host.

### Key Entities

- **Schedule**: A named on-call rotation belonging to a team, with a
  timezone, type (e.g., calendar, web), and an ordered set of shifts.
- **Shift**: A time window within a schedule during which one or more users
  are designated as on call.
- **User**: A Grafana OnCall user with an identifier, display name, and
  optional contact handles.
- **Team**: A grouping of users and schedules that owns a set of
  integrations and alert routes.
- **Alert Group**: A correlated set of alerts presented as a single unit,
  with a lifecycle state (new, acknowledged, resolved, silenced), severity,
  originating integration, and activity log.
- **Integration**: The configured upstream source that produces alerts
  routed into alert groups.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An AI assistant connected to the server can correctly answer
  "who's on call for <team>?" in under 3 seconds end-to-end (assistant
  request → MCP tool call → answer) in 95% of trials against a populated
  test instance.
- **SC-002**: An AI assistant can fetch an alert group's summary and
  acknowledge it through a single connected session, with the change visible
  in the Grafana OnCall UI within 5 seconds of the assistant's action, in
  100% of trials.
- **SC-003**: For a representative load of 10 concurrent read requests
  spanning the four most-used tools, p95 latency stays at or below 2 seconds
  and no request exceeds the 10-second outbound timeout.
- **SC-004**: A new operator can take the server from "credentials in hand"
  to "first successful tool call from a connected MCP client" in under 10
  minutes by following the README, with no source-code edits required.
- **SC-005**: At least 80% of consumers (measured via documentation
  feedback or integration support tickets, where available) report that
  tool names, parameter names, and error messages are consistent enough that
  they "don't have to look up the schema for each tool."
- **SC-006**: When the upstream Grafana OnCall API is unavailable, every
  affected tool call returns a structured error within the configured
  timeout in 100% of trials — no request hangs longer than 12 seconds
  (10 s timeout + 2 s overhead budget).
- **SC-007**: When deployed in read-only mode, no write tool is advertised
  to clients and no write call can succeed, verified by automated tests in
  100% of CI runs.

## Assumptions

- The target Grafana instances host the supported `grafana-oncall-app`
  plugin (the legacy/original OnCall plugin) and expose an API surface
  compatible with the upstream Grafana OnCall HTTP API; the server is not
  expected to abstract over the rebranded IRM schema.
- Authentication is performed via a single Grafana service account token or
  the equivalent OnCall API token; multi-tenant or per-request credential
  passing is out of scope for v1.
- Stdio is the primary MCP transport; SSE/HTTP transports are nice-to-have
  but not required for v1.
- The MCP client (the AI agent's runtime) handles user identity,
  confirmation prompts before destructive actions, and conversation context;
  the server is not responsible for asking "are you sure?".
- The README and a single quickstart document — not an interactive
  installer — are sufficient operator-facing documentation for v1.
- The server is operated by the same organization that operates the Grafana
  instance; cross-organization or hosted multi-tenant deployment is out of
  scope for v1.
- "Currently on call" is computed from the upstream API's authoritative
  view; the server does not attempt to re-derive coverage from raw shifts.
- Only Grafana OnCall functionality (schedules, shifts, current on-call,
  teams, users, and alert groups) is in scope. Other Grafana surfaces —
  dashboards, datasources, alerting rules, Loki/Prometheus query passthrough,
  incidents/Sift, navigation deeplinks, rendering, provisioning, admin — are
  explicitly out of scope and MUST NOT be implemented by this server.
- A dedicated single-schedule-detail tool is intentionally not provided in
  v1; callers can combine `list_oncall_schedules` with `get_oncall_shift`
  for per-shift information.
