# Feature Specification: Configurable Observability

**Feature Branch**: `001-configurable-observability`

**Created**: 2026-08-25

**Status**: Draft

**Input**: User description: "The built-in tracing is currently implemented through the database and must be removed completely rather than disabled by configuration. Backend implementation must not retain database-backed tracing logic, and deletion does not require extra unit tests beyond verifying the primary request flow. Add OpenTelemetry-based tracing middleware. During relay processing, record spans or events at key timing points and emit synchronized structured logs. Logs must include an additional prompt preview containing the prompt text truncated to the first 100 characters, an ellipsis, and the last 100 characters."

## Clarifications

### Session 2026-08-25

- Q: How complete should GenAI observability be across OpenTelemetry tracing and logs? → A: Emit all known applicable OpenTelemetry GenAI semantic fields in both OTel tracing and structured logs, including required provider/operation fields, request/response model fields, stream mode, upstream server fields, request parameters, response identifiers, finish reasons, usage tokens, and OneAPI routing fields.
- Q: Should the revised feature remove all DB-backed tracing code paths instead of keeping any configuration control for them? → A: Delete all DB-backed tracing logic; no DB tracing configuration remains in scope.
- Q: Which relay timing points must the OpenTelemetry middleware and synchronized logs record? → A: Record request received, relay start, upstream request sent, first upstream byte, upstream complete, response complete, and error events.
- Q: How should OpenTelemetry tracing behave when no exporter or collector is configured? → A: Always register OTel middleware; use no-op/default behavior when no exporter is configured.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Remove Database Trace Storage (Priority: P1)

As a production operator running one-api, I need database-backed trace persistence removed completely without changing user-facing API behavior or disabling non-database observability.

**Why this priority**: Production deployments must stop database trace writes permanently to reduce storage pressure, avoid high-cardinality trace persistence, and rely on external observability backends without retaining legacy backend tracing paths.

**Independent Test**: Can be fully tested by sending normal traffic and confirming requests continue while no new database-backed trace records are created.

**Acceptance Scenarios**:

1. **Given** database-backed tracing has been removed, **When** a user sends a supported API request, **Then** the request is processed without creating new built-in trace records.
2. **Given** database-backed tracing has been removed, **When** a frontend or management interface attempts to read built-in trace data, **Then** the service may return an existing error response without requiring new frontend or interface compatibility behavior.
3. **Given** normal relay traffic is processed, **When** observability is emitted, **Then** the system uses OpenTelemetry tracing and structured logs rather than database-backed trace storage.

---

### User Story 2 - Emit JSON Logs With Correlation Fields (Priority: P2)

As an operator ingesting one-api logs into a centralized observability platform, I need built-in logs to be configurable as JSON and to include consistent severity and request correlation fields.

**Why this priority**: Structured logs reduce parsing work and allow operators to correlate log events with distributed traces across systems.

**Independent Test**: Can be fully tested by enabling JSON log output, sending a traced request, and confirming each applicable log entry is valid JSON with the required fields.

**Acceptance Scenarios**:

1. **Given** JSON log output is enabled, **When** the service emits an application log entry, **Then** the entry is valid JSON and includes `severity_text`.
2. **Given** a request has trace context, **When** the service emits a request-scoped JSON log entry, **Then** the entry includes `trace_id` and `span_id` values that match OpenTelemetry identifier format rules.
3. **Given** JSON log output is not enabled, **When** the service emits logs, **Then** existing non-JSON log behavior remains available.

---

### User Story 3 - Capture GenAI Request Context in Traces and Logs (Priority: P2)

As an operator investigating model traffic, I need traces and logs to include GenAI request context and OneAPI routing details so I can understand which channel, stream mode, and upstream destination were used for a request.

**Why this priority**: Correlating model requests with routing decisions and upstream destinations is essential for diagnosing provider failures, latency, and billing disputes.

**Independent Test**: Can be fully tested by sending streaming and non-streaming model requests through a known channel and confirming the trace and JSON logs contain exact OpenTelemetry GenAI inference fields and OneAPI fields.

