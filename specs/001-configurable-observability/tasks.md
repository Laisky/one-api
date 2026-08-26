---

description: "Task list for configurable observability implementation"
---

# Tasks: Configurable Observability

**Input**: Design documents from `specs/001-configurable-observability/`

**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md), [data-model.md](./data-model.md), [contracts/observability-config.md](./contracts/observability-config.md), [quickstart.md](./quickstart.md)

**Tests**: Tests are included because the specification and constitution require proof for observability behavior. DB-backed tracing deletion does not need extra dedicated unit tests beyond primary request-flow verification.

**Organization**: Tasks are grouped by user story so each story can be implemented and tested independently.

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: Can run in parallel because it touches different files and does not depend on incomplete tasks.
- **[Story]**: Which user story this task belongs to.
- Every task includes exact repository-relative file paths.

## Phase 1: Setup (Shared Understanding)

**Purpose**: Locate the current tracing, logging, and relay touchpoints before implementation.

- [X] T001 Inspect current DB-backed tracing, OpenTelemetry, logger, and relay touchpoints in `main.go`, `middleware/tracing.go`, `model/trace.go`, `common/tracing/tracing.go`, `common/logger/logger.go`, and `relay/`
- [X] T002 [P] Verify current OpenTelemetry dependencies and span context APIs in `go.mod` and `common/tracing/tracing.go`
- [X] T003 [P] Identify all DB-backed tracing configuration, middleware registration, request-path create/update calls, and documentation references in `common/config/config.go`, `main.go`, `middleware/tracing.go`, `model/trace.go`, `README.md`, and `docs/arch/tracing_system.md`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Add shared non-database observability helpers required by every story.

**CRITICAL**: No user story work can begin until this phase is complete.

- [X] T004 Keep only non-DB observability configuration such as `RELAY_ACCESS_LOG_ENABLED` in `common/config/config.go`
- [X] T005 Keep validation free of deleted DB tracing and global log-format controls in `common/config/validation.go`
- [X] T006 [P] Add config validation tests for absence of DB tracing config in `common/config/validation_test.go`
- [X] T007 Add OpenTelemetry trace ID and span ID extraction helpers that return only valid standard IDs in `common/tracing/tracing.go`
- [X] T008 [P] Add OpenTelemetry trace ID and span ID extraction tests for valid, missing, invalid, and all-zero contexts in `common/tracing/tracing_test.go`
- [X] T009 Add prompt preview sanitization and first-100/last-100 character truncation helper in `common/logging_sanitize.go`
- [X] T010 [P] Add prompt preview tests for short, exact-boundary, long, multibyte, and sensitive-field cases in `common/logging_sanitize_test.go`
- [X] T011 Define relay timing event names for request received, relay start, upstream request sent, first upstream byte, upstream complete, response complete, and errors in `common/tracing/tracing.go`
- [X] T012 Define allowed OpenTelemetry GenAI inference span attributes, excluded input-detail attributes, and OneAPI namespaced attributes in `common/tracing/tracing.go`

**Checkpoint**: Shared helpers and constants are ready; user story implementation can begin.

---

## Phase 3: User Story 1 - Remove Database Trace Storage (Priority: P1) MVP

**Goal**: Backend request handling no longer creates or updates DB-backed trace records, while supported API requests continue to work.

**Independent Test**: Send normal supported API requests and verify existing primary request-flow checks pass without DB trace request-path persistence.

### Tests for User Story 1

- [X] T013 [P] [US1] Add or update middleware primary request-flow test proving requests succeed without DB trace middleware in `middleware/tracing_duplicate_traceid_test.go`
- [ ] T014 [P] [US1] Add or update relay primary request-flow test proving relay succeeds without DB trace writes in `relay/controller/relay_test.go`

### Implementation for User Story 1

- [X] T015 [US1] Remove DB-backed tracing middleware registration from `main.go`
- [X] T016 [US1] Remove DB-backed request tracing middleware logic from `middleware/tracing.go`
- [X] T017 [US1] Remove request-path DB trace create and update calls from `model/trace.go` and relay callers while preserving unrelated historical trace code only if still used outside request handling
- [X] T018 [US1] Remove DB tracing configuration names and validation from `common/config/config.go` and `common/config/validation.go`
- [X] T019 [US1] Update architecture documentation to describe DB trace removal and OTel replacement in `docs/arch/tracing_system.md`

**Checkpoint**: User Story 1 is independently functional and is the MVP.

---

## Phase 4: User Story 2 - Emit Opt-In Relay Access Logs (Priority: P2)

