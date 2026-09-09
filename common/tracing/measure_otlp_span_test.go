package tracing

// MEASUREMENT (not correctness) for the W1 OTLP span-reuse change -- proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 1 / W1, "OTLP":
// "Enrich the existing active request span, with one request SERVER span and
// correct parentage, timestamps, status and events."
//
// The correctness proof -- that the enclosing otelgin span is still live and is
// the one enriched, under the REAL middleware order from main.go -- is
// middleware/tracing_otlp_span_test.go. It is cited by the report rather than
// duplicated here.
//
// What this file adds is the export-volume consequence: how many spans, and how
// much attribute and event payload, one request hands to the SDK before and
// after. The "before" arm calls emitStandaloneRequestSpan, which is the exact
// function the pre-remediation sink called unconditionally and which the shipped
// sink still calls when no enclosing span exists.

import (
	"context"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// spanPayload is the measured export volume of one finished span.
type spanPayload struct {
	// Spans is how many spans the arm produced.
	Spans int
	// Attributes is the total attribute count across those spans.
	Attributes int
	// Events is the total event count across those spans.
	Events int
	// PayloadBytes is a PROXY for wire size: the summed length of attribute
	// keys, string attribute values and event names. It is not an OTLP
	// serialization measurement -- the protobuf transform lives in an internal
	// package -- and it excludes ids, timestamps and framing.
	PayloadBytes int
}

// measureSpanPayload sums the export volume of every span the exporter holds.
//
// Parameters:
//   - spans: the finished spans to measure.
//
// Return values:
//   - spanPayload: the aggregate volume.
func measureSpanPayload(spans tracetest.SpanStubs) spanPayload {
	out := spanPayload{Spans: len(spans)}
	for _, span := range spans {
		out.Attributes += len(span.Attributes)
		out.Events += len(span.Events)
		out.PayloadBytes += len(span.Name)
		for _, attr := range span.Attributes {
			out.PayloadBytes += len(string(attr.Key))
			if attr.Value.Type() == 0 {
				continue
			}
			out.PayloadBytes += len(attr.Value.Emit())
		}
		for _, event := range span.Events {
			out.PayloadBytes += len(event.Name)
			for _, attr := range event.Attributes {
				out.PayloadBytes += len(string(attr.Key)) + len(attr.Value.Emit())
			}
		}
	}
	return out
}

// runOneOTLPRequest drives one request's worth of span production and returns
// what the exporter received.
//
// Parameters:
//   - t: the test.
//   - exporter: the in-memory exporter to drain.
//   - provider: the provider to start the enclosing span from.
//   - standalone: true reproduces the pre-remediation sink, which started its
//     own SERVER span in addition to the enclosing one.
//
// Return values:
//   - spanPayload: the measured export volume for that request.
func runOneOTLPRequest(t *testing.T, exporter *tracetest.InMemoryExporter,
	provider *sdktrace.TracerProvider, standalone bool) spanPayload {
	t.Helper()

	exporter.Reset()
	row := newFinishedTraceRow(t, 200)
	timestamps, err := row.GetTraceTimestamps()
	if err != nil {
		t.Fatalf("parse timestamps: %+v", err)
	}

	// The enclosing span otelgin would have created for the request.
	ctx, span := provider.Tracer("otelgin-stand-in").Start(context.Background(),
		"POST /v1/chat/completions", oteltrace.WithSpanKind(oteltrace.SpanKindServer))

	if standalone {
		if err := emitStandaloneRequestSpan(ctx, row, timestamps); err != nil {
			t.Fatalf("emit standalone span: %+v", err)
		}
	} else {
		enrichActiveRequestSpan(span, row, timestamps)
	}
	span.End()

	return measureSpanPayload(exporter.GetSpans())
}

// TestMeasureOTLPSpansPerRequest reports spans and attribute/event payload per
// request, before and after the span-reuse change.
func TestMeasureOTLPSpansPerRequest(t *testing.T) {
	exporter, provider := installRecordingProvider(t)

	before := runOneOTLPRequest(t, exporter, provider, true)
	after := runOneOTLPRequest(t, exporter, provider, false)

	t.Logf("arm=%q spans_per_request=%d attributes=%d events=%d attribute_payload_bytes=%d",
		"before: sink starts its own SERVER span beside otelgin's",
		before.Spans, before.Attributes, before.Events, before.PayloadBytes)
	t.Logf("arm=%q spans_per_request=%d attributes=%d events=%d attribute_payload_bytes=%d",
		"after: sink enriches the enclosing otelgin span",
		after.Spans, after.Attributes, after.Events, after.PayloadBytes)
}
