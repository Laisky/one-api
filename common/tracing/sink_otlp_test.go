package tracing

// OTLP sink unit tests (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 1 / W1, "OTLP"
// and "Metrics").
//
// The end-to-end proof that the enclosing otelgin span is the one that gets
// enriched lives in middleware/tracing_otlp_span_test.go, because only there is
// the real middleware order available. These tests pin the sink's own contract:
// which span it touches, and which outcome it counts.

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
	noopotel "go.opentelemetry.io/otel/trace/noop"

	"github.com/Laisky/one-api/common/metrics"
	"github.com/Laisky/one-api/model"
)

// traceOutcomeSpy tallies trace-pipeline outcomes for tests in this package.
//
// It is deliberately not safe for concurrent use: every test that installs it
// drives the pipeline from the test goroutine.
type traceOutcomeSpy struct {
	*metrics.NoOpRecorder

	outcomes map[string]int
}

// RecordTraceRecord implements metrics.TracePipelineRecorder.
//
// Parameters:
//   - outcome: one of the metrics.TraceOutcome* constants.
//   - count: how many records the outcome applies to.
//
// Return values: none.
func (s *traceOutcomeSpy) RecordTraceRecord(outcome string, count int) {
	s.outcomes[outcome] += count
}

// UpdateTraceQueueDepth implements metrics.TracePipelineRecorder.
//
// Parameters:
//   - depth: buffered records.
//   - capacity: queue capacity.
//
// Return values: none.
func (s *traceOutcomeSpy) UpdateTraceQueueDepth(depth, capacity float64) {}

// installTraceOutcomeSpy swaps in an outcome-counting metrics recorder.
//
// Parameters:
//   - t: the test, used to register cleanup.
//
// Return values:
//   - *traceOutcomeSpy: the installed recorder.
func installTraceOutcomeSpy(t *testing.T) *traceOutcomeSpy {
	t.Helper()
	spy := &traceOutcomeSpy{NoOpRecorder: &metrics.NoOpRecorder{}, outcomes: map[string]int{}}
	prev := metrics.Recorder()
	metrics.SetRecorder(spy)
	t.Cleanup(func() { metrics.SetRecorder(prev) })
	return spy
}

// installRecordingProvider makes the global tracer provider export into memory.
//
// Parameters:
//   - t: the test, used to register cleanup.
//
// Return values:
//   - *tracetest.InMemoryExporter: the exporter holding finished spans.
//   - *sdktrace.TracerProvider: the installed provider.
func installRecordingProvider(t *testing.T) (*tracetest.InMemoryExporter, *sdktrace.TracerProvider) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(exporter)),
	)
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(prev)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = provider.Shutdown(ctx)
	})
	return exporter, provider
}

// newFinishedTraceRow builds a persistable row with a complete lifecycle.
//
// Parameters:
//   - t: the test, used to fail fast on serialization errors.
//   - status: the final HTTP status carried by the row.
//
// Return values:
//   - *model.Trace: the finished row.
func newFinishedTraceRow(t *testing.T, status int) *model.Trace {
	t.Helper()
	start := time.Now().Add(-2 * time.Second).UnixMilli()
	first := start + 100
	end := start + 1500

	row, _, err := model.NewTraceRow(model.TraceRowInput{
		TraceId:   "trace-otlp-1",
		URL:       "/v1/chat/completions",
		Method:    "POST",
		BodySize:  42,
		Status:    status,
		CreatedAt: start,
		Timestamps: &model.TraceTimestamps{
			RequestReceived:     &start,
			FirstClientResponse: &first,
			RequestCompleted:    &end,
			ExternalCalls: []model.TraceExternalCall{
				{Source: "mcp", Tool: "web_search", StartedAt: first, EndedAt: first + 10, DurationMs: 10},
			},
		},
	})
	require.NoError(t, err)
	return row
}

// TestOTLPSinkEnrichesLiveSpanWithoutEndingIt verifies the sink reuses a live
// request span: it must not create a second span, and it must not end the one it
// enriched, because that span's lifetime belongs to otelgin.
func TestOTLPSinkEnrichesLiveSpanWithoutEndingIt(t *testing.T) {
	exporter, provider := installRecordingProvider(t)
	spy := installTraceOutcomeSpy(t)

	ctx, enclosing := provider.Tracer("test").Start(context.Background(), "POST /v1/chat/completions",
		oteltrace.WithSpanKind(oteltrace.SpanKindServer))

	require.NoError(t, newOTLPSink().Submit(ctx, newFinishedTraceRow(t, 502)))

	require.Empty(t, exporter.GetSpans(), "the sink must not end the span it enriched")
	require.True(t, enclosing.IsRecording())
	require.Equal(t, 1, spy.outcomes[metrics.TraceOutcomeSpanRecorded])
	require.Zero(t, spy.outcomes[metrics.TraceOutcomeSpanRecordFailed])

	enclosing.End()

	spans := exporter.GetSpans()
	require.Len(t, spans, 1, "exactly one SERVER span per request")
	require.Equal(t, "POST /v1/chat/completions", spans[0].Name)
	require.Equal(t, codes.Error, spans[0].Status.Code)

	events := map[string]int{}
	for _, event := range spans[0].Events {
		events[event.Name]++
	}
	require.Equal(t, 1, events["one_api.external_call"])
	require.Zero(t, events[model.TimestampRequestReceived],
		"lifecycle events are added live by the recording hooks, never replayed here")
}