**Acceptance Scenarios**:

1. **Given** a model request is routed to an upstream provider, **When** OpenTelemetry middleware handles the request, **Then** the trace captures model request context using exact OpenTelemetry GenAI inference span fields for every known value in scope.
2. **Given** a request is routed through a channel, **When** trace and JSON log records are emitted, **Then** they include `oneapi.channel_id`, `oneapi.stream`, `oneapi.upstream_address`, and `oneapi.upstream_url` when those values are known.
3. **Given** the client sends a request body containing prompt content, **When** JSON logs are emitted, **Then** logs include only a truncated prompt preview using the first 100 characters, `...`, and the last 100 characters.
4. **Given** a model request contains OpenTelemetry GenAI inference attributes that OneAPI can determine, **When** OpenTelemetry tracing and structured logs are emitted, **Then** both observability surfaces include the same known GenAI fields with exact standard field names.
5. **Given** relay processing occurs, **When** OpenTelemetry tracing and structured request logs are emitted, **Then** both surfaces record request received, relay start, upstream request sent, first upstream byte, upstream complete, response complete, and error events when each point occurs.
6. **Given** no OpenTelemetry exporter or collector is configured, **When** relay traffic is processed, **Then** OpenTelemetry middleware remains registered and uses no-op/default behavior without changing request outcomes.

---

### User Story 4 - Preserve Existing Operational Surfaces (Priority: P3)

As a maintainer, I need this observability change to avoid expanding frontend or public interface scope so the release stays focused on deployment-time configuration and log interoperability.

**Why this priority**: The requested change explicitly allows existing trace UI or interface errors after database-backed tracing is removed, preventing scope growth that would delay the operational value.

**Independent Test**: Can be fully tested by exercising existing frontend and interface trace views after database-backed tracing is removed and confirming no new compatibility promise is introduced.

**Acceptance Scenarios**:

1. **Given** database-backed tracing has been removed, **When** existing trace-related frontend or interface actions are used, **Then** the system may surface an error without requiring new fallback screens or response formats.
2. **Given** the observability configuration is changed, **When** unrelated API format conversion requests are processed, **Then** ChatCompletion, Response, and Claude Messages compatibility behavior is unchanged.

### Edge Cases

