# Operational log content and privacy

The optional OTLP application-log sink preserves operational messages and
ordinary scalar structured fields, including the native SDK EventName field. It shares severity, timestamp, trace/span
correlation and trusted Zap caller metadata with the local logger. New or dynamic
benign messages do not require a message catalog, and new scalar keys do not
require registration. Local stdout/file and alert sinks keep their existing
behavior, except the source-marked raw-body debug preview is now redacted.

Both the production Zap bridge and SDK pre-admission processor sanitize content.
The SDK stage also covers direct SDK producers. The fixed SDK scope and immutable
safe-resource checks remain in effect; ordinary record attributes cannot override
service/host/environment identity or forge trusted caller metadata.

## What survives

- Original operational prose, including cache/option sync, initialization,
  retention, batch updates, shutdown and ordinary transport error reasons.
- String, boolean, signed integer and finite floating-point fields, including
  unknown operational keys, negative deltas, `sync_frequency`, `content_length`,
  `query_latency`, `token_count` and `password_reset_required`.
- Existing type/range validation for status, numeric identity, usage and transport
  counters. Generic `status` preserves bounded nonnegative channel states and
  sanitized native status strings; `status_code` keeps strict HTTP100–599
  validation. For example, `prompt_tokens` is a counter, not a prompt payload.
- Ordinary `error.Error()` diagnostic text after sanitization. Verbose exception
  formatting and arbitrary object/array/reflection/Stringer serializers are not
  invoked by the OTLP projection.

Unsupported or invalid fields are omitted with `log.fields_omitted=true`.
This flag never contains the omitted value. Text is bounded before normalization
and regular-expression work; the SDK's attribute limits, bounded record/byte
admission, drop counters, exporter timeout and shutdown behavior still apply.
Sanitized SDK EventName bytes participate in admission accounting, so a large
event cannot bypass the byte ceiling; event.name attributes are separate fields.

## Targeted protection

Credential/header and request/response/prompt payload fields are redacted by their
canonical names or recognized credential/payload suffixes. This is a sensitive
field policy rather than an allowlist of operational names. Recognizable
credential assignments, authentication values and API-key/JWT forms are removed
from messages and scalar strings. Quoted secrets include escaped, unterminated and
budget-truncated values. A labeled raw payload is replaced while preserving the
diagnostic prefix. This also applies to untyped text such as `read body: ...`;
read/reading prefixes do not establish provenance or bypass redaction.

For an actual typed Zap ErrorType wrapping error with an Unwrap cause, the bridge
can separate operation and sanitized cause, e.g. `read body (cause: unexpected EOF)`.
Traversal is bounded to eight edges. This projection never applies to message
Body, ordinary string fields or direct SDK text, and it does not classify a
private unlabeled cause as safe; source marking is still required.

Absolute and relative URL fields retain their route while removing userinfo,
all query values and fragments. Recognizable credential-bearing path segments
and data URLs are redacted. Malformed URL fields fail closed. Embedded absolute
URLs in ordinary error text receive the same projection.

These heuristics cannot identify an unlabeled arbitrary prompt, personal data or
unknown credential format embedded in otherwise ordinary text. Callers must
continue selecting private values with `logger.Redact` and source-specific
redaction. `MessageContainsRawBody` also protects the non-retry debug preview;
the actual API response is unchanged. Existing `RedactJSONErrorValue` preserves
decoder context while removing a caller-selected raw numeric payload value.
Do not log request bodies merely because another sink might sanitize them.

Malformed `OTEL_RESOURCE_ATTRIBUTES` syntax must fail with a fixed error before
the SDK can echo parse values through its global error handler. Validation does
not rewrite environment values or permit arbitrary resource keys. Log-exporter
endpoint/header/timeout/insecure syntax receives the same fixed-key validation
only where the pinned SDK would consume it; valid signal/global precedence and
configured options remain unchanged. This does not alter trace/metric exporter
configuration or suppress the process-global OTel error handler.

## Dependency and verification

This repair keeps the existing `go-utils/v6 v6.4.0` pin. That version does not
export the later `SanitizeDiagnosticText` helper, so the bounded adapter lives
locally in the OTLP bridge; no shared-library or collector change is required.

Local regressions use synthetic canaries and an in-memory SDK exporter, including
the complete Zap → SDK privacy → exporter path, direct SDK producers, URL and
malformed quoted-secret controls, immutable resource/scope rejection and the
source-marked raw-body debug path. They do not send logs or read production
payloads. Collector delivery/storage qualification is a separate concern.