**Goal**: Operators can opt into one concise access log entry per relay request without changing global logger output.

**Independent Test**: Start with `RELAY_ACCESS_LOG_ENABLED=true`, send a relay request, and verify exactly one `relay access` summary log is emitted with request context and no internal timing chatter.

### Tests for User Story 2

- [ ] T020 [P] [US2] Add relay access-log integration coverage only if automated integration hooks become available
- [ ] T021 [P] [US2] Verify manually that disabled `RELAY_ACCESS_LOG_ENABLED` emits no relay access logs

### Implementation for User Story 2

- [X] T022 [US2] Add `RELAY_ACCESS_LOG_ENABLED` configuration in `common/config/config.go`
- [X] T023 [US2] Restore global logger behavior so access logging does not change general log encoding in `common/logger/logger.go`
- [X] T024 [US2] Add opt-in relay access log middleware in `middleware/relay_access_log.go`
- [X] T025 [US2] Register relay access log middleware on relay routes only in `router/relay.go`
- [ ] T026 [US2] Update operational logging documentation for relay access logging in `docs/arch/logger.md`

**Checkpoint**: User Story 2 works independently from removed DB-backed tracing and does not increase global log volume.

---

## Phase 5: User Story 3 - Capture GenAI Request Context and Relay Timing (Priority: P2)

**Goal**: Routed model requests expose relay timing events, exact OpenTelemetry GenAI inference span fields from already-known values, and OneAPI routing fields in OpenTelemetry traces and structured logs.

**Independent Test**: Send streaming and non-streaming model requests through a known channel and verify traces/logs include required timing events, allowed GenAI fields, OneAPI routing fields, and prompt preview logs without tracing-driven client request parsing.

### Tests for User Story 3

- [X] T027 [P] [US3] Add OTel relay middleware test proving no exporter configuration does not fail request handling in `middleware/tracing_test.go`
- [X] T028 [P] [US3] Add tracing helper tests for span kind `CLIENT`, span name `{gen_ai.operation.name} {gen_ai.request.model}`, allowed already-known GenAI attributes, excluded input-detail attributes, and OneAPI attributes in `common/tracing/tracing_test.go`
- [ ] T029 [P] [US3] Add relay tracing verification for timing events, OneAPI routing fields, allowed GenAI fields, and prompt preview log-only behavior in `relay/controller/debug_logging_test.go`

### Implementation for User Story 3

- [X] T030 [US3] Add always-registered OpenTelemetry relay middleware with no-op/default behavior when no exporter exists in `middleware/tracing.go`
- [ ] T031 [US3] Emit trace events for required relay timing points without synchronized internal debug log lines in `relay/controller/text.go`
- [ ] T032 [US3] Capture `oneapi.channel_id`, `oneapi.stream`, `oneapi.upstream_address`, and `oneapi.upstream_url` before upstream dispatch in `relay/adaptor/common.go`
- [ ] T033 [US3] Add exact OpenTelemetry GenAI inference span fields from already-known relay, routing, upstream response, and usage accounting values in `relay/controller/text.go`
- [X] T034 [US3] Suppress input-detail and content attributes without parsing, re-serializing, or inspecting client request bodies solely for tracing in `common/tracing/tracing.go`
- [ ] T035 [US3] Add `prompt_preview` to request-scoped structured logs using the shared truncation helper in `common/render/stream_log.go` and `relay/controller/debug_logging.go`
- [ ] T036 [US3] Ensure touched relay functions retrieve the request-scoped logger with `gmw.GetLogger(c)` once per function in `relay/controller/text.go`, `relay/adaptor/common.go`, and `relay/controller/debug_logging.go`
- [X] T037 [US3] Update trace/log field contract with final implemented GenAI, OneAPI, and relay access log field names in `specs/001-configurable-observability/contracts/observability-config.md`

**Checkpoint**: User Story 3 model observability can be validated without frontend changes.

---

## Phase 6: User Story 4 - Preserve Existing Operational Surfaces (Priority: P3)

**Goal**: Existing trace frontend/API surfaces remain out of scope and may surface existing errors after DB-backed tracing is removed.

**Independent Test**: Exercise trace-related API/frontend paths and verify no new compatibility behavior is required.

### Tests for User Story 4

- [ ] T038 [P] [US4] Add or update trace API test showing removed DB-backed tracing does not require a new response contract in `controller/uuid_contract_test.go`

### Implementation for User Story 4

- [ ] T039 [US4] Confirm trace routes do not gain new removed-tracing response branches in `router/api.go`
- [ ] T040 [US4] Confirm trace controllers keep existing not-found/error behavior without frontend compatibility changes in `controller/tracing.go`