- Historical trace records may already exist; existing records are not required to be deleted or hidden by this feature.
- A request has no trace context; JSON logs must still be valid and must not invent non-standard trace or span identifiers.
- A request includes malformed trace context; logs must not emit invalid OpenTelemetry identifier values.
- JSON logging is enabled after database-backed tracing removal; logs must still support request correlation when valid trace context is available.
- Required log fields must never include secrets, tokens, credentials, or full sensitive request payloads.
- A request body is 200 characters or shorter; the prompt preview must not add an ellipsis unless truncation occurs.
- A request body contains credentials or token-like fields; prompt preview logging must respect existing redaction and must not expose those sensitive values.
- A GenAI semantic field would require extra parsing or serialization of the client request body only for observability; tracing must omit that field rather than adding request processing overhead.
- Upstream routing fails before a channel or upstream URL is selected; traces and logs must remain valid and include only fields that are known.
- No OpenTelemetry exporter or collector is configured; middleware must not fail startup or request processing solely because export is unavailable.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST remove database-backed tracing logic from backend request handling instead of retaining configuration controls for it.
- **FR-002**: System MUST stop creating new built-in trace records for all deployments.
- **FR-003**: System MUST avoid adding compatibility logic, fallback logic, or tests specifically for deleted database-backed tracing paths; verification focuses on the primary request flow and non-database observability.
- **FR-004**: System MUST preserve existing request handling behavior for ChatCompletion, Response, and Claude Messages API formats after database-backed tracing is removed.
- **FR-004a**: System MUST replace the original database-backed tracing middleware with OpenTelemetry tracing middleware and request log-correlation behavior.
- **FR-004b**: System MUST NOT retain database-backed tracing controls in backend request handling.
- **FR-005**: System MUST NOT require frontend or public interface compatibility changes for removed built-in tracing; existing error behavior for trace-related views or calls is acceptable.
- **FR-006**: System MUST provide a deployment-time configuration option that selects JSON log output.
- **FR-007**: System MUST keep the existing non-JSON log format available when JSON log output is not selected.
- **FR-008**: System MUST include `severity_text` in JSON log entries.
- **FR-009**: System MUST include `trace_id` and `span_id` in request-scoped JSON log entries when a valid trace and span context is available.
- **FR-010**: System MUST ensure `trace_id` and `span_id` values in logs comply with OpenTelemetry identifier format requirements.
- **FR-011**: System MUST avoid emitting invented, placeholder, malformed, or project-specific trace and span identifiers when valid OpenTelemetry context is unavailable.
- **FR-012**: System MUST ensure observability logs do not expose API keys, passwords, tokens, credentials, or sensitive request payloads.
- **FR-013**: System MUST align relay model-request spans with the OpenTelemetry GenAI inference client span convention.
- **FR-013a**: System MUST create GenAI inference spans with span kind `CLIENT` and span name `{gen_ai.operation.name} {gen_ai.request.model}` when the request model is already known from the normal relay flow.
- **FR-013b**: System MUST emit required GenAI inference span attributes `gen_ai.operation.name` and `gen_ai.provider.name` when known.
- **FR-013c**: System MUST emit conditional GenAI inference span attributes when applicable and already known without additional client request parsing: `error.type`, `gen_ai.output.type`, `gen_ai.request.model`, `gen_ai.request.stream`, and `server.port` when `server.address` is set.
- **FR-013d**: System MUST emit recommended GenAI inference span attributes when already known without additional client request parsing: `gen_ai.response.finish_reasons`, `gen_ai.response.id`, `gen_ai.response.model`, `gen_ai.response.time_to_first_chunk`, `gen_ai.usage.audio.cache_read.input_tokens`, `gen_ai.usage.audio.input_tokens`, `gen_ai.usage.audio.output_tokens`, `gen_ai.usage.cache_read.input_tokens`, `gen_ai.usage.cache_write.input_tokens`, `gen_ai.usage.image.cache_read.input_tokens`, `gen_ai.usage.image.input_tokens`, `gen_ai.usage.image.output_tokens`, `gen_ai.usage.input_tokens`, `gen_ai.usage.output_tokens`, `gen_ai.usage.reasoning.output_tokens`, `gen_ai.usage.text.cache_read.input_tokens`, `gen_ai.usage.text.input_tokens`, `gen_ai.usage.text.output_tokens`, and `server.address`.
- **FR-013e**: System MUST NOT emit input-detail or content attributes in traces, including `gen_ai.conversation.id`, `gen_ai.conversation.compacted`, `gen_ai.input.messages`, `gen_ai.output.messages`, `gen_ai.prompt.name`, `gen_ai.prompt.version`, `gen_ai.prompt.variable.*`, `gen_ai.request.choice.count`, `gen_ai.request.frequency_penalty`, `gen_ai.request.max_tokens`, `gen_ai.request.presence_penalty`, `gen_ai.request.previous_response.id`, `gen_ai.request.reasoning.level`, `gen_ai.request.seed`, `gen_ai.request.stop_sequences`, `gen_ai.request.temperature`, `gen_ai.request.top_k`, `gen_ai.request.top_p`, `gen_ai.system_instructions`, and `gen_ai.tool.definitions`, unless a future feature explicitly provides these values without extra client request parsing and with sanitization controls.
- **FR-013f**: System MUST include the same known GenAI semantic fields in structured request logs using the same standard field names used for OpenTelemetry tracing.
- **FR-013g**: System MUST NOT emit invented GenAI values when OneAPI cannot determine a field; unknown optional attributes are omitted.
- **FR-013h**: System MUST NOT parse, re-serialize, or inspect the client request body solely to populate tracing fields.
- **FR-014**: System MUST include OneAPI routing fields in both trace data and structured request logs when available: `oneapi.channel_id`, `oneapi.stream`, `oneapi.upstream_address`, and `oneapi.upstream_url`.
- **FR-015**: System MUST include the same OneAPI routing fields in request-scoped JSON log entries when available.
- **FR-016**: System MUST include an additional sanitized prompt preview in request-scoped JSON logs when prompt text is present.
- **FR-017**: System MUST truncate logged client request body or prompt previews longer than 200 characters to the first 100 characters, followed by `...`, followed by the last 100 characters.
- **FR-018**: System MUST avoid logging untruncated large request bodies and must apply existing sensitive-data redaction before or during prompt preview generation.
- **FR-019**: System MUST record synchronized OpenTelemetry trace events and structured request log entries for request received, relay start, upstream request sent, first upstream byte, upstream complete, response complete, and error events when each point occurs.
- **FR-020**: System MUST always register OpenTelemetry tracing middleware for relay request paths and rely on OpenTelemetry no-op/default behavior when no exporter or collector is configured.