// TestOTLPSinkStartsSpanWhenNoneIsLive verifies the fallback path: with no
// enclosing span the sink emits the complete request span itself.
func TestOTLPSinkStartsSpanWhenNoneIsLive(t *testing.T) {
	exporter, _ := installRecordingProvider(t)
	spy := installTraceOutcomeSpy(t)

	require.NoError(t, newOTLPSink().Submit(context.Background(), newFinishedTraceRow(t, 200)))

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	require.Equal(t, otlpFallbackSpanName, spans[0].Name)
	require.Equal(t, oteltrace.SpanKindServer, spans[0].SpanKind)
	require.Equal(t, codes.Unset, spans[0].Status.Code)

	events := map[string]int{}
	for _, event := range spans[0].Events {
		events[event.Name]++
	}
	require.Equal(t, 1, events[model.TimestampRequestReceived])
	require.Equal(t, 1, events[model.TimestampFirstClientResponse])
	require.Equal(t, 1, events[model.TimestampRequestCompleted])
	require.Equal(t, 1, events["one_api.external_call"])
	require.Equal(t, 1, spy.outcomes[metrics.TraceOutcomeSpanRecorded])
}

// TestOTLPSinkCountsSpanRecordFailureSeparately verifies a provider that will
// not record is reported without claiming collector export, and never as a
// locally recorded span.
func TestOTLPSinkCountsExportFailureSeparately(t *testing.T) {
	spy := installTraceOutcomeSpy(t)

	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(noopotel.NewTracerProvider())
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	err := newOTLPSink().Submit(context.Background(), newFinishedTraceRow(t, 200))
	require.Error(t, err)
	require.Equal(t, 1, spy.outcomes[metrics.TraceOutcomeSpanRecordFailed])
	require.Zero(t, spy.outcomes[metrics.TraceOutcomeSpanRecorded])
}

// TestOTLPSinkNonRecordingParentIsNotRecoverable documents the limit the
// proposal calls out: when SDK head sampling drops the enclosing span, local
// error selection cannot bring it back. The child inherits the parent's
// nonrecording decision and the sink reports the failure instead of pretending
// the trace was exported.
func TestOTLPSinkNonRecordingParentIsNotRecoverable(t *testing.T) {
	spy := installTraceOutcomeSpy(t)

	provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.NeverSample()))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(prev)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = provider.Shutdown(ctx)
	})

	ctx, dropped := provider.Tracer("test").Start(context.Background(), "POST /v1/chat/completions")
	require.False(t, dropped.IsRecording())

	err := newOTLPSink().Submit(ctx, newFinishedTraceRow(t, 500))
	require.Error(t, err)
	require.Equal(t, 1, spy.outcomes[metrics.TraceOutcomeSpanRecordFailed])
	require.Zero(t, spy.outcomes[metrics.TraceOutcomeSpanRecorded])
}

// TestOTLPSinkIgnoresNilRow verifies the sink tolerates a nil row without
// counting an outcome for it.
func TestOTLPSinkIgnoresNilRow(t *testing.T) {
	spy := installTraceOutcomeSpy(t)
	require.NoError(t, newOTLPSink().Submit(context.Background(), nil))
	require.Empty(t, spy.outcomes)
}

// TestOTLPOutcomeNamesDescribeLocalSDKState verifies neither sink outcome
// claims asynchronous collector delivery, which the sink cannot observe.
func TestOTLPOutcomeNamesDescribeLocalSDKState(t *testing.T) {
	require.Equal(t, "span_recorded", metrics.TraceOutcomeSpanRecorded)
	require.Equal(t, "span_record_failed", metrics.TraceOutcomeSpanRecordFailed)
	require.Equal(t, metrics.TraceOutcomeSpanRecorded, metrics.TraceOutcomeExported)
	require.Equal(t, metrics.TraceOutcomeSpanRecordFailed, metrics.TraceOutcomeExportFailed)
}
