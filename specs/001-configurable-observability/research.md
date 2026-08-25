# Research: Configurable Observability

## Decision: Delete database-backed tracing instead of gating it

**Rationale**: The feature scope is deletion of database trace storage from backend request handling. The backend must remove the old database-backed tracing request path and must not keep environment variables, middleware gates, helper checks, or compatibility branches for that legacy tracing mode. Existing historical trace rows, if any, are outside the migration scope.

**Alternatives considered**:

- Keep a disabled-by-default database tracing switch. Rejected because the requirement is deletion, not configurable retention.
- Keep the old middleware behind an internal flag. Rejected because it preserves backend DB tracing logic.
- Add extra tests for deleted DB tracing behavior. Rejected because verification should focus on the primary request flow and new non-database observability.

## Decision: Redesign request tracing around OpenTelemetry middleware

**Rationale**: Relay observability should be emitted through OpenTelemetry spans or events and synchronized structured logs. The middleware should always be registered for relay request paths and rely on OpenTelemetry default or no-op behavior when no exporter or collector is configured, so deployments without an OTel backend keep normal request behavior.

**Alternatives considered**:

- Register OTel middleware only when an exporter is configured. Rejected because request code would become configuration-dependent.
- Fail startup when no exporter exists. Rejected because observability export should not be required for normal gateway operation.

## Decision: Record a fixed relay timing baseline

**Rationale**: Operators need consistent timing evidence across traces and logs. The required baseline is request received, relay start, upstream request sent, first upstream byte, upstream complete, response complete, and error events when each point occurs.

**Alternatives considered**:

- Record only start and completion. Rejected because it hides upstream latency and streaming first-byte behavior.
- Trace every internal conversion and routing step. Rejected because it creates excessive detail for the first OTel redesign.

## Decision: Add opt-in JSON log format using existing logger stack

**Rationale**: `common/logger` already centralizes logger construction and sink configuration. JSON format should remain an operator-selected log encoding while existing console-style output stays available.

**Alternatives considered**:

- Replace the logging stack. Rejected as too broad.
- Emit a parallel JSON log stream. Rejected because it complicates sinks and doubles operational noise.

## Decision: Use OpenTelemetry SpanContext for `trace_id` and `span_id`

**Rationale**: Correlation fields must use valid OpenTelemetry identifiers, not project-defined request IDs. The implementation should use the active OpenTelemetry span context and attach IDs to request-scoped JSON logs only when that context is valid.

**Alternatives considered**:

- Reuse existing project trace IDs. Rejected because the requirement explicitly calls for OpenTelemetry-compliant IDs.
- Parse `traceparent` headers directly. Rejected because OpenTelemetry propagation and middleware should own context extraction.

## Decision: Omit correlation IDs when no valid OTel context exists

**Rationale**: OpenTelemetry trace IDs are 32 lowercase hexadecimal characters and span IDs are 16 lowercase hexadecimal characters, with all-zero values invalid. If no valid context exists, fabricated values would violate the requirement.

**Alternatives considered**:

- Generate new IDs for every log entry. Rejected because local values may not correspond to the real distributed trace.
- Copy request IDs into trace fields. Rejected because that would be a project-specific substitute.

## Decision: Leave trace routes and frontend unchanged

**Rationale**: Frontend and public interface adaptation is outside scope. Existing trace views or API routes may surface existing errors after DB-backed tracing is removed, and the feature should not add new response contracts for those surfaces.

**Alternatives considered**:

- Hide trace UI or routes. Rejected because it adds UI/API compatibility work outside the requested scope.
- Return a new removed-tracing response shape. Rejected for the same reason.

## Decision: Align GenAI trace data with OpenTelemetry GenAI inference client spans

**Rationale**: OpenTelemetry GenAI inference client spans define the canonical span kind, span name, and model request attributes for client-side GenAI calls. OneAPI should map relay facts already known from normal routing, upstream response, and usage accounting to those exact field names, including `gen_ai.operation.name`, `gen_ai.provider.name`, `gen_ai.request.model`, `gen_ai.response.model`, `gen_ai.response.time_to_first_chunk`, `gen_ai.response.finish_reasons`, `gen_ai.response.id`, `error.type`, and usage-token fields such as `gen_ai.usage.input_tokens`, `gen_ai.usage.output_tokens`, `gen_ai.usage.cache_read.input_tokens`, and `gen_ai.usage.cache_write.input_tokens`. Tracing must not parse or re-serialize the client request body only to populate optional request-parameter or content attributes.

**Alternatives considered**:

- Use a loose project-specific GenAI metadata object. Rejected because it does not fully align with the span convention.
- Invent values for unknown recommended fields. Rejected because semantic convention attributes must be omitted when OneAPI cannot determine them cheaply from existing flow state.
- Parse the client request body for optional request parameters. Rejected because tracing should not add serialization or parsing overhead.
- Emit input-detail or content attributes by default. Rejected because `gen_ai.input.messages`, `gen_ai.output.messages`, prompt variables, system instructions, tool definitions, and similar request-detail fields may contain sensitive content and require request parsing.

## Decision: Log sanitized prompt previews with deterministic 100/100 truncation

**Rationale**: Prompt previews provide useful debugging context while bounding log size and reducing exposure risk. The preview keeps the first 100 characters and last 100 characters, separated by `...`, only when the sanitized prompt is longer than 200 characters.

**Alternatives considered**:

- Log full prompts. Rejected because it creates size and privacy risk.
- Log only metadata. Rejected because prompt preview logging is explicitly required.
- Use byte truncation. Rejected because character-based truncation better preserves readable text and avoids splitting multibyte characters.
