package tracing

// OTLP trace sink (proposal docs/proposals/20260905_observability-data-tiering.md,
// Phase 1 / W1.2, W3.1).
//
// The `traces` table and an OpenTelemetry span carry the same information: the
// pre-proposal model layer already mirrored every lifecycle mark onto the active
// span. This sink makes that the only destination, so a deployment with an OTLP
// collector writes zero trace rows to SQL.
//
// WHY THIS SINK NO LONGER STARTS ITS OWN SERVER SPAN
//
// An earlier revision started a second `one_api.request` SERVER span here and
// justified it with "otelgin's span has already ended by the time a request
// completes". That claim was never measured and it is wrong. main.go registers
// otelgin.Middleware BEFORE middleware.TracingMiddleware, so otelgin's deferred
// span.End() runs AFTER TracingMiddleware's deferred end-of-request hook, which
// is what calls Submit.
//
// TestOtelginSpanIsLiveWhenTraceSinkSubmits (middleware package) drives the real
// middleware chain with sdktrace/tracetest.NewInMemoryExporter and measures it:
// at Submit time the context span IsRecording() is true and its span id equals
// the id of the single SERVER span the exporter later receives. The pre-fix code
// produced TWO nested SERVER spans for one request; it now produces one.
//
// So: when the enclosing request span is still live, this sink ENRICHES it and
// never ends it (otelgin owns its lifetime, and therefore its start/end
// timestamps, which already bracket the request exactly). A new span is started
// only when the context carries no live span at all, which happens when:
//
//   - the request never passed through otelgin (a synthetic gin context, e.g.
//     channel testing, or OTEL_ENABLED false -- itself rejected for TRACE_SINK
//     otlp by config.ValidateTraceSinkOpenTelemetryConfig); or
//   - SDK head sampling made the enclosing span nonrecording. Local selection
//     cannot recover a span the SDK dropped: the child started here inherits the
//     nonrecording parent decision and the sink reports span_record_failed.
//
// Note also that enriching the otelgin span does NOT suppress its independent
// SDK export: TRACE_SAMPLE_RATE governs local enrichment and SQL rows only.
// Reducing OTLP span volume requires the separately configured SDK/collector
// sampling policy (proposal section 4).
//
// Section 3.2 of the proposal restricts `otlp` to TRACE_WRITE_MODE=batched, and
// only the batched path reaches a sink at all, so Submit always runs on the
// request goroutine while the enclosing span is open.
//
// METRICS
//
// metrics.TraceOutcomeSpanRecorded counted here means the local SDK accepted a
// recording span. It is not proof that any collector received or persisted
// anything. Asynchronous processor and transport outcomes belong at the
// exporter boundary, which this sink cannot observe.

import (
	"context"

	"github.com/Laisky/errors/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/Laisky/one-api/common/metrics"
	"github.com/Laisky/one-api/model"
)

// otlpSinkTracerName scopes the instrumentation the sink emits under.
const otlpSinkTracerName = "github.com/Laisky/one-api/common/tracing"

// otlpFallbackSpanName names the span started only when no live request span is
// available; the enrichment path keeps otelgin's route-derived span name.
const otlpFallbackSpanName = "one_api.request"

// otlpSink emits finished traces onto the active OpenTelemetry request span.
type otlpSink struct{}

// newOTLPSink builds the OTLP trace sink.
//
// Parameters: none.
//
// Return values:
//   - TraceSink: the sink; it holds no state and needs no shutdown.
func newOTLPSink() TraceSink { return otlpSink{} }

// Submit implements TraceSink.Submit by completing exactly one SERVER span per
// finished trace.
//
// Parameters:
//   - ctx: parent context; a live recording span in it is enriched in place, and
//     a valid-but-nonrecording span context is used as the parent of a new span.
//   - row: the finished trace row.
//
// Return values:
//   - error: wrapped failure when the row's timestamp document cannot be read,
//     or when the SDK refuses to record the span. Both are counted as
//     TraceOutcomeSpanRecordFailed; asynchronous transport is owned by the
//     exporter and is not observable here.
func (otlpSink) Submit(ctx context.Context, row *model.Trace) error {
	if row == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	timestamps, err := row.GetTraceTimestamps()
	if err != nil {
		metrics.RecordTraceOutcome(metrics.TraceOutcomeSpanRecordFailed, 1)
		return errors.Wrapf(err, "read trace timestamps for span export")
	}

	// The enclosing span, when it is still recording, is the request's one and
	// only SERVER span. Enrich it and leave its lifetime to otelgin.
	if span := oteltrace.SpanFromContext(ctx); span.IsRecording() {
		enrichActiveRequestSpan(span, row, timestamps)
		metrics.RecordTraceOutcome(metrics.TraceOutcomeSpanRecorded, 1)
		return nil
	}

	return emitStandaloneRequestSpan(ctx, row, timestamps)
}

// enrichActiveRequestSpan completes the live request span with the finished
// trace's state.
//
// It deliberately does NOT re-add the lifecycle events. In batched mode
// RecordTraceStart, RecordTraceTimestamp and RecordTraceStatus already added
// each of them onto this same span at the instant it happened, which is more
// accurate than replaying them here; re-adding would duplicate every event. What
// only the completed document knows -- the external-call timeline and the final
// error status -- is added here. Attributes are keyed, so re-setting them is
// idempotent and simply guarantees the final values are present.
//
// Parameters:
//   - span: the live, recording request span; its lifetime belongs to otelgin.
//   - row: the finished trace row.
//   - timestamps: the row's decoded timestamp document.
//
// Return values: none.
func enrichActiveRequestSpan(span oteltrace.Span, row *model.Trace, timestamps *model.TraceTimestamps) {
	span.SetAttributes(requestSpanAttributes(row)...)
	addExternalCallEvents(span, timestamps)
	if row.Status >= 400 {
		span.SetStatus(codes.Error, "")
	}
}

