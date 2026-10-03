package otelbridge

// Trace-correlation field for the OTLP application-log bridge (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 3 / W3.2:
// "The bridge needs a context-bearing field to emit with request context;
// otherwise it uses a background context").
//
// Correlation in OTLP is not an attribute. trace_id, span_id and trace_flags
// are top-level LogRecord protocol fields, and the SDK fills them from the
// SpanContext found in the context passed to Logger.Emit. A bridge therefore
// has exactly one job: get a real SpanContext to the emit call. Putting
// "trace_id" in the attributes instead produces records that no backend joins.
//
// The upstream otelzap answer is to log a whole context.Context as a field.
// This repository cannot adopt that as its primary mechanism. Request loggers
// here are deliberately context-free: identity.Bind stores the request logger
// by value and relayctx.Detach snapshots it into background goroutines that
// outlive the request, so binding the request context into the logger would
// keep the gin context, its request, and everything they reference alive for
// the lifetime of the detached work.
//
// SpanContextField carries the 24 bytes that actually matter instead, and it
// does so in a field zap's encoders ignore completely.

import (
	"context"

	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// SpanContextFieldKey is the key of the field produced by SpanContextField.
//
// It is exported so a test can assert the field's shape. It never appears in
// any output: the field's type makes every encoder skip it.
const SpanContextFieldKey = "otel_span_context"

// SpanContextField returns a zap field that carries a request's OpenTelemetry
// span context to the OTLP bridge and is invisible to every other sink.
//
// The field's type is zapcore.SkipType, which zapcore.Field.AddTo handles as a
// no-op, so the console and JSON encoders that write the log file and stdout
// never render it. That is what lets the bridge be enabled without changing a
// single byte of the application log files, which section 2.1 requires: "Full
// application log-line fields, existing sink selection". The bridge reads
// Field.Interface directly, before AddTo is ever called, so it sees what the
// encoders cannot.
//
// Parameters:
//   - ctx: the context whose span context should correlate later log lines; a
//     nil context, or one with no valid span, yields zap.Skip().
//
// Return values:
//   - zap.Field: a skip-typed field carrying the span context, or zap.Skip()
//     when there is nothing to correlate.
func SpanContextField(ctx context.Context) zap.Field {
	if ctx == nil {
		return zap.Skip()
	}
	return SpanContextFieldFrom(oteltrace.SpanContextFromContext(ctx))
}

// SpanContextFieldFrom returns a correlation field for an already-extracted
// span context.
//
// Parameters:
//   - sc: the span context to carry; an invalid one yields zap.Skip().
//
// Return values:
//   - zap.Field: a skip-typed field carrying sc, or zap.Skip().
func SpanContextFieldFrom(sc oteltrace.SpanContext) zap.Field {
	if !sc.IsValid() {
		return zap.Skip()
	}
	return zap.Field{
		Key:       SpanContextFieldKey,
		Type:      zapcore.SkipType,
		Interface: sc,
	}
}

// spanContextFromField reports whether a field is a correlation field and
// returns the span context it carries.
//
// Parameters:
//   - field: the field to inspect.
//
// Return values:
//   - oteltrace.SpanContext: the carried span context, valid only when ok.
//   - bool: true when the field is a correlation field with a valid span
//     context, which is also the signal that it must not be encoded as an
//     attribute.
func spanContextFromField(field zapcore.Field) (oteltrace.SpanContext, bool) {
	if field.Type != zapcore.SkipType || field.Interface == nil {
		return oteltrace.SpanContext{}, false
	}
	sc, ok := field.Interface.(oteltrace.SpanContext)
	if !ok || !sc.IsValid() {
		return oteltrace.SpanContext{}, false
	}
	return sc, true
}
