# Feature Specification: Multi-Plugin OnCall Support

**Feature Branch**: `002-multi-plugin-support`  
**Created**: 2026-06-09  
**Status**: Draft  
**Input**: User description: "tôi đã đổi ý, support cả grafana-irm-app và grafana-oncall-app" ("I changed my mind, support both `grafana-irm-app` and `grafana-oncall-app`")

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Operator runs the server against an IRM-only Grafana (Priority: P1)

A platform operator who recently upgraded their Grafana to the rebranded
`grafana-irm-app` (and no longer has the legacy `grafana-oncall-app`)
installs the MCP server, points it at their Grafana URL with a service
account token, and starts it. The server detects the `grafana-irm-app`
plugin at startup, configures itself against the IRM OnCall URL
shape, and serves all 11 OnCall tools without any operator-side
configuration changes (no new flags, no manual URL overrides).

**Why this priority**: This is the most common production case in 2026 —
Grafana Cloud stacks that have been on the IRM plugin for ≥1 year cannot
use a server that only supports the archived `grafana-oncall-app`. Without
IRM support the server has no users in the largest deployment base.

**Independent Test**: With a Grafana instance hosting only `grafana-irm-app`,
the server starts, registers the same 11 tools as the single-plugin build,
and a smoke call (`list_oncall_teams`) returns the same data shape the
operator sees in the Grafana OnCall UI.

**Acceptance Scenarios**:

1. **Given** a Grafana instance with `grafana-irm-app` installed (and the
   legacy `grafana-oncall-app` absent),
   **When** the operator starts the server,
   **Then** startup succeeds, the operator-facing log states
   `plugin=irm`, and the server registers all 11 tools.
2. **Given** the server is running against an IRM-only Grafana,
   **When** the assistant calls any of the 11 tools,
   **Then** the request reaches the IRM OnCall API and the response shape
   is identical to the single-plugin build.
3. **Given** an operator who previously had the server connected to a
   `grafana-oncall-app` Grafana and is now migrating to `grafana-irm-app`,
   **When** they re-point `GRAFANA_URL` and restart,
   **Then** no code change, no config-file edit, and no tool rename is
   required — the same prompt and tool names continue to work.

---

### User Story 2 - Operator runs the server against the legacy on-call-app (Priority: P1)

A platform operator whose Grafana still hosts the legacy (archived)
`grafana-oncall-app` plugin — either on Grafana ≤ 10.x or on a self-hosted
Grafana that never picked up the IRM migration — installs the MCP
server, points it at their Grafana URL, and starts it. The server
detects `grafana-oncall-app`, configures itself against the legacy
OnCall resource-proxy URL shape, and serves the same 11 OnCall tools.

**Why this priority**: Self-hosted and pinned-version Grafana deployments
frequently remain on the archived plugin. Without legacy support, those
operators have no upgrade path and continue to be unable to use the
upstream `grafana/mcp-grafana` server (which only supports IRM).

**Independent Test**: With a Grafana instance hosting only the legacy
`grafana-oncall-app`, the server starts, registers all 11 tools, and
calls match what the OnCall UI shows.

**Acceptance Scenarios**:

1. **Given** a Grafana instance with `grafana-oncall-app` installed,
   **When** the operator starts the server,
   **Then** startup succeeds, the operator-facing log states
   `plugin=oncall-app`, and the server registers all 11 tools.
2. **Given** the server is running against a legacy `grafana-oncall-app`
   Grafana,
   **When** the assistant calls any of the 11 tools,
   **Then** the request reaches the legacy OnCall resource-proxy API and
   the response shape is identical to the IRM build.
3. **Given** the operator's `GRAFANA_URL` is pointed at a stack that
   hosts only `grafana-oncall-app`,
   **When** the server starts,
   **Then** it does NOT refuse to start, does NOT log
   `ONCALL_PLUGIN_MISSING`, and does NOT require any new configuration
   variable.

---

### User Story 3 - Operator runs the server against a Grafana with BOTH plugins installed (Priority: P2)

Some self-hosted Grafana deployments may carry both the legacy
`grafana-oncall-app` (pinned) and the rebranded `grafana-irm-app` while
they migrate. The operator wants the server to pick one plugin
deterministically (the legacy one first, since it is the deployment's
known-stable surface) and log which one it picked, so that misconfiguration
is obvious.