**Checkpoint**: Existing operational surfaces are preserved.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Documentation, validation, and final quality gates across all stories.

- [ ] T041 [P] Update quickstart validation steps with final runnable commands and observed checks in `specs/001-configurable-observability/quickstart.md`
- [ ] T042 [P] Update README observability section with DB trace removal, OTel relay tracing, GenAI field policy, and relay access log configuration in `README.md`
- [ ] T043 Run focused package tests from quickstart in `specs/001-configurable-observability/quickstart.md`
- [ ] T044 Run `go vet ./...` from `/Users/leo/Documents/codes/one-api`
- [ ] T045 Run `go test -race ./...` from `/Users/leo/Documents/codes/one-api`
- [ ] T046 Run `make build-frontend-modern` from `/Users/leo/Documents/codes/one-api`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies.
- **Foundational (Phase 2)**: Depends on Setup completion and blocks all user stories.
- **User Story 1 (Phase 3)**: Depends on Foundational; this is the MVP.
- **User Story 2 (Phase 4)**: Depends on Foundational; can run independently of US1 after shared helpers exist.
- **User Story 3 (Phase 5)**: Depends on Foundational and benefits from US2 logger correlation but can begin once OTel helpers exist.
- **User Story 4 (Phase 6)**: Depends on US1 because it validates surfaces after DB trace removal.
- **Polish (Phase 7)**: Depends on all desired user stories.

### User Story Dependencies

- **US1**: Can start after Phase 2 and has no dependency on other user stories.
- **US2**: Can start after Phase 2 and has no dependency on other user stories.
- **US3**: Can start after Phase 2; final log validation is clearer after US2.
- **US4**: Depends on US1 DB trace removal.

### Parallel Opportunities

- T002 and T003 can run in parallel after T001.
- T006, T008, T010, T011, and T012 can run in parallel once related helpers are drafted.
- US1 tests T013 and T014 can run in parallel.
- US2 tests T020 and T021 can run in parallel.
- US3 tests T027, T028, and T029 can run in parallel.
- Documentation tasks T041 and T042 can run in parallel after implementation stabilizes.

---

## Parallel Example: User Story 1

```text
Task: "Add or update middleware primary request-flow test proving requests succeed without DB trace middleware in middleware/tracing_duplicate_traceid_test.go"
Task: "Add or update relay primary request-flow test proving relay succeeds without DB trace writes in relay/controller/relay_test.go"
```

## Parallel Example: User Story 2

```text
Task: "Verify relay access logs are emitted only when RELAY_ACCESS_LOG_ENABLED=true"
Task: "Verify disabled relay access logs do not add request log volume"
```

## Parallel Example: User Story 3

```text
Task: "Add OTel relay middleware test proving no exporter configuration does not fail request handling in middleware/tracing_test.go"
Task: "Add tracing helper tests for span kind CLIENT, span name, allowed already-known GenAI attributes, excluded input-detail attributes, and OneAPI attributes in common/tracing/tracing_test.go"
Task: "Add relay request logging tests for timing events, OneAPI routing fields, allowed GenAI fields, and prompt preview presence in relay/controller/debug_logging_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1 setup.
2. Complete Phase 2 shared non-DB observability helpers.
3. Complete Phase 3 to remove database-backed request tracing.
4. Stop and validate US1 independently before adding relay access logging and GenAI timing work.

### Incremental Delivery

1. Deliver US1 so backend request handling no longer writes DB trace records.
2. Deliver US2 so operators can enable concise relay access logs without changing global logger behavior.
3. Deliver US3 so OTel traces/logs include relay timing, exact OpenTelemetry GenAI inference fields from already-known values, OneAPI routing context, and bounded prompt previews.
4. Deliver US4 to verify existing trace routes/controllers remain outside new compatibility scope.

### Parallel Team Strategy

After Phase 2, separate agents can work on US1, US2, and US3 with limited file overlap if they coordinate shared files `main.go`, `common/tracing/tracing.go`, `middleware/tracing.go`, and relay controller/adaptor files.

---

## Notes

- Do not retain database-backed tracing controls, middleware gates, helper checks, request-path writes, or compatibility branches.
- Do not add frontend compatibility behavior for removed database tracing.
- Do not use project-specific IDs for JSON `trace_id` or `span_id`.
- Do not parse, re-serialize, or inspect client request bodies solely to populate tracing fields.
- Keep prompt preview logging sanitized and bounded before emission.
- Use existing dependencies before considering any new library.
