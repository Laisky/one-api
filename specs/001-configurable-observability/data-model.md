# Data Model: Configurable Observability

## Observability Configuration

**Purpose**: Represents deployment-time operator choices for operational log formatting and non-database observability output.

**Fields**:

- `log_format`: String setting that selects operational log output format.
- `otel_exporter`: Optional OpenTelemetry exporter configuration supplied by the existing deployment environment.
- `prompt_preview_limit`: Fixed preview behavior of first 100 characters and last 100 characters for long sanitized prompt text.

**Validation Rules**:

- `log_format` must accept the default console behavior and the JSON behavior.
- Invalid `log_format` values must fail configuration validation instead of silently choosing an unexpected format.
- Missing OpenTelemetry exporter configuration must not fail startup or request processing.

**Relationships**:

- Controls how Structured Log Entries are encoded.
- Does not control whether OpenTelemetry middleware is registered.
- Does not control any database-backed trace storage because that storage path is removed.

## Legacy Built-In Trace Record

**Purpose**: Historical database-backed trace rows may exist from earlier versions, but this feature removes backend request-path creation and update logic for them.

**Fields**:

- Existing historical fields remain out of scope for redesign.

**Validation Rules**:

- New database-backed trace records must not be created by request handling.
- Request handling must not update database-backed trace records.
- Migration, deletion, hiding, or UI redesign for historical records is outside this feature scope.

**State Transitions**:

1. Historical records may already exist before deployment.
2. No new request-path state transition creates or updates database-backed trace records.
3. Any existing cleanup behavior not tied to request tracing may remain outside this feature scope.

## OpenTelemetry Relay Trace

**Purpose**: Request-scoped OpenTelemetry trace data emitted for relay observability without database persistence.

**Fields**:

- `trace_id`: OpenTelemetry trace ID when a valid span context exists.
- `span_id`: OpenTelemetry span ID when a valid span context exists.
- `span_kind`: OpenTelemetry span kind `CLIENT`.
- `span_name`: `{gen_ai.operation.name} {gen_ai.request.model}` when request model is already known from the normal relay flow.
- `events`: Relay timing events for request received, relay start, upstream request sent, first upstream byte, upstream complete, response complete, and errors when each point occurs.
- `attributes`: OpenTelemetry GenAI inference client span attributes listed in GenAI Request Context when known without extra client request parsing.
- `oneapi.channel_id`: OneAPI channel selected for the upstream request when known.
- `oneapi.stream`: Whether the request used streaming behavior when known.
- `oneapi.upstream_address`: Host or address used for the upstream request when known.
- `oneapi.upstream_url`: Concrete upstream request URL when known, sanitized before emission.

**Validation Rules**:

- Middleware must be registered even when no exporter or collector is configured.
- Missing exporter configuration must result in OpenTelemetry no-op/default behavior, not failed request handling.
- GenAI inference fields must use the exact OpenTelemetry semantic convention names listed in GenAI Request Context.
- GenAI inference fields must be populated only when the corresponding model request data is already known from normal relay, routing, upstream response, or usage accounting flow.
- Tracing must not parse, re-serialize, or inspect the client request body solely to populate GenAI fields.
- OneAPI internal routing fields must remain clearly distinguishable from standard OpenTelemetry fields.
- Sensitive prompt contents must not be stored in trace attributes unless represented by bounded and sanitized preview rules explicitly allowed by logging requirements.

**Relationships**:

- Supplies trace context and event names used by Structured Log Entries.
- Replaces the old database-backed tracing request path.

## Structured Log Entry

**Purpose**: Operational log event emitted to stdout/file sinks for operator ingestion and debugging.

**Fields**:

- `severity_text`: Text severity label required for JSON log output.
- `trace_id`: OpenTelemetry trace ID when a valid span context exists.
- `span_id`: OpenTelemetry span ID when a valid span context exists.
- `event`: Relay timing event name when the log corresponds to a relay timing point.
- `oneapi.channel_id`: OneAPI channel selected for the upstream request when known.
- `oneapi.stream`: Whether the request used streaming behavior when known.
- `oneapi.upstream_address`: Host or address used for the upstream request when known.
- `oneapi.upstream_url`: Concrete upstream request URL when known, sanitized before logging.
- `prompt_preview`: Sanitized preview of prompt text when prompt text is present.
- OpenTelemetry GenAI fields listed in GenAI Request Context when available without extra client request parsing, using the same field names as trace attributes.
- Existing fields such as message, timestamp, logger name, host, request ID, status, latency, model, channel, and error details as applicable.

**Validation Rules**:

- JSON log entries must be valid JSON when JSON format is selected.
- `severity_text` must appear on JSON log entries.
- `trace_id`, when emitted, must be exactly 32 hexadecimal characters and must not be the all-zero ID.
- `span_id`, when emitted, must be exactly 16 hexadecimal characters and must not be the all-zero ID.
- Logs must not expose API keys, passwords, tokens, credentials, or sensitive request payloads.
- `prompt_preview` must be omitted or empty when prompt text cannot be safely read or sanitized.
- `prompt_preview` must preserve the full sanitized value when it is 200 characters or shorter.
- `prompt_preview` must use first 100 characters, then `...`, then last 100 characters when the sanitized value is longer than 200 characters.