### Key Entities

- **Observability Configuration**: Deployment settings that determine which log output format is used and how non-database observability is emitted.
- **Built-In Trace Record**: A legacy service-generated record formerly used by the database-backed tracing capability; new records are no longer created by this feature.
- **Structured Log Entry**: A log event formatted for machine parsing with severity and, when available, distributed trace correlation identifiers.
- **Trace Context**: Request correlation information that may contain standard trace and span identifiers.
- **GenAI Request Context**: Model request metadata and events aligned with OpenTelemetry GenAI semantic convention concepts.
- **Prompt Preview**: Sanitized and size-bounded representation of the client request body for operational debugging.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In all deployments, 100% of normal user API requests complete without creating new built-in trace records.
- **SC-002**: Existing primary request-flow tests continue to pass after database-backed tracing removal.
- **SC-003**: When JSON logging is selected, 100% of sampled application log lines are valid JSON and include `severity_text`.
- **SC-004**: For sampled request logs with valid trace context, 100% include OpenTelemetry-compliant `trace_id` and `span_id` values.
- **SC-005**: Existing frontend or interface trace-related errors caused by removed database-backed tracing require zero new compatibility work for this feature release.
- **SC-006**: No sampled observability log entry contains secrets, credentials, or sensitive request payload content.
- **SC-007**: For sampled routed model requests where routing succeeds, 100% of OpenTelemetry traces and request-scoped JSON logs include `oneapi.channel_id`, `oneapi.stream`, `oneapi.upstream_address`, and `oneapi.upstream_url` fields when known.
- **SC-008**: For sampled client request bodies longer than 200 characters, 100% of prompt previews are exactly first 100 characters plus `...` plus last 100 characters after sanitization.
- **SC-009**: For sampled routed model requests with known GenAI data, OpenTelemetry spans or events and structured logs expose matching GenAI semantic field names and values for all known required, conditional, and recommended fields in scope.
- **SC-010**: For sampled relay requests, 100% of occurred required timing points have matching OpenTelemetry trace events and structured request log entries.
- **SC-011**: With no OpenTelemetry exporter or collector configured, 100% of sampled primary request-flow tests continue to pass with OpenTelemetry middleware registered.

## Assumptions

- Database-backed tracing is deleted rather than controlled by configuration.
- JSON logging is opt-in so deployments that rely on the current log format can continue unchanged.
- Removing built-in database-backed tracing removes backend database trace logic; migration, deletion, or hiding of historical records is outside the feature scope.
- OpenTelemetry-compliant identifiers are required for emitted correlation fields, while the choice of propagation source is decided during planning.
- OpenTelemetry middleware can be registered without an exporter because default or no-op provider behavior is acceptable when export is unavailable.
- Frontend and public interface adaptations for removed database-backed tracing are intentionally outside this feature scope.
- OpenTelemetry GenAI semantic conventions are used as an alignment target where they fit OneAPI request data, while OneAPI-specific routing details remain clearly named internal attributes.
- Prompt previews are intended for operational debugging and are not a replacement for full request archival.