**Why this priority**: Real but uncommon. Operators in this state are
mid-migration, and the deterministic-preference behaviour avoids
"works differently after a Grafana upgrade" surprises.

**Independent Test**: With a Grafana hosting both plugins, the server
starts, logs which plugin it selected, and serves the 11 tools against
the chosen plugin. A second start with the legacy plugin uninstalled
succeeds and logs the switch to IRM.

**Acceptance Scenarios**:

1. **Given** a Grafana with both plugins installed,
   **When** the server starts,
   **Then** it selects `grafana-oncall-app` first and logs
   `plugin=oncall-app preferred over irm-app`.
2. **Given** a Grafana with both plugins installed,
   **When** the operator sets `GRAFANA_ONCALL_PLUGIN_PREFERENCE=irm`
   **Then** the server selects `grafana-irm-app` and logs
   `plugin=irm preferred per operator config`.
3. **Given** a Grafana with only `grafana-irm-app`,
   **When** the operator sets `GRAFANA_ONCALL_PLUGIN_PREFERENCE=oncall-app`
   (an unsupported value),
   **Then** startup fails with a structured `INVALID_CONFIG` error naming
   the unsupported preference value, rather than silently falling back.

---

### User Story 4 - Operator migrates an MCP client from the upstream IRM-only server to this one (Priority: P2)

An AI engineer already using upstream `grafana/mcp-grafana` against a
`grafana-irm-app` Grafana wants to consolidate to this server (single
binary, single config surface). They swap the MCP client entry, point
at this server's transport, and expect every existing prompt to work
unchanged.

**Why this priority**: Migration-from-upstream is the project's primary
distribution story; without it the project is "yet another server" rather
than "a server that works for everyone the upstream one left behind".

**Independent Test**: Run a prompt that exercises
`list_oncall_teams`, `get_current_oncall_users`, and
`acknowledge_alert_group` against both the upstream server and this one
on the same IRM Grafana. Tool names, parameter names, and error
envelope codes are identical.

**Acceptance Scenarios**:

1. **Given** an MCP client configured for upstream `mcp-grafana`,
   **When** the operator re-points the client at this server,
   **Then** all 5 user-enumerated upstream tool names
   (`list_oncall_schedules`, `get_oncall_shift`, `get_current_oncall_users`,
   `list_oncall_teams`, `list_oncall_users`) remain available without
   prompt rewrites.
