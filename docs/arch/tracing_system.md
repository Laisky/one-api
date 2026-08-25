# Request Tracing System Architecture

## Overview

Request tracing is implemented with OpenTelemetry middleware and request-scoped structured logs. The old database-backed request tracing path is removed from backend request handling: requests no longer create trace rows, update trace timestamps, append trace external-call records, or rely on DB tracing feature flags.

Historical trace tables and read endpoints may still exist for previously stored data or older operational surfaces, but new request handling does not write to those tables.

## Runtime Flow

`main.go` always registers `otelgin.Middleware(config.OpenTelemetryServiceName)` and the storage-free `middleware.TracingMiddleware()`. If no OpenTelemetry exporter endpoint is configured, the default OpenTelemetry provider behaves as a no-op and startup and requests continue normally.

The tracing middleware records these lifecycle events through the active span when one is recording and emits synchronized debug logs through the request-scoped logger:

- `request.received`
- `relay.start`
- `upstream.request.sent`
- `upstream.first_byte`
- `upstream.complete`
- `response.complete`
- `error`

Relay code records upstream timing events at the existing dispatch and response points without writing to storage.

## Logging

Request logs must use the context-aware logger from `gmw.GetLogger(c)` in Gin request paths. `LOG_FORMAT=console` keeps colored console output. `LOG_FORMAT=json` selects JSON logger encoding and disables colored Gin middleware output.

When a valid OpenTelemetry span context exists, logs may include standard `trace_id` and `span_id` values. The service does not fabricate IDs when the OpenTelemetry context is missing or invalid.

Prompt previews are log-only. They are sanitized and bounded to the first 100 characters, `...`, and the last 100 characters. Tracing must not parse, re-serialize, or inspect client request bodies solely to populate trace fields.

## GenAI Fields

GenAI trace attributes follow the OpenTelemetry GenAI inference client convention for values already known from normal routing, relay, upstream response, and usage accounting. Allowed field names include:

- `gen_ai.operation.name`
- `gen_ai.request.model`
- `gen_ai.response.model`
- `gen_ai.usage.input_tokens`
- `gen_ai.usage.output_tokens`

OneAPI-specific routing context uses the `oneapi.*` namespace:

- `oneapi.channel_id`
- `oneapi.stream`
- `oneapi.upstream_address`
- `oneapi.upstream_url`

Input-detail and content fields are intentionally excluded by default, including `gen_ai.input.messages`, `gen_ai.output.messages`, system instructions, prompt variables, tool definitions, and sampling parameters that would require extra client request parsing.