**Relationships**:

- May be correlated to OpenTelemetry traces through `trace_id` and `span_id`.
- Must not use project-specific IDs as substitutes for OpenTelemetry fields.

## Trace Context

**Purpose**: Request-scoped correlation context supplied by OpenTelemetry instrumentation and propagation.

**Fields**:

- `otel_trace_id`: Standard OpenTelemetry trace identifier.
- `otel_span_id`: Standard OpenTelemetry span identifier.
- `is_valid`: Whether both identifiers are valid according to OpenTelemetry rules.

**Validation Rules**:

- Only valid OpenTelemetry span context may populate JSON log `trace_id` and `span_id`.
- Malformed, missing, or all-zero context must not be converted into project-specific replacement IDs.

**Relationships**:

- Source of `trace_id` and `span_id` for request-scoped JSON logs.
- Independent from database-backed tracing because that backend path is removed.

## GenAI Request Context

**Purpose**: OpenTelemetry GenAI inference client span metadata used to make OneAPI traces understandable in OpenTelemetry-compatible observability tools.

**Fields**:

- Span kind: `CLIENT`.
- Span name: `{gen_ai.operation.name} {gen_ai.request.model}` when `gen_ai.request.model` is already known from the normal relay flow.
- Required attributes: `gen_ai.operation.name`, `gen_ai.provider.name`.
- Conditional attributes allowed when already known without extra client request parsing: `error.type`, `gen_ai.output.type`, `gen_ai.request.model`, `gen_ai.request.stream`, and `server.port` when `server.address` is set.
- Recommended attributes allowed when already known without extra client request parsing: `gen_ai.response.finish_reasons`, `gen_ai.response.id`, `gen_ai.response.model`, `gen_ai.response.time_to_first_chunk`, `gen_ai.usage.audio.cache_read.input_tokens`, `gen_ai.usage.audio.input_tokens`, `gen_ai.usage.audio.output_tokens`, `gen_ai.usage.cache_read.input_tokens`, `gen_ai.usage.cache_write.input_tokens`, `gen_ai.usage.image.cache_read.input_tokens`, `gen_ai.usage.image.input_tokens`, `gen_ai.usage.image.output_tokens`, `gen_ai.usage.input_tokens`, `gen_ai.usage.output_tokens`, `gen_ai.usage.reasoning.output_tokens`, `gen_ai.usage.text.cache_read.input_tokens`, `gen_ai.usage.text.input_tokens`, `gen_ai.usage.text.output_tokens`, and `server.address`.
- Excluded input-detail and content attributes: `gen_ai.conversation.id`, `gen_ai.conversation.compacted`, `gen_ai.input.messages`, `gen_ai.output.messages`, `gen_ai.prompt.name`, `gen_ai.prompt.version`, `gen_ai.prompt.variable.*`, `gen_ai.request.choice.count`, `gen_ai.request.frequency_penalty`, `gen_ai.request.max_tokens`, `gen_ai.request.presence_penalty`, `gen_ai.request.previous_response.id`, `gen_ai.request.reasoning.level`, `gen_ai.request.seed`, `gen_ai.request.stop_sequences`, `gen_ai.request.temperature`, `gen_ai.request.top_k`, `gen_ai.request.top_p`, `gen_ai.system_instructions`, and `gen_ai.tool.definitions`.
- OneAPI-specific fields: `oneapi.channel_id`, `oneapi.stream`, `oneapi.upstream_address`, and `oneapi.upstream_url`.

**Validation Rules**:

- Standard GenAI fields must follow OpenTelemetry GenAI inference client span names exactly.
- OneAPI-specific fields must use a clear OneAPI namespace or equivalent distinction.
- Unknown optional fields must be omitted rather than invented.
- Excluded input-detail and content attributes must not be emitted by tracing unless a future feature explicitly provides these values without extra client request parsing and with sanitization controls.

**Relationships**:

- Enriches OpenTelemetry Relay Traces.
- Supplies overlapping fields to Structured Log Entries for operational correlation.

## Prompt Preview

**Purpose**: Bounded, sanitized prompt context included in logs for debugging prompt-related incidents.

**Fields**:

- `value`: Sanitized prompt preview.
- `original_length`: Length of the sanitized source before preview truncation when available.
- `truncated`: Whether the preview was shortened.

**Validation Rules**:

- Sensitive values must be redacted before the preview is emitted.
- Sanitized values longer than 200 characters must be shortened to first 100 characters, `...`, and last 100 characters.
- Sanitized values at or below 200 characters must remain unchanged.
- The preview must be character-safe and must not split multibyte characters.
