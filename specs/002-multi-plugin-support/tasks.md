# Tasks: Multi-Plugin OnCall Support

**Input**: Design documents from `/specs/002-multi-plugin-support/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/, quickstart.md
**Inherits**: `/specs/001-oncall-mcp-server/tasks.md` (T001–T050) — every task there is already `[x]`; this document defines only the additive tasks T051+ for dual-plugin support.

**Tests**: Tests are OPTIONAL per spec — only contract tests and the dual-plugin unit tests are included because they are mandated by the project constitution (Principle II) and the dual-plugin behavior in `plan.md`.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story. The dual-plugin change is intentionally small and additive: it touches a single new file (`internal/oncall/plugin.go`), the existing startup probe (`cmd/oncall-mcp/main.go::startUpHealthCheck` and `internal/oncall/client.go::resolveOnCallURL`), the env-var/flag surface (`internal/config/config.go`), the error envelope (one rename in `internal/oncall/errors.go` + JSON schema), the docker-compose matrix (`tests/integration/docker-compose.yaml`), and one new CLI flag in `cmd/oncall-mcp/main.go`. No new MCP tools; the 11 existing tools, DTOs, contracts, and per-tool handlers are reused as-is.

**Reuse note**: The 50 tasks from `001-oncall-mcp-server/tasks.md` (T001–T050) are all already complete (`[x]`) in this repository. The 11 MCP tool handlers, DTOs, error envelope, server wiring, transports, read-only mode, observability stack, and contract tests are reused without modification. Only the 9 tasks below (T051–T059) are required to ship feature 002.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3, US4)
- Include exact file paths in descriptions

## Path Conventions

- Go single-binary CLI layout rooted at the repository top-level (`cmd/`, `internal/`, `tests/`, `contracts/`)
- New file: `internal/oncall/plugin.go`
- Touched files: `internal/oncall/client.go`, `internal/oncall/errors.go`, `internal/config/config.go`, `cmd/oncall-mcp/main.go`, `specs/002-multi-plugin-support/contracts/error_envelope.schema.json`, `tests/integration/docker-compose.yaml`, `tests/integration/oncall_reads_test.go`, `tests/integration/oncall_writes_test.go`

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: No new setup work for 002 — Go module, top-level directory skeleton, gitignore, golangci-lint config from 001/T001–T005 are reused unchanged.

**Reused from 001**: T001, T002, T003, T004, T005 — all `[x]`.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: No new foundational work for 002 — DTOs, contracts, error envelope struct, retry helpers, config struct, OnCall client wrapper, contract tests, logging, OTel, transport, read-only mode, server wiring, and CLI entrypoint from 001/T006–T018 are reused.

**Reused from 001**: T006, T007, T008, T009, T010, T011, T012, T013, T014, T015, T016, T017, T018 — all `[x]`.

**Note**: The 11 MCP tool handlers and the 22 per-tool JSON Schemas are also reused unchanged. The dual-plugin change does not alter the wire format the server exposes to MCP clients (US4 of 002 explicitly requires this), so the per-tool `contracts/*.json` files are NOT modified — only the shared `error_envelope.schema.json` gets one new enum value plus a deprecated alias.

---

## Phase 3: User Story 1 - Operator runs the server against an IRM-only Grafana (Priority: P1) 🎯 MVP

**Goal**: With only `grafana-irm-app` installed on the target Grafana, the server starts, logs `plugin=irm` at `INFO`, and serves all 11 tools against the IRM OnCall API URL extracted from the plugin settings — same tool names, same response shape, no operator configuration beyond what 001 already requires.

**Independent Test**: Point the server at a Grafana that hosts only `grafana-irm-app`; the startup log includes one `INFO` line with `plugin=irm`; the resolved OnCall API base URL matches the URL reported in `/api/plugins/grafana-irm-app/settings::jsonData.onCallApiUrl`; the 11 tools respond correctly to a smoke call (e.g., `list_oncall_teams`).

### Tests for User Story 1

- [x] T051 [P] [US1] Add unit tests for the dual-plugin selection algorithm in `internal/oncall/plugin_test.go`: 9-row table (preference × legacy-installed × irm-installed → expected selected plugin + expected error code) covering all branches of `research.md` Decision 3; reuse existing `testify` setup from `internal/oncall/dtos_test.go` style.
- [x] T052 [P] [US1] Add unit tests for the per-plugin OnCall API base URL resolver in `internal/oncall/plugin_test.go`: assert `oncall-app` resolves to `{GRAFANA_URL}/api/plugins/grafana-oncall-app/resources/api/v1/` (passed without trailing `api/v1/`) and `irm` resolves to `jsonData.onCallApiUrl` from the mocked settings response; reuse existing `testify` HTTP-test pattern.
- [x] T053 [P] [US1] Create `internal/oncall/plugin.go` (NEW FILE) with: `Plugin` type (`PluginOnCallApp` | `PluginIRM` constants, `String()` method, `PluginID()` returning `grafana-oncall-app` or `grafana-irm-app`); `PluginPreference` type (`PluginPrefOnCallApp` | `PluginPrefIRM` | `PluginPrefUnset` zero value); `func ParsePluginPreference(s string) (PluginPreference, error)` accepting `oncall-app` | `irm` | `""` and returning `INVALID_CONFIG` for any other value (reuse the `ErrorCode` enum from `internal/oncall/errors.go`); `func SelectAndResolve(ctx context.Context, grafanaURL, token string, pref PluginPreference) (Plugin, string, error)` that issues the two parallel `/settings` probes, applies the precedence table from `research.md` Decision 3, returns the selected plugin + resolved OnCall API base URL; doc comments on every exported symbol.
- [x] T054 [US1] Refactor `internal/oncall/client.go::resolveOnCallURL` to call into `SelectAndResolve` from `internal/oncall/plugin.go` (T053), preserving the existing return type `(string, error)`. **Reuse the existing `httpClient`, the existing `aapi.NewWithGrafanaURL(baseURL, token, grafanaURL)` construction, and the existing `BuildTransport()` parity** — only the URL-resolution body is replaced. The function MUST still fail closed: if `SelectAndResolve` returns an error (no plugin installed, preference points to a missing plugin, or both probes non-2xx/404), `resolveOnCallURL` MUST return the error without falling back.
- [x] T055 [P] [US1] Replace the error constant `ErrCodeIRMPluginMissing` in `internal/oncall/errors.go` with `ErrCodeOnCallPluginMissing ErrorCode = "ONCALL_PLUGIN_MISSING"`; keep `ErrCodeIRMPluginMissing` as a deprecated alias (`ErrorCode = "IRM_PLUGIN_MISSING"`) that is never emitted by the server but remains in the public API for one minor release (constitution Principle III deprecation window); update any internal callers in `internal/oncall/client.go` (introduced by T054) to use the new name.
- [x] T056 [P] [US1] Update `specs/002-multi-plugin-support/contracts/error_envelope.schema.json` (already in place from Phase 1 of this plan) to add `IRM_PLUGIN_MISSING` to the `code` enum as a deprecated alias of `ONCALL_PLUGIN_MISSING`; update the schema `description` to state "The server never emits IRM_PLUGIN_MISSING; it is retained as a deprecated alias for ONCALL_PLUGIN_MISSING for one minor release per constitution Principle III". **Reuse the existing JSON Schema structure verbatim** — only the enum array and description change.
- [x] T057 [P] [US1] Update `tests/contract/jsonschema_test.go` (existing 001 test) to assert that BOTH `ONCALL_PLUGIN_MISSING` and `IRM_PLUGIN_MISSING` are accepted as valid `code` values; reuse the existing contract test harness. Add a single new test case that asserts an envelope with `code=IRM_PLUGIN_MISSING` passes validation and that an envelope with `code=ONCALL_PLUGIN_MISSING` also passes validation; this anchors the deprecation-alias contract.

**Checkpoint**: User Story 1 is fully functional — an agent can connect to a server started against an IRM-only Grafana and use all 11 tools with no behavior change vs. 001.

---

## Phase 4: User Story 2 - Operator runs the server against the legacy `grafana-oncall-app` (Priority: P1)

**Goal**: With only `grafana-oncall-app` installed, the server starts, logs `plugin=oncall-app` at `INFO`, and serves all 11 tools against the legacy OnCall resource-proxy URL — same tool names, same response shape, no operator configuration beyond what 001 already requires.

**Independent Test**: Point the server at a Grafana that hosts only the legacy `grafana-oncall-app`; the startup log includes one `INFO` line with `plugin=oncall-app`; the resolved OnCall API base URL is `{GRAFANA_URL}/api/plugins/grafana-oncall-app/resources/api/v1/`; the 11 tools respond correctly to a smoke call (e.g., `list_oncall_teams`).

### Tests for User Story 2

- [x] T058 [P] [US2] Add unit tests for the legacy `grafana-oncall-app` URL resolution path in `internal/oncall/plugin_test.go` (extend T051/T052): assert that when only the legacy probe returns 200, the resolver returns `(PluginOnCallApp, <expected URL>)`; reuse the existing parallel-probe test pattern.

### Implementation for User Story 2

> **Reuse note**: The implementation for US2 is already substantially covered by T053 (`SelectAndResolve`), T054 (`resolveOnCallURL` refactor), and the dual-probe table in `research.md` Decision 3. The T053 function handles the legacy-only case in the "preference=unset, legacy 200, irm 404" row of the precedence table. No additional implementation tasks are required beyond the US1 work.

> **One small follow-up**: T053's `SelectAndResolve` MUST log the resolved OnCall API base URL with the token redacted (per FR-007) when the legacy plugin is selected, identical to the IRM case. This is already in the T053 task description ("returns the selected plugin + resolved OnCall API base URL" plus the logging requirement inherited from FR-007). If the reviewer decides the log line needs a separate task, append it to T053 rather than creating a new T-number.

**Checkpoint**: User Story 2 is fully functional — an agent can connect to a server started against a legacy-only Grafana and use all 11 tools with no behavior change vs. the IRM-only path.

---

## Phase 5: User Story 3 - Operator runs the server against a Grafana with BOTH plugins installed (Priority: P2)

**Goal**: With both plugins installed, the server deterministically picks one plugin per the precedence rule (operator preference > default legacy), logs which it selected and why, and refuses to start cleanly when the operator preference names a plugin that isn't installed.

**Independent Test**: With both plugins installed, start the server with no preference — the log shows `INFO plugin=oncall-app` plus `WARN legacy plugin preferred over irm-app per default`. Restart with `GRAFANA_ONCALL_PLUGIN_PREFERENCE=irm` — the log shows `INFO plugin=irm` and no `WARN` line. Restart with `GRAFANA_ONCALL_PLUGIN_PREFERENCE=oncall-app` while only IRM is installed — startup fails with `INVALID_CONFIG`.

### Tests for User Story 3

- [x] T059 [P] [US3] Add unit tests for the operator-preference precedence in `internal/oncall/plugin_test.go` (extend T051): the "preference=irm, only legacy installed" row must return `INVALID_CONFIG`; the "preference=oncall-app, only IRM installed" row must also return `INVALID_CONFIG`; the "preference=foo (invalid value)" row must return `INVALID_CONFIG` with a hint naming the accepted values; the "preference=irm, both installed" row must return `(PluginIRM, …)` with no `WARN` log; the "preference=unset, both installed" row must return `(PluginOnCallApp, …)` and emit exactly one `WARN` line whose body matches the spec's exact text. Reuse the existing `testify` log-capture pattern from `internal/obs/logging_test.go` if present.

### Implementation for User Story 3

> **Reuse note**: T053's `SelectAndResolve` already implements the precedence table; T054 routes the result through the existing `resolveOnCallURL`; T053 also performs the `WARN` log emission in the "both installed, no preference" case. The new env-var and flag wiring is captured in T060 below. The docker-compose matrix growth is captured in T061–T063.

- [x] T060 [US3] Wire the new env-var and flag in `internal/config/config.go` and `cmd/oncall-mcp/main.go`: add `PluginPreference` field to `config.Config`; add `parseStringEnv` helper (reuse the style of `parseBoolEnv` / `parseIntEnv` already in `config.go`) for `GRAFANA_ONCALL_PLUGIN_PREFERENCE`; add a new `flag.String("plugin-preference", "", "Force plugin selection: oncall-app|irm (default: unset, prefer oncall-app)")` to `cmd/oncall-mcp/main.go`; in `main()`, apply the flag-wins-over-env rule identical to `*readOnly` (line 64 of `main.go`): `cfg.PluginPreference = cfg.PluginPreference.WithFlagOverride(*pluginPreference)`; pass `cfg.PluginPreference` to `oncall.NewClient` (which forwards to `SelectAndResolve` from T053). **Reuse** the existing `Validate()` method, the existing redacted `String()` method, and the existing `parseStringEnv` style — no new validation pattern.
- [x] T061 [P] [US3] Update `cmd/oncall-mcp/main.go::startUpHealthCheck` to remove the legacy 001 IRM-only probe and the "legacy plugin detected → error" branch; replace with a single call to `oncall.NewClient(grafanaURL, token, httpTimeout, pluginPreference)` which (via T054) internally calls `SelectAndResolve`; map the returned error to the same `STARTUP_HEALTH_CHECK_FAILED` exit-code path used by 001. **Reuse** the existing 10-second `http.Client` timeout, the existing `User-Agent` header construction, and the existing structured stderr writer (`writeStartupError`).

**Checkpoint**: User Story 3 is fully functional — the operator can override the default plugin selection at runtime, the server logs deterministically, and startup fails cleanly when the preference names a missing plugin.

---

## Phase 6: User Story 4 - Operator migrates an MCP client from the upstream IRM-only server to this one (Priority: P2)

**Goal**: An existing `grafana/mcp-grafana` user can swap the MCP client entry from the upstream server to this server on an IRM Grafana and have every existing prompt work unchanged; the 11 tool names, parameter names, and error-envelope codes are identical to the upstream surface (and to the 001 single-plugin surface).

**Independent Test**: Run an existing prompt that calls `list_oncall_teams`, `get_current_oncall_users`, and `acknowledge_alert_group` against both the upstream server and this server on the same IRM Grafana; tool names, parameter names, and `error_envelope.code` values are identical (within the new alias). The same prompt works against a legacy `grafana-oncall-app` Grafana with no prompt rewrite.

### Tests for User Story 4

> **Reuse note**: US4 is fundamentally a "no-regression" assertion. The contract tests from 001 (`tests/contract/jsonschema_test.go`, extended in T057) are the primary anchor: they assert that every tool's input/output JSON shape is unchanged. The integration test matrix growth in T062/T063 below is the operational anchor: it proves the same client prompt works against both plugin paths.

- [x] T062 [P] [US4] Extend `tests/integration/docker-compose.yaml` to support a parameterized matrix of three plugin installations: add a second docker-compose file `tests/integration/docker-compose.legacy.yaml` (or convert the existing one into a multi-file setup using `docker compose -f ...`) that installs only `grafana-oncall-app` (legacy plugin) with `GF_PLUGINS_ALLOW_LOADING_UNSIGNED PLUGINS: "grafana-oncall-app"`; keep the existing `tests/integration/docker-compose.yaml` for IRM-only; add a `tests/integration/docker-compose.both.yaml` that installs both via `GF_PLUGINS_ALLOW_LOADING_UNSPECIFIED_PLUGINS` or equivalent env vars. **Reuse** the existing `grafana/grafana-oss:11.2.0` base image, the existing `oncall-seed` curl container, the existing `oncall-mcp` build service, and the existing healthcheck; only the plugin-installation env vars and the `volumes` for plugin provisioning change.
- [x] T063 [P] [US4] Add parameterized integration tests that drive the same MCP client prompt against all three plugin fixtures: extend `tests/integration/oncall_reads_test.go` and `tests/integration/oncall_writes_test.go` with a new top-level test function that takes the plugin fixture as a parameter (legacy-only | irm-only | both-installed) and asserts that every tool from the 11-tool set returns the same response shape. **Reuse** the existing `mcp-go` client harness, the existing `os/exec` driver that spawns the binary, the existing seed data, and the existing assertion style — only the per-fixture setup/teardown is new.

### Implementation for User Story 4

> **Reuse note**: US4 has no new implementation tasks beyond the work already captured in T053–T061. The "no prompt rewrite" guarantee is structurally guaranteed by the fact that no wire-format surface (tool names, parameter names, response shapes, error codes excluding the new alias) changed in this feature. If the reviewer wants an explicit end-to-end test that exercises the same MCP client against all three fixtures, append it to T063 rather than creating a new T-number.

**Checkpoint**: User Story 4 is fully functional — an MCP client configured for upstream `mcp-grafana` on an IRM Grafana works unchanged when re-pointed at this server, and the same prompts work unchanged on a legacy `grafana-oncall-app` Grafana.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Documentation update, agent context sync, and final verification that the dual-plugin change is operationally complete.

> **Reuse note**: All 9 polish tasks from 001 (T042–T050) are `[x]`. The dual-plugin change only requires documentation, agent-context, and CI/contract-test updates that anchor the new behavior.

- [x] T064 [P] Update `README.md` to document the new env-var / flag pair: add `GRAFANA_ONCALL_PLUGIN_PREFERENCE` to the env-var table and `--plugin-preference` to the flag table; add a short "Supported plugins" section that explains the dual-plugin auto-detection and the default precedence (legacy first, overridable); **reuse** the existing README structure verbatim — only the new section is added.
- [x] T065 [P] Update `AGENTS.md` (existing auto-managed file at repo root) to add: a new entry in the **Recent Changes** section for `002-multi-plugin-support`; a new bullet in the **Code Style** section for "the startup plugin probe MUST verify at least one of `grafana-oncall-app` or `grafana-irm-app` is installed"; a new bullet for "plugin selection is a startup decision only"; a new bullet for the env-var / flag. **Reuse** the existing AGENTS.md layout verbatim — only the new bullets are added.
- [x] T066 [P] Update `Makefile` to add a `make test-integration-legacy` target that runs the integration tests against the legacy-only docker-compose matrix from T062; **reuse** the existing `test-integration` target structure and the existing `docker compose` invocations.
- [x] T067 [P] Update `tests/integration/docker-compose.yaml` and the new `docker-compose.legacy.yaml` / `docker-compose.both.yaml` from T062 to set `GRAFANA_ONCALL_PLUGIN_PREFERENCE` and (in the both-installed case) verify the warning line in the test; **reuse** the existing per-test `os.Setenv` pattern.
- [x] T068 Validate `specs/002-multi-plugin-support/quickstart.md` smoke-test commands (the "Dual-Plugin Specific Tests" table) by executing them against the completed implementation; confirm the seven rows of the table (legacy-only, irm-only, both-installed with unset, both-installed with `=irm`, both-installed with `=oncall-app`, legacy-only with `=irm`, none-installed) all produce the documented outcome.
- [x] T069 Re-run `golangci-lint run` and `go test ./internal/... ./tests/contract/...` and confirm the new files (`internal/oncall/plugin.go`, `internal/oncall/plugin_test.go`) and the modified files (`internal/oncall/client.go`, `internal/oncall/errors.go`, `internal/config/config.go`, `cmd/oncall-mcp/main.go`) all pass lint + unit + contract tests; **reuse** the existing CI gate from the 001 setup.

**Final Checkpoint**: All 11 tools registered, all contracts validated, all four dual-plugin user stories (IRM-only, legacy-only, both-installed with operator preference, migration-from-upstream) independently testable.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — T001–T005 already `[x]` from 001
- **Foundational (Phase 2)**: Depends on Setup completion — T006–T018 already `[x]` from 001; T051–T069 all build on these existing surfaces without re-touching them
- **User Stories (Phase 3–6)**: All depend on T053 (the `SelectAndResolve` helper) being complete; once T053 is in, US1 (T054–T057), US2 (T058), US3 (T059–T061), and US4 (T062–T063) can proceed in priority order (P1 → P2) or in parallel if staffed
- **Polish (Phase 7)**: Depends on T053–T063 being complete; T064–T069 are documentation and CI work that can run as soon as the implementation tasks land

### User Story Dependencies

- **User Story 1 (P1)**: Depends on T053 only. T054, T055, T056, T057 can run in parallel after T053. This is the minimum viable cut: `SelectAndResolve` exists, the legacy client wiring uses it, the error code is renamed, the schema is updated, and the contract test is extended.
- **User Story 2 (P1)**: Depends on T053 only (the precedence table covers the legacy-only row). T058 is the only new test task.
- **User Story 3 (P2)**: Depends on T053 + T060 (the env-var/flag wiring). T059 (precedence tests) and T061 (rewriting `startUpHealthCheck` to call `oncall.NewClient`) follow.
- **User Story 4 (P2)**: Depends on T053–T061. T062 (matrix docker-compose) and T063 (parameterized integration tests) are the only new tasks; the rest is structurally guaranteed by reusing the unchanged wire-format surface from 001.

### Within Each User Story

- T053 (`SelectAndResolve`) is the keystone — every other implementation task depends on it
- T055 (error code rename) and T056 (schema update) MUST be done together (the server must not emit `IRM_PLUGIN_MISSING` while the schema rejects it)
- T054 (refactor `resolveOnCallURL`) MUST be done after T053; T061 (refactor `startUpHealthCheck`) is an independent cleanup
- T060 (config + flag wiring) MUST be done before T061 (which uses `cfg.PluginPreference`)
- T062 (docker-compose matrix) and T063 (parameterized integration tests) MUST be done together; T063's tests reference T062's compose files

### Key Cross-Stage Dependencies

- T053 (`SelectAndResolve`) → T054 (`resolveOnCallURL` refactor) → T055 (error code rename) + T056 (schema update) + T057 (contract test)
- T053 → T060 (config + flag wiring) → T061 (startUpHealthCheck rewrite)
- T053 → T062 (docker-compose matrix) → T063 (parameterized integration tests)
- T053 + T061 → T064–T067 (docs + agent context + Makefile)
- T064 + T065 + T066 + T067 + T068 + T069 → Final Checkpoint

---

## Parallel Opportunities

### Phase 1 (Setup)

- **No new tasks** — T001–T005 reused from 001.

### Phase 2 (Foundational)

- **No new tasks** — T006–T018 reused from 001.

### Phase 3 (US1)

- T051 and T052 can run in parallel (different test functions in the same `plugin_test.go` file; merge carefully)
- T055 and T056 can run in parallel (different files; one is Go, one is JSON)
- T057 can run in parallel with T055/T056 (different test file)
- T054 must wait for T053

### Phase 4 (US2)

- T058 is the only new task; it extends T051/T052 in the same test file and can run in parallel with T054–T057 if carefully merged

### Phase 5 (US3)

- T059 (test) can run in parallel with T060 (config + flag wiring) — different files
- T061 must wait for T060

### Phase 6 (US4)

- T062 (docker-compose matrix) and T063 (parameterized integration tests) can run in parallel — different files (compose files vs Go test files)
- Both must wait for T061

### Phase 7 (Polish)

- T064, T065, T066, T067, T068, T069 can all run in parallel — different files (README, AGENTS, Makefile, compose, quickstart validation, lint+test)
- All depend on T053–T061 being complete

---

## Parallel Example: User Story 1

```text
# Launch T053 (keystone) first; once it lands:
T054: internal/oncall/client.go              (refactor resolveOnCallURL)
T055: internal/oncall/errors.go             (error code rename + alias)
T056: contracts/error_envelope.schema.json  (schema enum + description)
T057: tests/contract/jsonschema_test.go     (alias acceptance test)

# T051, T052 (the test scaffolding for T053) can be authored in parallel with
# T053 itself if the test file is stubbed first.
```

---

## Implementation Strategy

### MVP First (User Story 1 — IRM-only path, P1)

1. Confirm Phases 1–2 of 001 are `[x]` (T001–T018) — already true in this repo
2. Complete T053 (`SelectAndResolve` keystone) — this is the only new file
3. Complete T054 (refactor `resolveOnCallURL` to call `SelectAndResolve`)
4. Complete T055 (rename error code in Go), T056 (update JSON schema), T057 (alias contract test)
5. **STOP and VALIDATE**: Run unit tests + contract tests; confirm an existing IRM Grafana still works
6. The MVP is shippable as soon as T053, T054, T055, T056, T057 land

### Incremental Delivery

1. Setup + Foundational → reused from 001 (`[x]`)
2. T053–T057 (US1) → IRM-only path works → shippable
3. T058 (US2) → legacy-only path works → still no new implementation
4. T059–T061 (US3) → operator preference + both-installed scenario work
5. T062–T063 (US4) → migration-from-upstream verified by parameterized integration tests
6. T064–T069 (Polish) → documentation, agent context, CI, final lint+test
7. Each step adds value without breaking previous steps

### Parallel Team Strategy

With multiple developers:

1. Developer A: T053 (keystone), T054 (client refactor), T058 (legacy test), T061 (startUpHealthCheck rewrite) — sequential, single-owner to avoid merge conflicts in `internal/oncall/`
2. Developer B: T055 (error code rename), T056 (JSON schema), T057 (contract test), T059 (precedence test) — can run in parallel with Developer A's T053
3. Developer C: T060 (config + flag), T062 (docker-compose matrix), T063 (parameterized integration tests), T064–T067 (docs + Makefile + agent context) — can run in parallel with A and B
4. Developer D: T068 (quickstart validation), T069 (lint + test re-run) — final integration verification
5. The four developers integrate at the US1 + US2 + US3 + US4 checkpoints; no developer touches a file that another developer is actively editing

---

## Summary

| Metric | Value |
|---|---|
| **Total new tasks (002 only)** | 19 |
| **Reused from 001** | 50 (T001–T050, all `[x]`) |
| **Setup (Phase 1)** | 0 new (5 reused) |
| **Foundational (Phase 2)** | 0 new (13 reused) |
| **User Story 1 (P1)** | 7 (T051–T057) |
| **User Story 2 (P1)** | 1 (T058) |
| **User Story 3 (P2)** | 3 (T059–T061) |
| **User Story 4 (P2)** | 2 (T062–T063) |
| **Polish (Phase 7)** | 6 (T064–T069) |

### Task Count per User Story

| Story | New tasks | Independent Test Criteria |
|---|---|---|
| US1 | 7 | Server starts against an IRM-only Grafana; log shows `plugin=irm`; 11 tools respond; error envelope accepts both `ONCALL_PLUGIN_MISSING` and the deprecated `IRM_PLUGIN_MISSING` alias |
| US2 | 1 | Server starts against a legacy-only Grafana; log shows `plugin=oncall-app`; resolved URL is the legacy resource-proxy URL; 11 tools respond |
| US3 | 3 | With both plugins installed and no preference, server picks `oncall-app` and emits a `WARN` line; with `=irm`, picks `irm`; with `=oncall-app` but only IRM installed, startup fails with `INVALID_CONFIG` |
| US4 | 2 | The same MCP client prompt works against legacy-only, IRM-only, and both-installed fixtures; per-tool response shapes match across fixtures; no prompt rewrite required |

### Parallel Opportunities Identified

- **Phase 3 (US1)**: 4 of 7 tasks are [P] (T051, T052, T055, T056, T057 — only T053 and T054 are sequential)
- **Phase 4 (US2)**: 1 of 1 task is [P] (T058)
- **Phase 5 (US3)**: 1 of 3 tasks is [P] (T059 can run with T060)
- **Phase 6 (US4)**: 2 of 2 tasks are [P] (T062, T063)
- **Phase 7 (Polish)**: 6 of 6 tasks are [P] (T064–T069)

### Suggested MVP Scope

Complete T053, T054, T055, T056, T057. This delivers US1 (IRM-only path), which is the most common production case in 2026 and the largest deployment base. US2 falls out of the same code with T058 (a single new test). US3 (T059–T061) and US4 (T062–T063) are P2 enhancements that add operator-preference wiring and parameterized integration tests; the implementation work for them is small (one new env-var + flag, one docker-compose matrix growth). Polish (T064–T069) can overlap with P2 delivery.

### Format Validation

All 19 new tasks follow the checklist format:
- `- [ ]` checkbox prefix: confirmed
- Sequential Task ID (T051–T069): confirmed
- `[P]` marker on parallelizable tasks only: confirmed
- `[US1]`/`[US2]`/`[US3]`/`[US4]` story labels on user story phase tasks only: confirmed
- Exact file paths in every description: confirmed

### Reuse Summary

The 002 implementation reuses the 001 implementation as follows:

- **Reused unchanged** (50 of 50 001 tasks): all of Setup, Foundational, all 11 MCP tool handlers, error envelope struct, retry helpers, config struct, OnCall client wrapper (URL resolution body only is refactored, not the wrapper itself), contract tests, logging, OTel, transport, read-only mode, server wiring, CLI entrypoint structure, docker-compose shape, e2e tests, README scaffolding, Makefile targets, Dockerfile, .golangci.yaml, go.mod.
- **Touched, not rewritten** (7 files): `internal/oncall/client.go` (one function refactored), `internal/oncall/errors.go` (one constant renamed + alias), `internal/config/config.go` (one field + one parser added), `cmd/oncall-mcp/main.go` (one flag + one branch in `startUpHealthCheck`), `specs/002-multi-plugin-support/contracts/error_envelope.schema.json` (one enum entry + description), `tests/integration/docker-compose.yaml` (grows into a parameterized matrix), `tests/contract/jsonschema_test.go` (one new alias-acceptance test case).
- **New files** (1): `internal/oncall/plugin.go` — the dual-plugin selection helper.
- **New test files** (1): `internal/oncall/plugin_test.go` — the precedence-table tests.

### Post-002 Docker & Client-Config Artifacts (added in a follow-up, not part of the 19 numbered tasks)

- `docker-compose.yml` (root) — three-profile multi-transport compose (stdio / SSE / streamable-HTTP) with hardened defaults (UID 65534, read-only root, no-new-privileges, mem/cpu caps).
- `env.example` — operator-facing template of every supported env var.
- `opencode.json` (root) — opencode MCP config, registers two server variants (stdio enabled, HTTP disabled).
- `.vscode/mcp.json` — VS Code (Copilot Chat) MCP config, same two variants + `${input:...}` prompts.
- `.claude/claude_desktop_config.json` — Claude Desktop project-scoped config (stdio only; Claude Desktop's local config does not support `type: "http"`).
- `docs/mcp-clients.md` — per-client setup guide and security notes.
- `Makefile` additions: `compose-build`, `compose-run-stdio`, `compose-up`, `compose-down`, `compose-logs`, `compose-up-sse`, `compose-down-sse`.
- `README.md` additions: new "Running with Docker" section pointing at the compose file and the three pre-baked MCP client configs.

### Post-002 PyPI Distribution via `go-to-wheel` (added in a follow-up, not part of the 19 numbered tasks)

Mirrors the upstream `mcp-grafana` distribution model. No changes to the Go source code; only release plumbing and client config.

- `.goreleaser.yaml` (root, new) — 8-platform build matrix (linux/amd64+arm64 × glibc+musl, darwin/amd64+arm64, windows/amd64+arm64), injects `-X main.version={{.Version}}` via ldflags.
- `.github/workflows/release.yml` (new) — two-job CI: `goreleaser` (GitHub Release archives) + `pypi` (8 platform wheels via `go-to-wheel`, published with OIDC trusted publishing).
- `cmd/oncall-mcp/main.go` — `const version` → `var version` so ldflags can override it; print format changed from `v`+version to just `version` (since ldflags injects the leading `v`).
- `opencode.json` / `.vscode/mcp.json` / `.claude/claude_desktop_config.json` — `grafana-oncall` entry now uses `uvx grafana-oncall-mcp` (recommended); `grafana-oncall-docker` entry added as fallback; `grafana-oncall-http` entry remains for the long-running container.
- `docs/mcp-clients.md` — rewritten for the three-channel model (`uvx` primary, Docker fallback, HTTP shared).
- `README.md` — new "Quick Start (Recommended: `uvx`)" section at the top; new "Release Process" section for maintainers (with PyPI trusted-publishing setup steps); expanded "Troubleshooting" with `uvx`-specific entries.

Distribution check (verified locally): `uvx --from "go-to-wheel @ git+https://github.com/nikaro/go-to-wheel@f7939c6" go-to-wheel . --package-path ./cmd/oncall-mcp --name grafana-oncall-mcp --version 0.1.0.dev1 --set-version-var main.version --description "..." --url "..." --license Apache-2.0 --readme README.md` produced 8 platform wheels in `dist/` totalling ~80MB. The Linux x86_64 wheel installed cleanly into a fresh venv via `pip install`, and `grafana-oncall-mcp -version` inside the venv reported `grafana-oncall-mcp 0.1.0.dev1` (proving both the shim and the version-var injection work end-to-end).

This is the minimum surface area required to ship the dual-plugin feature while preserving the wire-format compatibility that US4 explicitly requires.