// emitStandaloneRequestSpan starts and ends one SERVER span for a request whose
// enclosing span is not available, keeping the parentage carried by ctx.
//
// Parameters:
//   - ctx: the context whose span context, when valid, parents the new span.
//   - row: the finished trace row.
//   - timestamps: the row's decoded timestamp document.
//
// Return values:
//   - error: wrapped failure when the SDK will not record the span, which is
//     also counted as TraceOutcomeSpanRecordFailed.
func emitStandaloneRequestSpan(ctx context.Context, row *model.Trace, timestamps *model.TraceTimestamps) error {
	startMillis := row.CreatedAt
	if timestamps.RequestReceived != nil {
		startMillis = *timestamps.RequestReceived
	}

	tracer := otel.GetTracerProvider().Tracer(otlpSinkTracerName)
	_, span := tracer.Start(ctx, otlpFallbackSpanName,
		oteltrace.WithSpanKind(oteltrace.SpanKindServer),
		oteltrace.WithTimestamp(unixMilliToTime(startMillis)),
	)
	if !span.IsRecording() {
		span.End()
		metrics.RecordTraceOutcome(metrics.TraceOutcomeSpanRecordFailed, 1)
		return errors.WithStack(errors.New("OpenTelemetry trace provider is not recording"))
	}

	span.SetAttributes(requestSpanAttributes(row)...)

	addTimestampEvent(span, model.TimestampRequestReceived, timestamps.RequestReceived)
	addTimestampEvent(span, model.TimestampRequestForwarded, timestamps.RequestForwarded)
	addTimestampEvent(span, model.TimestampFirstUpstreamResponse, timestamps.FirstUpstreamResponse)
	addTimestampEvent(span, model.TimestampFirstClientResponse, timestamps.FirstClientResponse)
	addTimestampEvent(span, model.TimestampUpstreamCompleted, timestamps.UpstreamCompleted)
	addTimestampEvent(span, model.TimestampRequestCompleted, timestamps.RequestCompleted)
	addExternalCallEvents(span, timestamps)

	if row.Status >= 400 {
		span.SetStatus(codes.Error, "")
	}

	endMillis := row.CreatedAt
	if timestamps.RequestCompleted != nil {
		endMillis = *timestamps.RequestCompleted
	}
	span.End(oteltrace.WithTimestamp(unixMilliToTime(endMillis)))

	metrics.RecordTraceOutcome(metrics.TraceOutcomeSpanRecorded, 1)
	return nil
}

// requestSpanAttributes builds the one-api attributes carried by a request span.
//
// The URL is the already-sanitized column value, so no credential reaches the
// exporter.
//
// Parameters:
//   - row: the finished trace row.
//
// Return values:
//   - []attribute.KeyValue: the attribute set, in a stable order.
func requestSpanAttributes(row *model.Trace) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("one_api.trace_id", row.TraceId),
		attribute.String("one_api.url", row.URL),
		attribute.String("one_api.method", row.Method),
		attribute.Int64("one_api.body_size", row.BodySize),
		attribute.Int("one_api.status", row.Status),
	}
}

// addExternalCallEvents replays the external-call timeline onto a span.
//
// Parameters:
//   - span: the span receiving the events.
//   - timestamps: the decoded timestamp document.
//
// Return values: none.
func addExternalCallEvents(span oteltrace.Span, timestamps *model.TraceTimestamps) {
	for _, call := range timestamps.ExternalCalls {
		span.AddEvent("one_api.external_call",
			oteltrace.WithTimestamp(unixMilliToTime(call.StartedAt)),
			oteltrace.WithAttributes(
				attribute.String("source", call.Source),
				attribute.String("tool", call.Tool),
				attribute.Int("server_id", call.ServerID),
				attribute.String("server_label", call.ServerLabel),
				attribute.Int64("duration_ms", call.DurationMs),
				attribute.Bool("is_error", call.IsError),
			))
	}
}

// Flush implements TraceSink.Flush as a no-op: span export is owned by the
// OpenTelemetry SDK's batch span processor, which main flushes on shutdown.
//
// Parameters:
//   - ctx: unused.
//
// Return values:
//   - error: always nil.
func (otlpSink) Flush(context.Context) error { return nil }

// Close implements TraceSink.Close as a no-op.
//
// Parameters:
//   - ctx: unused.
//
// Return values:
//   - error: always nil.
func (otlpSink) Close(context.Context) error { return nil }

// addTimestampEvent adds one lifecycle event at its recorded instant.
//
// Parameters:
//   - span: the span receiving the event.
//   - name: the lifecycle key.
//   - at: the Unix-millisecond instant, or nil when the mark never happened.
//
// Return values: none.
func addTimestampEvent(span oteltrace.Span, name string, at *int64) {
	if at == nil {
		return
	}
	span.AddEvent(name, oteltrace.WithTimestamp(unixMilliToTime(*at)))
}
