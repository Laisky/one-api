package tracing

// OTLP trace sink (proposal docs/proposals/20260905_observability-data-tiering.md,
// Phase 1 / W1.2, W3.1).
//
// The `traces` table and an OpenTelemetry span carry the same information: the
// pre-proposal model layer already mirrored every lifecycle mark onto the active
// span. This sink makes that the only destination, so a deployment with an OTLP
// collector writes zero trace rows to SQL.
//
// The span emitted here is a short, self-contained span whose start and end
// match the request, carrying the lifecycle marks as span events. It is
// deliberately independent of the otelgin server span: otelgin's span has
// already ended by the time a request completes, and re-opening it is not
// possible.

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

// otlpSink emits finished traces as OpenTelemetry spans.
type otlpSink struct{}

// newOTLPSink builds the OTLP trace sink.
//
// Parameters: none.
//
// Return values:
//   - TraceSink: the sink; it holds no state and needs no shutdown.
func newOTLPSink() TraceSink { return otlpSink{} }

// Submit implements TraceSink.Submit by emitting one span per finished trace.
//
// Parameters:
//   - ctx: parent context; a valid span context in it links the emitted span to
//     the surrounding distributed trace.
//   - row: the finished trace row.
//
// Return values:
//   - error: wrapped failure when the row's timestamp document cannot be read.
func (otlpSink) Submit(ctx context.Context, row *model.Trace) error {
	if row == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	timestamps, err := row.GetTraceTimestamps()
	if err != nil {
		return err
	}

	startMillis := row.CreatedAt
	if timestamps.RequestReceived != nil {
		startMillis = *timestamps.RequestReceived
	}

	tracer := otel.GetTracerProvider().Tracer(otlpSinkTracerName)
	_, span := tracer.Start(ctx, "one_api.request",
		oteltrace.WithSpanKind(oteltrace.SpanKindServer),
		oteltrace.WithTimestamp(unixMilliToTime(startMillis)),
	)
	if !span.IsRecording() {
		span.End()
		return errors.New("OpenTelemetry trace provider is not recording")
	}

	span.SetAttributes(
		attribute.String("one_api.trace_id", row.TraceId),
		attribute.String("one_api.url", row.URL),
		attribute.String("one_api.method", row.Method),
		attribute.Int64("one_api.body_size", row.BodySize),
		attribute.Int("one_api.status", row.Status),
	)

	addTimestampEvent(span, model.TimestampRequestReceived, timestamps.RequestReceived)
	addTimestampEvent(span, model.TimestampRequestForwarded, timestamps.RequestForwarded)
	addTimestampEvent(span, model.TimestampFirstUpstreamResponse, timestamps.FirstUpstreamResponse)
	addTimestampEvent(span, model.TimestampFirstClientResponse, timestamps.FirstClientResponse)
	addTimestampEvent(span, model.TimestampUpstreamCompleted, timestamps.UpstreamCompleted)
	addTimestampEvent(span, model.TimestampRequestCompleted, timestamps.RequestCompleted)

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

	if row.Status >= 400 {
		span.SetStatus(codes.Error, "")
	}

	endMillis := row.CreatedAt
	if timestamps.RequestCompleted != nil {
		endMillis = *timestamps.RequestCompleted
	}
	span.End(oteltrace.WithTimestamp(unixMilliToTime(endMillis)))

	metrics.RecordTraceOutcome(metrics.TraceOutcomeExported, 1)
	return nil
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
