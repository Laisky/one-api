# Contract: Observability Configuration

This contract describes operator-visible configuration behavior for the configurable observability feature.

## Environment Variables

| Variable | Values | Default | Behavior |
|----------|--------|---------|----------|
| `LOG_FORMAT` | `console` or `json` | `console` | Selects operational log encoding for stdout and file sinks. |
| OpenTelemetry exporter variables | Existing OpenTelemetry-supported values | unset | Optional exporter configuration; absence must not prevent middleware registration or request processing. |

## Removed Database Trace Storage Behavior

- Backend request handling must not create or update database-backed trace records.
- Backend request handling must not retain a database tracing environment flag, middleware gate, helper check, or compatibility branch.
- OpenTelemetry tracing middleware replaces the old database-backed tracing request path.
- Trace-related frontend or API calls are not given a new compatibility contract by this feature and may surface existing errors when records are unavailable.
- Removing database-backed trace storage does not require migration, deletion, or hiding of historical trace records.

## OpenTelemetry Trace Behavior

- OpenTelemetry middleware must be registered for relay request paths.
- When no exporter or collector is configured, OpenTelemetry no-op/default behavior must preserve startup and request processing.
- Relay traces must record request received, relay start, upstream request sent, first upstream byte, upstream complete, response complete, and error events when each point occurs.
- GenAI inference spans must use span kind `CLIENT` and span name `{gen_ai.operation.name} {gen_ai.request.model}` when request model is already known from the normal relay flow.
- GenAI inference spans must use OpenTelemetry semantic convention attribute names exactly.
- Required GenAI inference span attributes are `gen_ai.operation.name` and `gen_ai.provider.name` when known.
- Conditional GenAI inference span attributes allowed when already known without extra client request parsing are `error.type`, `gen_ai.output.type`, `gen_ai.request.model`, `gen_ai.request.stream`, and `server.port` when `server.address` is set.
- Recommended GenAI inference span attributes allowed when already known without extra client request parsing are `gen_ai.response.finish_reasons`, `gen_ai.response.id`, `gen_ai.response.model`, `gen_ai.response.time_to_first_chunk`, `gen_ai.usage.audio.cache_read.input_tokens`, `gen_ai.usage.audio.input_tokens`, `gen_ai.usage.audio.output_tokens`, `gen_ai.usage.cache_read.input_tokens`, `gen_ai.usage.cache_write.input_tokens`, `gen_ai.usage.image.cache_read.input_tokens`, `gen_ai.usage.image.input_tokens`, `gen_ai.usage.image.output_tokens`, `gen_ai.usage.input_tokens`, `gen_ai.usage.output_tokens`, `gen_ai.usage.reasoning.output_tokens`, `gen_ai.usage.text.cache_read.input_tokens`, `gen_ai.usage.text.input_tokens`, `gen_ai.usage.text.output_tokens`, and `server.address`.
- Tracing must not emit input-detail or content attributes such as `gen_ai.conversation.id`, `gen_ai.conversation.compacted`, `gen_ai.input.messages`, `gen_ai.output.messages`, `gen_ai.prompt.name`, `gen_ai.prompt.version`, `gen_ai.prompt.variable.*`, `gen_ai.request.choice.count`, `gen_ai.request.frequency_penalty`, `gen_ai.request.max_tokens`, `gen_ai.request.presence_penalty`, `gen_ai.request.previous_response.id`, `gen_ai.request.reasoning.level`, `gen_ai.request.seed`, `gen_ai.request.stop_sequences`, `gen_ai.request.temperature`, `gen_ai.request.top_k`, `gen_ai.request.top_p`, `gen_ai.system_instructions`, and `gen_ai.tool.definitions`, unless a future feature explicitly provides these values without extra client request parsing and with sanitization controls.
- Tracing must not parse, re-serialize, or inspect the client request body solely to populate GenAI fields.
- Trace data must include OneAPI-specific `oneapi.channel_id`, `oneapi.stream`, `oneapi.upstream_address`, and `oneapi.upstream_url` when known.
- OneAPI-specific trace fields must remain distinguishable from standard OpenTelemetry fields.

## JSON Log Behavior

- When `LOG_FORMAT=json`, each operational log line emitted through the application logger must be valid JSON.
- JSON log entries must include `severity_text`.
- Request-scoped JSON log entries must include `trace_id` and `span_id` when valid OpenTelemetry span context is available.
- Request-scoped JSON log entries must include synchronized relay timing event names when emitted for required relay timing points.
- Request-scoped JSON log entries must include known GenAI inference attributes using the same OpenTelemetry semantic convention field names as traces, subject to the same no-extra-client-request-parsing rule.
- Request-scoped JSON log entries must include `oneapi.channel_id`, `oneapi.stream`, `oneapi.upstream_address`, and `oneapi.upstream_url` when those values are known.
- Request-scoped JSON log entries must include `prompt_preview` when prompt text is safely available for logging.
- `trace_id` must follow OpenTelemetry trace ID format: 32 hexadecimal characters and not all zeroes.
- `span_id` must follow OpenTelemetry span ID format: 16 hexadecimal characters and not all zeroes.
- When valid OpenTelemetry context is unavailable, the system must not invent project-specific values for `trace_id` or `span_id`.
- Logs must not include API keys, passwords, tokens, credentials, or sensitive request payloads.
- `prompt_preview` must use the full sanitized prompt when it is 200 characters or shorter.
- `prompt_preview` must use first 100 characters, then `...`, then last 100 characters when the sanitized prompt is longer than 200 characters.

## Non-JSON Log Behavior

- When `LOG_FORMAT` is absent, empty, or `console`, existing readable console-style output remains available.
- Existing log rotation, retention, and alert push behavior continues to use the selected logger output format where supported by the current sink.
