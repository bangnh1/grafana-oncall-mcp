# Tasks: Grafana OnCall MCP Server

**Input**: Design documents from `/specs/001-oncall-mcp-server/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/, quickstart.md

**Tests**: Tests are OPTIONAL per spec — only contract tests are included because they are mandated by the project constitution (Principle II) and plan.md.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

- Go single-binary CLI layout rooted at the repository top-level (`cmd/`, `internal/`, `tests/`, `contracts/`)

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Go module initialization and project skeleton matching the implementation plan.

- [x] T001 Initialize Go module at go.mod with module path github.com/bangnh1/grafana-oncall-mcp and Go 1.26.x
- [x] T002 Add Go dependencies to go.mod: mark3labs/mcp-go v0.46.0, grafana/amixr-api-go-client v0.0.28, invopop/jsonschema v0.13.0, stretchr/testify v1.11.1, go.opentelemetry.io/otel v1.43.0, and remaining dependencies from plan.md Technical Context
- [x] T003 [P] Create top-level directory skeleton: cmd/oncall-mcp/, internal/{config,oncall,tools,server,obs}/, tests/{contract,integration,e2e}/
- [x] T004 [P] Add .gitignore entries for Go build artifacts, vendor/, .env, coverage.out, and IDE files
- [x] T005 [P] Add golangci-lint v2 configuration to .golangci.yaml mirroring the linters listed in plan.md (gofmt, govet, staticcheck, gocyclo, depguard, sloglint)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented.

**CRITICAL**: No user story work can begin until this phase is complete.

- [x] T006 [P] Define DTO structs and mapping helpers in internal/oncall/dtos.go for Schedule, Shift, UserSummary, User, Team, AlertGroupSummary, AlertGroup, Integration
- [x] T007 [P] Generate and add all 22 tool JSON Schema files to contracts/ (one input + one output per tool, using shared types from contracts/_defs.schema.json), matching the schemas in contracts/
- [x] T008 [P] Define error codes and envelope struct in internal/oncall/errors.go mapping to contracts/error_envelope.schema.json (INVALID_INPUT, NOT_FOUND, UNAUTHENTICATED, FORBIDDEN, UPSTREAM_RATE_LIMITED, UPSTREAM_UNAVAILABLE, UPSTREAM_TIMEOUT, IRM_PLUGIN_MISSING, INTERNAL, READ_ONLY_MODE, STATE_TRANSITION_REJECTED)
- [x] T009 [P] Implement retry/backoff logic in internal/oncall/retry.go with exponential backoff + jitter, honoring upstream Retry-After, max 3 retries, 10s default timeout per plan.md FR-041
- [x] T010 [P] Implement environment-driven config struct in internal/config/config.go with redacted Stringer, covering GRAFANA_URL, GRAFANA_SERVICE_ACCOUNT_TOKEN, GRAFANA_API_KEY, GRAFANA_ONCALL_READ_ONLY, GRAFANA_ONCALL_HTTP_TIMEOUT, GRAFANA_ONCALL_MAX_RETRIES, OTEL_* passthrough
- [x] T011 [P] Implement OnCall client wrapper in internal/oncall/client.go: BuildTransport() helper mirroring upstream, IRM plugin discovery via /api/plugins/grafana-irm-app/settings, and typed service accessors (ScheduleService, OnCallShiftService, UserService, TeamService, AlertGroupService)
- [x] T012 [P] Add contract validation test file tests/contract/jsonschema_test.go that validates tool I/O against contracts/*.schema.json using invopop/jsonschema, covering all 11 tools plus error_envelope.schema.json
- [x] T013 [P] Implement redacted structured logging in internal/obs/logging.go using log/slog with a custom handler that redacts token/apiKey/authorization/password keys before emission, satisfying FR-050/FR-051
- [x] T014 [P] Implement OpenTelemetry setup in internal/obs/otel.go covering traces, metrics, and logs, mirroring upstream package versions, off by default and activated via env or flags
- [x] T015 [P] Implement transport selection and wiring in internal/server/transport.go for stdio (server.NewStdioServer), SSE (server.NewSSEServer), and streamable-http (server.NewStreamableHTTPServer) with --transport flag dispatch
- [x] T016 [P] Implement read-only mode suppression in internal/server/readonly.go: filter write tools from the tool registry when GRAFANA_ONCALL_READ_ONLY=true or --read-only is set, satisfying FR-024 and SC-007
- [x] T017 Implement server construction in internal/server/server.go: MCP server wiring, hook registration (slow-request logger via --slow-request-threshold, metrics hook), tool aggregator AddOnCallTools(mcp), and startup sequence
- [x] T018 Implement CLI entrypoint in cmd/oncall-mcp/main.go: stdlib flag parsing for --transport, --address, --base-path, --endpoint-path, --read-only, --log-level, --debug, --metrics, --metrics-address, --slow-request-threshold, --slow-request-log-level, --session-idle-timeout-minutes, -version; load config, run startup checks, serve

**Checkpoint**: Foundation ready — user story implementation can now begin in parallel.

---

## Phase 3: User Story 1 - Inspect on-call coverage during an incident (Priority: P1) 🎯 MVP

**Goal**: Retrieve schedules, shifts, and current on-call users so an AI assistant can answer "who's on call right now?" during an incident.

**Independent Test**: With a populated test Grafana OnCall instance, call list_oncall_schedules, pick a schedule, call get_oncall_shift, then call get_current_oncall_users — all return correct names, handles, and shift end times matching the OnCall UI.

### Tests for User Story 1

- [x] T019 [P] [US1] Add unit tests for schedule and shift DTO mapping in internal/oncall/dtos_test.go
- [x] T020 [P] [US1] Add unit tests for list_oncall_schedules handler with mocked amixr client in internal/tools/schedules_test.go
- [x] T021 [P] [US1] Add unit tests for get_oncall_shift handler with mocked amixr client in internal/tools/schedules_test.go
- [x] T022 [P] [US1] Register list_oncall_schedules tool in internal/tools/schedules.go: input validation, cursor/limit handling, API call, DTO mapping to contracts/list_oncall_schedules.output.schema.json
- [x] T023 [P] [US1] Register get_oncall_shift tool in internal/tools/schedules.go: shift_id input validation, API lookup, DTO mapping to contracts/get_oncall_shift.output.schema.json

**Checkpoint**: User Story 1 is fully functional — an agent can discover schedules and read shift details.

---

## Phase 4: User Story 2 - Triage and act on active alert groups (Priority: P1)

**Goal**: Fetch alert group details and perform state transitions (acknowledge, resolve, silence, unresolve) so an AI assistant can act on incidents on behalf of an engineer.

**Independent Test**: With a known alert group in a test instance, call get_alert_group for details, then call acknowledge_alert_group and resolve_alert_group — the OnCall UI reflects the transitions within 5 seconds and attributes them to the configured identity.

### Tests for User Story 2

- [x] T024 [P] [US2] Add unit tests for AlertGroup DTO mapping and state-transition validation in internal/oncall/dtos_test.go and internal/tools/alert_groups_write_test.go
- [x] T025 [P] [US2] Add unit tests for all four write handlers with mocked amixr client in internal/tools/alert_groups_write_test.go (ack, resolve, silence, unresolve — including idempotent and STATE_TRANSITION_REJECTED cases)
- [x] T026 [P] [US2] Register acknowledge_alert_group tool in internal/tools/alert_groups_write.go: alert_group_id validation, POST /acknowledge/, WriteResult DTO mapping to contracts/acknowledge_alert_group.output.schema.json
- [x] T027 [P] [US2] Register resolve_alert_group tool in internal/tools/alert_groups_write.go: alert_group_id validation, POST /resolve/, WriteResult DTO mapping to contracts/resolve_alert_group.output.schema.json
- [x] T028 [P] [US2] Register silence_alert_group tool in internal/tools/alert_groups_write.go: alert_group_id + until validation, POST /silence/, WriteResult + silenced_until mapping to contracts/silence_alert_group.output.schema.json
- [x] T029 [P] [US2] Register unresolve_alert_group tool in internal/tools/alert_groups_write.go: alert_group_id validation, POST /unresolve/, WriteResult DTO mapping to contracts/unresolve_alert_group.output.schema.json
- [x] T030 [US2] Wire write tool registration into internal/server/readonly.go so write tools are excluded when read-only mode is active

**Checkpoint**: User Story 2 is fully functional — an agent can fetch alert group details and transition their states.

---

## Phase 5: User Story 3 - Discover schedules, teams, and users (Priority: P2)

**Goal**: Enumerate teams, users, and on-call coverage so an agent can answer "what schedules exist and who is on each rotation?"

**Independent Test**: An agent calls list_oncall_teams, then list_oncall_schedules filtered by a team ID, then get_current_oncall_users for one schedule — results match the OnCall UI for names, IDs, and member counts.

### Tests for User Story 3

- [x] T031 [P] [US3] Add unit tests for UserSummary, User, and Team DTO mapping in internal/oncall/dtos_test.go
- [x] T032 [P] [US3] Add unit tests for get_current_oncall_users handler with mocked amixr client in internal/tools/users_test.go
- [x] T033 [P] [US3] Add unit tests for list_oncall_teams handler with mocked amixr client in internal/tools/teams_test.go
- [x] T034 [P] [US3] Add unit tests for list_oncall_users handler with mocked amixr client in internal/tools/users_test.go

### Implementation for User Story 3

- [x] T035 [P] [US3] Register get_current_oncall_users tool in internal/tools/users.go: schedule_id validation, API call, UserSummary list + shift_end_at mapping to contracts/get_current_oncall_users.output.schema.json (empty users list treated as "no coverage", not error)
- [x] T036 [P] [US3] Register list_oncall_teams tool in internal/tools/teams.go: cursor/limit handling, API call, Team DTO mapping to contracts/list_oncall_teams.output.schema.json
- [x] T037 [P] [US3] Register list_oncall_users tool in internal/tools/users.go: optional username exact-match filter, cursor/limit handling, API call, User DTO mapping to contracts/list_oncall_users.output.schema.json

**Checkpoint**: User Story 3 is fully functional — an agent can enumerate teams, users, and current coverage independently.

---

## Phase 6: User Story 4 - Filter and search alert groups (Priority: P2)

**Goal**: Query alert groups by state, integration, route, team, labels, time range, and free text so an agent can surface targeted triage views.

**Independent Test**: Given known alert groups across states, integrations, labels, and times, the agent calls list_alert_groups with each filter and receives only matching results with correct pagination cursors.

### Tests for User Story 4

- [x] T038 [P] [US4] Add unit tests for list_alert_groups filter mapping and pagination in internal/tools/alert_groups_read_test.go
- [x] T039 [P] [US4] Add unit tests for get_alert_group handler with mocked amixr client in internal/tools/alert_groups_read_test.go

### Implementation for User Story 4

- [x] T040 [P] [US4] Register list_alert_groups tool in internal/tools/alert_groups_read.go: filter params (state, integration_id, route_id, team_id, labels, started_at_from, started_at_to, search), cursor/limit, API call, AlertGroupSummary DTO mapping to contracts/list_alert_groups.output.schema.json
- [x] T041 [P] [US4] Register get_alert_group tool in internal/tools/alert_groups_read.go: alert_group_id validation, API call, full AlertGroup DTO mapping to contracts/get_alert_group.output.schema.json

**Checkpoint**: User Story 4 is fully functional — an agent can filter and search alert groups with pagination.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Reliability hardening, operator experience, and end-to-end verification.

 - [x] T042 Implement startup health checks in cmd/oncall-mcp/main.go: config sanity (HTTPS enforcement per FR-052, token presence), IRM plugin probe with legacy-plugin detection, OnCall API reachability check — exit non-zero with structured stderr on failure
- [x] T043 [P] Add Prometheus /metrics endpoint in internal/obs/otel.go (or internal/server/server.go) activated via --metrics flag, off by default, mirroring upstream behavior
- [x] T044 [P] Add integration test scaffolding in tests/integration/docker-compose.yaml with grafana-irm-app seed data and tests/integration/oncall_reads_test.go covering read tools against a live instance
- [x] T045 [P] Add integration test file tests/integration/oncall_writes_test.go covering write tools against a live grafana-irm-app instance
- [x] T046 [P] Add end-to-end test file tests/e2e/mcp_client_test.go that spawns the binary in stdio mode and drives tool calls via an mcp-go client
- [x] T047 Write README.md covering prerequisites, env vars, build, transport modes, RBAC notes, and troubleshooting
- [x] T048 Add Makefile with targets: build, test, test-unit, test-integration, lint, run, run-sse, build-image
- [x] T049 Add Dockerfile mirroring upstream shape: golang:1.26-bookworm builder → debian:bookworm-slim final stage, CGO_ENABLED=0, multi-arch via BUILDPLATFORM/TARGETPLATFORM
- [x] T050 Validate quickstart.md smoke-test commands (go test ./..., golangci-lint run, transport modes) by executing them against the completed implementation

**Final Checkpoint**: All 11 tools registered, all contracts validated, all acceptance scenarios from spec.md independently testable.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion — BLOCKS all user story phases
- **User Stories (Phase 3–6)**: All depend on Foundational phase completion; can then proceed in priority order (P1 → P2) or in parallel if staffed
- **Polish (Phase 7)**: Depends on all desired user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Foundational — no dependencies on other stories; this is the MVP
- **User Story 2 (P1)**: Can start after Foundational — independently testable via its own alert group lifecycle
- **User Story 3 (P2)**: Can start after Foundational — independently testable via team/schedule/user enumeration
- **User Story 4 (P2)**: Can start after Foundational — independently testable via alert-group filtering

### Within Each User Story

- Tests (if included) are written to FAIL before the corresponding implementation
- DTOs and helper structs (Phase 2) before tool handlers
- Read handlers before write handlers within the same domain
- Story complete before moving to the next priority

### Key Cross-Stage Dependencies

- T007 (contract schemas) → T012 (contract validation test)
- T006 (DTOs) → T019–T041 (all tool implementations)
- T011 (client wrapper) → T022–T041 (all tool implementations)
- T017 (server wiring) → T018 (CLI entrypoint)
- T016 (read-only suppression) → T026–T029 (write tools registered conditionally)

---

## Parallel Opportunities

### Phase 1 (Setup)
- T002, T003, T004, T005 can all run in parallel — different files, no dependencies

### Phase 2 (Foundational)
- T006, T007, T008, T009, T010, T011, T012, T013, T014, T015, T016 can all run in parallel — different files, no dependencies within the phase
- T017 depends on T009 (errors), T010 (config), T011 (client)
- T018 depends on T017

### Phase 3 (US1)
- T019, T020, T021 can run in parallel (different test files)
- T022, T023 can run in parallel (different tool registrations within the same file — merge carefully)

### Phase 4 (US2)
- T024, T025 can run in parallel (different test functions within the same file)
- T026, T027, T028, T029 can run in parallel (different tool registrations within the same file — merge carefully)
- T030 depends on T026–T029

### Phase 5 (US3)
- T031, T032, T033, T034 can run in parallel (different test files)
- T035, T036, T037 can run in parallel (different files)

### Phase 6 (US4)
- T038, T039 can run in parallel (different test functions)
- T040, T041 can run in parallel (different tool registrations within the same file — merge carefully)

### Phase 7 (Polish)
- T043, T044, T045, T046, T047, T048, T049 can run in parallel — different files, no dependencies

---

## Parallel Example: User Story 1

```
# Launch all US1 test tasks together:
T019: internal/oncall/dtos_test.go
T020: internal/tools/schedules_test.go
T021: internal/tools/schedules_test.go

