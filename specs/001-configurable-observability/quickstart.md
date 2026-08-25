# Quickstart: Configurable Observability

## Prerequisites

- Go toolchain compatible with the repository's `go.mod`.
- Existing database setup available for normal one-api local execution.
- No frontend changes are required for this validation.

## Focused Test Commands

Run targeted checks while implementing:

```sh
go test ./common/config ./common/logger ./common/tracing ./middleware ./relay/...
```

Run broader backend checks before handoff when feasible:

```sh
go vet ./...
go test -race ./...
```

## Scenario 1: Database Trace Request Path Removed

1. Start the service with normal configuration:

   ```sh
   go run .
   ```

2. Send a normal supported API request.
3. Confirm the request completes according to existing API behavior.
4. Confirm request handling does not create or update database-backed trace records.
5. Confirm no database tracing environment flag is required or documented for this behavior.

Expected outcome: request processing works, database trace persistence is absent from the request path, and no frontend/API compatibility work is required.

Implementation checkpoint: the old database-backed tracing middleware and request-path trace write/update calls are removed. The replacement path is OpenTelemetry middleware plus structured logging.

## Scenario 2: OpenTelemetry Middleware Without Exporter

1. Start the service without OpenTelemetry exporter or collector configuration:

   ```sh
   LOG_FORMAT=json go run .
   ```

2. Send a normal relay request.
3. Inspect emitted logs and request behavior.

Expected outcome: OpenTelemetry middleware is registered, request processing succeeds, and missing exporter configuration does not fail startup or requests.

## Scenario 3: JSON Log Output With OTel Context

1. Start the service with JSON logs and OpenTelemetry request context available:

   ```sh
   LOG_FORMAT=json OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4318 go run .
   ```

2. Send a request with a valid `traceparent` header:

   ```sh
   curl -H 'traceparent: 00-0123456789abcdef0123456789abcdef-0123456789abcdef-01' http://localhost:3000/
   ```

3. Inspect emitted log lines.

Expected outcome: sampled application log lines are valid JSON, include `severity_text`, and request-scoped entries include `trace_id` and `span_id` values that match OpenTelemetry format.

Implementation checkpoint: log correlation uses the active OpenTelemetry span context and must not call database trace write helpers.

## Scenario 4: Relay Timing Events

1. Start the service with JSON logs:

   ```sh
   LOG_FORMAT=json go run .
   ```

2. Send a model request through relay.
3. Inspect OpenTelemetry spans/events when an exporter is available and inspect request-scoped JSON logs.

Expected outcome: occurred relay timing points have synchronized trace events and log entries for request received, relay start, upstream request sent, first upstream byte, upstream complete, response complete, and errors.

## Scenario 5: GenAI and OneAPI Routing Fields

1. Start the service with JSON logs and optional OpenTelemetry export:

   ```sh
   LOG_FORMAT=json OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4318 go run .
   ```

2. Send both a streaming and non-streaming model request through a known channel.
3. Inspect OpenTelemetry traces and JSON request logs.

Expected outcome: traces and request-scoped logs use exact OpenTelemetry GenAI inference client span field names for values already known from normal relay flow, including span kind `CLIENT`, span name `{gen_ai.operation.name} {gen_ai.request.model}`, required provider/operation fields, request/response model fields, `gen_ai.response.time_to_first_chunk`, finish reasons, response ID, usage-token fields, and OneAPI routing fields `oneapi.channel_id`, `oneapi.stream`, `oneapi.upstream_address`, and `oneapi.upstream_url`. Tracing does not parse or re-serialize the client request body to populate optional request-parameter or content attributes.

## Scenario 6: Prompt Preview Truncation

1. Start the service with JSON logs:

   ```sh
   LOG_FORMAT=json go run .
   ```

2. Send a model request with prompt text longer than 200 characters.
3. Inspect emitted request-scoped JSON logs.

Expected outcome: `prompt_preview` contains the sanitized first 100 characters, then `...`, then the sanitized last 100 characters, and no full large prompt body is emitted.

## Scenario 7: JSON Log Output Without OTel Context

1. Start the service with JSON logs but without valid OpenTelemetry request context:

   ```sh
   LOG_FORMAT=json go run .
   ```

2. Send a normal request without a valid `traceparent` header.
3. Inspect emitted log lines.

Expected outcome: log lines remain valid JSON and include `severity_text`; the service does not invent project-specific `trace_id` or `span_id` values.

## Scenario 8: Default Console Logs

1. Start the service without selecting JSON logs:

   ```sh
   go run .
   ```

2. Send a normal request.
3. Inspect emitted logs.

Expected outcome: existing console-style log output remains available.