2. **Given** the same client,
   **When** the operator re-points the client at this server,
   **Then** the 6 additional alert-group tools
   (`list_alert_groups`, `get_alert_group`, `acknowledge_alert_group`,
   `resolve_alert_group`, `silence_alert_group`, `unresolve_alert_group`)
   are now available (the upstream server's tool list is a strict subset
   of this server's tool list).

---

### Edge Cases

- The Grafana instance hosts NEITHER `grafana-irm-app` NOR the legacy
  `grafana-oncall-app` — the server refuses to start with a structured
  `ONCALL_PLUGIN_MISSING` error, naming both plugin IDs and pointing the
  operator to install one of them.
- The Grafana instance hosts the legacy plugin but the operator's
  service-account token lacks the OnCall scope — the server starts
  (plugin probe succeeded) but the first call to any OnCall tool
  returns `FORBIDDEN` with a hint naming the missing scope, consistent
  with FR-033.
- The Grafana instance hosts both plugins but the legacy plugin's
  settings endpoint returns 500 (broken install) while IRM returns 200 —
  the server falls back to IRM and logs a `WARN` line identifying the
  broken legacy plugin, rather than refusing to start.
- The operator overrides the preference to a value that names neither
  installed plugin (e.g., `GRAFANA_ONCALL_PLUGIN_PREFERENCE=irm` but only
  `grafana-oncall-app` is installed) — the server refuses to start with
  `INVALID_CONFIG` rather than silently overriding the preference.
- The OnCall API base URL extracted at startup becomes unreachable
  mid-session (e.g., plugin disabled via the Grafana UI) — the next
  tool call returns `UPSTREAM_UNAVAILABLE` and the operator-facing log
  includes the previously selected plugin ID for diagnosis; the server
  does NOT re-probe and re-select (plugin selection is a startup
  decision only).
- The legacy plugin returns a different OnCall API URL shape than the
  IRM plugin — the server stores the resolved URL per plugin and routes
  each call through the correct path; no per-call re-resolution.

## Requirements *(mandatory)*

### Functional Requirements

#### Server lifecycle and configuration

- **FR-001**: The server MUST run as a standalone Model Context Protocol
  server that an MCP-compatible AI client can connect to over the
  standard MCP transport (stdio at minimum). *(inherited from 001-oncall-mcp-server; unchanged.)*
- **FR-002**: The server MUST accept configuration for the Grafana base
  URL and an authentication credential (token) exclusively through
  environment variables or an equivalent secrets mechanism; credentials
  MUST NOT be required as command-line arguments or written to logs.
  *(inherited; unchanged.)*
- **FR-003 (revised)**: The server MUST support both the legacy
  `grafana-oncall-app` plugin AND the rebranded `grafana-irm-app`
  plugin. The server MUST refuse to start if neither plugin is
  installed.
- **FR-004 (revised)**: On startup, the server MUST probe the Grafana
  instance for both `grafana-oncall-app` and `grafana-irm-app`, select
  exactly one (per FR-008), resolve the OnCall API base URL appropriate
  to the selected plugin, and MUST refuse to serve requests with a
  clear, actionable error if neither plugin is available.
- **FR-005**: The server MUST fail closed: missing required configuration
  or failed startup checks MUST prevent the server from accepting tool
  calls rather than degrading silently. *(inherited; unchanged.)*
- **FR-006 (new)**: The server MUST accept an optional operator override
  `GRAFANA_ONCALL_PLUGIN_PREFERENCE` (or `--plugin-preference` flag)
  accepting one of `oncall-app` (default) or `irm`; the selected value
  MUST be honored only if the corresponding plugin is installed, and
  MUST be rejected as `INVALID_CONFIG` otherwise.
- **FR-007 (new)**: The server MUST log the selected plugin ID
  (`oncall-app` or `irm`) at startup, alongside the resolved OnCall API
  base URL with the token redacted, so an operator can confirm which
  surface the server is talking to.
- **FR-008 (new)**: When no operator preference is set and both plugins
  are installed, the server MUST prefer `grafana-oncall-app` (the
  legacy plugin) and MUST emit a `WARN`-level log line explaining that
  the legacy plugin was preferred; the operator may override via
  FR-006.
- **FR-009 (new)**: The selected plugin MUST be re-validated only at
  startup. Mid-session plugin changes (a plugin being uninstalled or
  disabled) MUST surface as `UPSTREAM_UNAVAILABLE` on the next call,
  with the previously selected plugin ID included in the error hint for
  diagnosis. The server MUST NOT re-probe and silently re-select a
  different plugin mid-session.

#### Tool surface (unchanged from 001-oncall-mcp-server)

- **FR-010**: The server MUST expose a tool to list on-call schedules,
  filterable by team. *(inherited.)*
- **FR-012**: The server MUST expose a tool to retrieve the user(s)
  currently on call for a given schedule, including each user's
  identifier, display name, and shift end time. *(inherited.)*
- **FR-013**: The server MUST expose a tool to retrieve a single
  on-call shift by ID. *(inherited.)*
- **FR-014**: The server MUST expose a tool to list teams. *(inherited.)*
- **FR-015**: The server MUST expose a tool to list users, filterable
  by username. *(inherited.)*
- **FR-016**: The server MUST expose a tool to list alert groups with
  filters for state, integration, route, team, time range, labels, and
  free text. *(inherited.)*
- **FR-017**: The server MUST expose a tool to retrieve a single alert
  group by ID, including its current state, severity, integration,
  creation time, and member alert counts. *(inherited.)*
- **FR-020**: The server MUST expose a tool to acknowledge an alert
  group. *(inherited.)*
- **FR-021**: The server MUST expose a tool to resolve an alert group.
  *(inherited.)*
- **FR-022**: The server MUST expose a tool to silence an alert group
  for a caller-specified duration. *(inherited.)*
- **FR-023**: The server MUST expose a tool to unacknowledge or unresolve
  an alert group where the underlying API supports it. *(inherited.)*
- **FR-024**: The server MUST support a read-only mode (toggled via
  configuration) that disables every write tool; in this mode write
  tools MUST be absent from the tool list rather than
  present-but-erroring. *(inherited.)*

#### Tool contract and consistency (inherited; unchanged)

- **FR-030**: Tool names MUST follow `<verb>_<resource>` snake_case.
- **FR-031**: All tools MUST declare and validate input and output JSON
  schemas; unknown or malformed inputs MUST be rejected with a
  structured validation error before any outbound call is made.
- **FR-032**: All tools MUST use identical parameter names, casing, and
  units for the same concept (timestamps as ISO 8601 UTC strings,
  durations as ISO 8601, IDs as strings, pagination via a shared cursor
  parameter).
- **FR-033**: Every error response MUST include a stable error code, a
  human-readable message, the failing tool name, and — where applicable
  — an actionable hint.
- **FR-034**: List-style tools MUST return a pagination cursor when more
  results exist and MUST accept that cursor on subsequent calls.
- **FR-035 (new)**: The error envelope's stable code for the
  "neither plugin installed" case MUST be `ONCALL_PLUGIN_MISSING`
  (replacing the prior `IRM_PLUGIN_MISSING` code, which presupposed a
  single target). The hint MUST name both plugin IDs.

#### Reliability and performance (inherited; unchanged)

- **FR-040**: Read-only tool calls MUST respond within 2 seconds at p95
  under nominal load (up to 10 concurrent requests).
- **FR-041**: Every outbound call to the Grafana OnCall API MUST have an
  explicit timeout (default 10 seconds) and MUST apply exponential
  backoff with jitter on retryable failures (HTTP 429 and 5xx), up to
  3 retries.
- **FR-042**: The server MUST surface upstream rate-limit signals
  (HTTP 429 with `Retry-After`) to the caller in a structured form
  instead of silently blocking.
- **FR-043**: Per-request peak memory MUST remain under 100 MB.

#### Observability and security (inherited; FR-050/051/052 unchanged)

- **FR-050**: The server MUST emit structured logs for every tool
  invocation, including tool name, duration, outcome, and a correlation
  identifier — without leaking secrets, full bearer tokens, or alert
  payload contents.
- **FR-051**: The server MUST redact credentials in any error or debug
  output.
- **FR-052**: The server MUST refuse to start if the configured base URL
  uses an unencrypted scheme against a non-local host.
- **FR-053 (new)**: The startup log line that announces the selected
  plugin MUST be emitted at `INFO` level; the warning emitted when
  both plugins are present and the legacy one is preferred MUST be
  emitted at `WARN` level.

### Key Entities

- **Plugin**: One of two supported Grafana OnCall plugin IDs —
  `grafana-oncall-app` (legacy, archived upstream) or `grafana-irm-app`
  (rebranded). Selected exactly once at startup. Identified by the
  internal short codes `oncall-app` and `irm` in operator-facing config
  and logs.
- **OnCallAPIBase**: The resolved HTTP base URL the server routes
  OnCall calls through. Shape differs per plugin:
  - For `oncall-app`: `{GRAFANA_URL}/api/plugins/grafana-oncall-app/resources/api/v1/`
  - For `irm`: the `jsonData.onCallApiUrl` extracted from
    `GET {GRAFANA_URL}/api/plugins/grafana-irm-app/settings`, with
    `api/v1/` appended.
- **Preference**: Operator-supplied plugin selection override, sourced
  from `GRAFANA_ONCALL_PLUGIN_PREFERENCE` env var or
  `--plugin-preference` flag. Values: `oncall-app` | `irm` | unset.
- **Schedule, Shift, User, Team, AlertGroup, Integration**: Inherited
  from 001-oncall-mcp-server without shape changes; the dual-plugin
  support does not alter the data model the server exposes to clients.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A new operator with a `grafana-irm-app`-only Grafana can
  go from "credentials in hand" to a first successful
  `list_oncall_teams` response in under 10 minutes, with zero source
  edits and zero tool-name or prompt rewrites compared to the
  IRM-targeted upstream `mcp-grafana` flow.
- **SC-002**: A new operator with a legacy `grafana-oncall-app`
  Grafana can go from "credentials in hand" to a first successful
  `list_oncall_teams` response in under 10 minutes, with zero source
  edits and the same 11 tool names regardless of which plugin is
  installed on the target Grafana.
- **SC-003**: For 10 concurrent read requests spanning the four
  most-used tools against an IRM Grafana, p95 latency stays at or
  below 2 seconds; the same load against a legacy `grafana-oncall-app`
  Grafana also stays at or below 2 seconds. Neither path adds more
  than 100 ms of overhead vs. the single-plugin build.
- **SC-004**: When the configured Grafana hosts BOTH plugins, the
  server's startup log includes a single line stating which plugin it
  selected and why, so an operator can audit the decision in
  ≤ 5 seconds of log review.
- **SC-005**: When the configured Grafana hosts NEITHER plugin, the
  server fails to start within 1 second of probe completion, and the
  emitted `ONCALL_PLUGIN_MISSING` error message names both
  `grafana-oncall-app` and `grafana-irm-app` plus the operator-install
  hint in 100% of trials.
- **SC-006**: An existing `mcp-grafana` prompt that calls the 5
  upstream tool names works unchanged when the client is re-pointed at
  this server, in 100% of trials on an IRM Grafana; the same prompt
  works unchanged on a legacy `grafana-oncall-app` Grafana, in 100% of
  trials.
- **SC-007**: At least 80% of consumers (measured via documentation
  feedback or integration support tickets) report that the
  tool-name, parameter-name, and error-envelope surface is identical
  between the two plugin paths, so they "don't have to special-case
  the plugin in their prompt."

## Assumptions

- The two plugins (`grafana-oncall-app` and `grafana-irm-app`) expose
  materially the same OnCall data model and write-action surface,
  differing only in the URL shape and authentication header convention
  required to reach them. This was true of the upstream
  `amixr-api-go-client` at the time of the previous feature's
  completion and is presumed to remain true.
- The operator runs this server on the same host (or at least the same
  trust boundary) as the Grafana instance they are pointing it at;
  cross-trust-boundary deployments and multi-tenant routing remain out
  of scope (inherited assumption from 001-oncall-mcp-server).
- The default preference of `oncall-app` is chosen because (a) the
  legacy plugin is the project's original target and (b) operators
  self-hosting both plugins are typically mid-migration off the legacy
  plugin and would prefer the stable surface they already trust. If
  the project's user base shifts toward IRM-first deployments, the
  default can be flipped in a future minor release with a deprecation
  warning.
- The OnCall API base URL for `grafana-oncall-app` is
  `{GRAFANA_URL}/api/plugins/grafana-oncall-app/resources/api/v1/`
  (i.e. the Grafana resource proxy at the legacy plugin's
  well-known path). This URL shape is the upstream convention used by
  the `amixr-api-go-client` and is presumed stable; the implementation
  will verify this assumption via a HEAD/GET probe at startup and
  surface `ONCALL_PLUGIN_MISSING` with a specific hint if the
  resolved URL does not return 2xx for the user-listing endpoint.
- Mid-session plugin selection is intentionally NOT supported: the
  selected plugin is a startup decision and the server does not
  silently re-route to a different plugin if the operator changes the
  Grafana configuration under the running process. This is a
  deliberate choice for predictability and auditability; the cost is
  that operators who change plugins must restart the server.
- A dedicated single-schedule-detail tool is intentionally not
  provided; callers compose `list_oncall_schedules` with
  `get_oncall_shift` for per-shift information. Inherited from
  001-oncall-mcp-server without change.
- Only Grafana OnCall functionality is in scope. Other Grafana
  surfaces (dashboards, datasources, alerting rules, Loki/Prometheus
  query passthrough, incidents/Sift, navigation deeplinks, rendering,
  provisioning, admin) remain explicitly out of scope and MUST NOT be
  implemented by this server. Inherited.

## Clarifications

### Session 2026-06-09

- Q: Should the server target the legacy `grafana-oncall-app` plugin,
  the rebranded `grafana-irm-app` plugin, or both? → A: Both. The
  server auto-detects which plugin is installed on the target Grafana
  and selects exactly one at startup. A new operator override
  `GRAFANA_ONCALL_PLUGIN_PREFERENCE` (or `--plugin-preference` flag)
  lets operators force a specific plugin when both are installed;
  when the override names a plugin that is NOT installed, startup
  fails with `INVALID_CONFIG`. The error code for "neither plugin
  installed" is renamed from `IRM_PLUGIN_MISSING` to
  `ONCALL_PLUGIN_MISSING`, and its hint names both plugin IDs. When
  both plugins are present and no preference is set, the server
  prefers `grafana-oncall-app` (the legacy plugin) and logs a
  `WARN`-level line explaining the choice.