# Launch all US1 implementation tasks together (same file — serialize merges):
T022: internal/tools/schedules.go (list_oncall_schedules)
T023: internal/tools/schedules.go (get_oncall_shift)
```

---

## Implementation Strategy

### MVP First (User Stories 1 + 2 — P1)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (CRITICAL — blocks all stories)
3. Complete Phase 3: User Story 1 — on-call coverage lookup
4. Complete Phase 4: User Story 2 — alert group triage and state transitions
5. **STOP and VALIDATE**: Test US1 + US2 independently against a test Grafana instance
6. Deploy/demo if ready — this is the full MVP

### Incremental Delivery

1. Setup + Foundational → Foundation ready
2. Add US1 → Test independently → Deploy/Demo
3. Add US2 → Test independently → Deploy/Demo
4. Add US3 → Test independently → Deploy/Demo
5. Add US4 → Test independently → Deploy/Demo
6. Each story adds value without breaking previous stories

### Parallel Team Strategy

With multiple developers:

1. Team completes Setup + Foundational together
2. Once Foundational is done:
   - Developer A: User Story 1 (P1)
   - Developer B: User Story 2 (P1)
   - Developer C: User Story 3 (P2)
   - Developer D: User Story 4 (P2)
3. Stories complete and integrate independently

---

## Summary

| Metric | Value |
|---|---|
| **Total tasks** | 50 |
| **Setup (Phase 1)** | 5 |
| **Foundational (Phase 2)** | 13 |
| **User Story 1 (P1)** | 5 |
| **User Story 2 (P1)** | 7 |
| **User Story 3 (P2)** | 7 |
| **User Story 4 (P2)** | 5 |
| **Polish (Phase 7)** | 9 |

### Task Count per User Story

| Story | Tasks | Independent Test Criteria |
|---|---|---|
| US1 | 5 | list_oncall_schedules + get_oncall_shift + get_current_oncall_users return correct names, handles, and shift end times |
| US2 | 7 | get_alert_group details + acknowledge/resolve/silence/unresolve transitions visible in OnCall UI within 5 seconds |
| US3 | 7 | list_oncall_teams, list_oncall_schedules (team filter), get_current_oncall_users return IDs and names matching OnCall UI |
| US4 | 5 | list_alert_groups with each filter returns only matching groups with correct pagination cursors |

### Parallel Opportunities Identified

- **Phase 1**: 4 of 5 tasks are [P]
- **Phase 2**: 11 of 13 tasks are [P] (only T017 and T018 are sequential)
- **US1**: 3 of 5 tasks are [P]
- **US2**: 5 of 7 tasks are [P]
- **US3**: 5 of 7 tasks are [P]
- **US4**: 3 of 5 tasks are [P]
- **Phase 7**: 7 of 9 tasks are [P]

### Suggested MVP Scope

Complete Phases 1–4 (Setup + Foundational + US1 + US2). This delivers the two P1 user stories: on-call coverage lookup and alert group triage/action. Phases 5–6 (discovery + filtering) are P2 enhancements. Phase 7 (polish) can overlap with P2 delivery.

### Format Validation

All 50 tasks follow the checklist format:
- `- [ ]` checkbox prefix: confirmed
- Sequential Task ID (T001–T050): confirmed
- `[P]` marker on parallelizable tasks only: confirmed
- `[US1]`/`[US2]`/`[US3]`/`[US4]` story labels on user story phase tasks only: confirmed
- Exact file paths in every description: confirmed
