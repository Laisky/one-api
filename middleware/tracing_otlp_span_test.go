package middleware

// Empirical verification of the OTLP request span (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 1 / W1, "OTLP":
// "Verify with an in-memory exporter and the actual middleware order, then reuse
// that span").
//
// These tests drive the REAL middleware order from main.go -- otelgin.Middleware
// enclosing middleware.TracingMiddleware -- against
// sdktrace/tracetest.NewInMemoryExporter, and measure what the enclosing span is
// actually doing when the trace sink runs.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/common/telemetry"
	"github.com/Laisky/one-api/common/tracing"
	"github.com/Laisky/one-api/model"
)

// spanLivenessProbe wraps a trace sink and records what the context span looked
// like at Submit time. It is the measurement instrument for the question the
// proposal asks: is the enclosing otelgin span still open when the trace
// pipeline hands a finished trace to a sink?
type spanLivenessProbe struct {
	tracing.TraceSink

	submits   int
	recording bool
	spanID    oteltrace.SpanID
}

// Submit implements tracing.TraceSink.Submit by measuring the context span and
// then delegating to the wrapped sink.
//
// Parameters:
//   - ctx: the context the trace pipeline passes to the sink.
//   - row: the finished trace row.
//
// Return values:
//   - error: whatever the wrapped sink returned.
func (p *spanLivenessProbe) Submit(ctx context.Context, row *model.Trace) error {
	span := oteltrace.SpanFromContext(ctx)
	p.submits++
	p.recording = span.IsRecording()
	p.spanID = span.SpanContext().SpanID()
	return p.TraceSink.Submit(ctx, row)
}

// installInMemoryTracerProvider makes the global tracer provider export into an
// in-memory exporter for one test.
//
// Parameters:
//   - t: the test, used to register cleanup.
//
// Return values:
//   - *tracetest.InMemoryExporter: the exporter holding every finished span.
func installInMemoryTracerProvider(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()

	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(exporter)),
	)

	prevProvider := otel.GetTracerProvider()
	prevPropagator := otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	// This test installs a real provider by hand instead of going through
	// telemetry.InitOpenTelemetry, so it must also state the fact that the
	// provider-installed flag records; tracing.InitSinks refuses to build an
	// OTLP sink against what would otherwise look like the global no-op provider.
	t.Cleanup(telemetry.SetProviderInitializedForTest(true))
	t.Cleanup(func() {
		otel.SetTracerProvider(prevProvider)
		otel.SetTextMapPropagator(prevPropagator)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = provider.Shutdown(ctx)
	})

	return exporter
}

// useOTLPTraceSink configures the process for TRACE_SINK=otlp with
// TRACE_WRITE_MODE=batched, which section 3.2 of the proposal makes the only
// valid combination for the OTLP sink.
//
// Parameters:
//   - t: the test, used to register cleanup.
//
// Return values:
//   - *spanLivenessProbe: the installed sink, wrapping the real OTLP sink.
func useOTLPTraceSink(t *testing.T) *spanLivenessProbe {
	t.Helper()

	prev := struct {
		sinks      []string
		writeMode  string
		sampleRate float64
		otelOn     bool
		otelURL    string
		prefixes   []string
	}{
		config.TraceSinks, config.TraceWriteMode, config.TraceSampleRate,
		config.OpenTelemetryEnabled, config.OpenTelemetryEndpoint, config.TraceExcludedPathPrefixes,
	}

	config.TraceSinks = []string{config.TraceSinkOTLP}
	config.TraceWriteMode = config.TraceWriteModeBatched
	config.TraceSampleRate = 1
	config.OpenTelemetryEnabled = true
	config.OpenTelemetryEndpoint = "http://127.0.0.1:4318"
	config.TraceExcludedPathPrefixes = nil

	require.NoError(t, tracing.InitSinks(context.Background()))

	probe := &spanLivenessProbe{TraceSink: tracing.Sink()}
	restoreSink := tracing.SetSinkForTest(probe)

	t.Cleanup(func() {
		restoreSink()
		config.TraceSinks, config.TraceWriteMode, config.TraceSampleRate = prev.sinks, prev.writeMode, prev.sampleRate
		config.OpenTelemetryEnabled, config.OpenTelemetryEndpoint = prev.otelOn, prev.otelURL
		config.TraceExcludedPathPrefixes = prev.prefixes
	})

	return probe
}

// newOtelginTracedEngine builds the engine exactly the way main.go builds it:
// otelgin.Middleware first, then the gin logger, then TracingMiddleware.
//
// Parameters:
//   - t: the test, used for helper bookkeeping.
//   - withOtelgin: whether to register otelgin, so a test can contrast a request
//     that has an enclosing span with one that has none.
//   - handler: the route handler for POST /v1/chat/completions.
//
// Return values:
//   - *gin.Engine: the configured engine.
func newOtelginTracedEngine(t *testing.T, withOtelgin bool, handler gin.HandlerFunc) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	if withOtelgin {
		engine.Use(otelgin.Middleware("one-api-test"))
	}
	engine.Use(gmw.NewLoggerMiddleware(
		gmw.WithLevel(glog.LevelError.String()),
		gmw.WithLogger(logger.Logger.Named("gin-test")),
	))
	engine.Use(TracingMiddleware())
	engine.POST("/v1/chat/completions", handler)
	return engine
}

// relayLifecycleHandler emits the same lifecycle marks a relay request emits.
//
// Parameters:
//   - status: the status the handler responds with.
//
// Return values:
//   - gin.HandlerFunc: the handler.
func relayLifecycleHandler(status int) gin.HandlerFunc {
	return func(c *gin.Context) {
		tracing.RecordTraceTimestamp(c, model.TimestampRequestForwarded)
		tracing.RecordTraceTimestamp(c, model.TimestampFirstUpstreamResponse)
		tracing.RecordTraceExternalCall(c, model.TraceExternalCall{Source: "mcp", Tool: "web_search"})
		tracing.RecordTraceTimestamp(c, model.TimestampUpstreamCompleted)
		c.String(status, "ok")
	}
}

// countEvents tallies how many times each event name appears on a span.
//
// Parameters:
//   - span: the finished span snapshot.
//
// Return values:
//   - map[string]int: event name to occurrence count.
func countEvents(span tracetest.SpanStub) map[string]int {
	counts := map[string]int{}
	for _, event := range span.Events {
		counts[event.Name]++
	}
	return counts
}

// attributeOf reads one attribute from a finished span.
//
// Parameters:
//   - span: the finished span snapshot.
//   - key: the attribute key.
//
// Return values:
//   - attribute.Value: the value, or an invalid value when absent.
func attributeOf(span tracetest.SpanStub, key string) attribute.Value {
	for _, kv := range span.Attributes {
		if string(kv.Key) == key {
			return kv.Value
		}
	}
	return attribute.Value{}
}

// TestOtelginSpanIsLiveWhenTraceSinkSubmits is the measurement the proposal asks
// for, and it refutes the comment the OTLP sink used to carry.
//
// The old comment claimed otelgin's span "has already ended by the time a
// request completes", and started a second SERVER span on that basis. main.go
// registers otelgin BEFORE TracingMiddleware, so otelgin's deferred End runs
// AFTER TracingMiddleware's deferred end-of-request hook. Measured here: at
// Submit time the context span is still RECORDING, and its span id is the id of
// the one span the exporter receives. Before the fix this test saw two nested
// SERVER spans ("one_api.request" parented by "POST /v1/chat/completions").
func TestOtelginSpanIsLiveWhenTraceSinkSubmits(t *testing.T) {
	exporter := installInMemoryTracerProvider(t)
	probe := useOTLPTraceSink(t)
	engine := newOtelginTracedEngine(t, true, relayLifecycleHandler(http.StatusOK))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?turnstile=secret-token", http.NoBody)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	require.Equal(t, 1, probe.submits)
	require.True(t, probe.recording,
		"MEASURED: the enclosing otelgin span is still recording when the sink runs")

	spans := exporter.GetSpans()
	require.Len(t, spans, 1, "one request must produce exactly one SERVER span")

	span := spans[0]
	require.Equal(t, oteltrace.SpanKindServer, span.SpanKind)
	require.Equal(t, probe.spanID, span.SpanContext.SpanID(),
		"the enriched span must be the very span the sink saw in its context")
	require.NotEqual(t, "one_api.request", span.Name,
		"the sink must reuse otelgin's span rather than start its own")

	events := countEvents(span)
	for _, name := range []string{
		model.TimestampRequestReceived,
		model.TimestampRequestForwarded,
		model.TimestampFirstUpstreamResponse,
		model.TimestampFirstClientResponse,
		model.TimestampUpstreamCompleted,
		model.TimestampRequestCompleted,
	} {
		require.Equal(t, 1, events[name], "lifecycle event %q must appear exactly once", name)
	}
	require.Equal(t, 1, events["one_api.external_call"],
		"the external-call timeline is only known at completion, so only the sink can add it")

	require.Equal(t, int64(http.StatusOK), attributeOf(span, "one_api.status").AsInt64())
	require.NotEmpty(t, attributeOf(span, "one_api.trace_id").AsString())
	require.NotContains(t, attributeOf(span, "one_api.url").AsString(), "secret-token",
		"the span must carry the sanitized URL")

	// otelgin owns the span's lifetime, so its start and end already bracket the
	// whole request; the sink must not have re-timestamped or ended it early.
	require.False(t, span.StartTime.IsZero())
	require.True(t, span.EndTime.After(span.StartTime))
}

// TestOtelginSpanCarriesRemoteParentAndErrorStatus verifies the reused span
// keeps the parentage the caller propagated, and that a failing request marks it
// as an error.
func TestOtelginSpanCarriesRemoteParentAndErrorStatus(t *testing.T) {
	exporter := installInMemoryTracerProvider(t)
	useOTLPTraceSink(t)
	engine := newOtelginTracedEngine(t, true, relayLifecycleHandler(http.StatusBadGateway))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", http.NoBody)
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadGateway, w.Code)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)

	span := spans[0]
	require.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", span.SpanContext.TraceID().String())
	require.Equal(t, "00f067aa0ba902b7", span.Parent.SpanID().String(),
		"the request span must stay a child of the propagated remote parent")
	require.Equal(t, codes.Error, span.Status.Code)
	require.Equal(t, int64(http.StatusBadGateway), attributeOf(span, "one_api.status").AsInt64())
}

// TestTraceSinkStartsItsOwnSpanWithoutEnclosingSpan documents the ONLY
// configuration in which the sink still starts a span: a request that never
// passed through otelgin, so there is no enclosing span to enrich. The span it
// starts must then carry the full lifecycle itself.
func TestTraceSinkStartsItsOwnSpanWithoutEnclosingSpan(t *testing.T) {
	exporter := installInMemoryTracerProvider(t)
	probe := useOTLPTraceSink(t)
	engine := newOtelginTracedEngine(t, false, relayLifecycleHandler(http.StatusOK))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", http.NoBody)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	require.False(t, probe.recording, "there is no enclosing span to reuse here")

	spans := exporter.GetSpans()
	require.Len(t, spans, 1, "still exactly one SERVER span per request")

	span := spans[0]
	require.Equal(t, "one_api.request", span.Name)
	require.Equal(t, oteltrace.SpanKindServer, span.SpanKind)
	require.False(t, span.Parent.IsValid(), "with no enclosing span it is a root span")

	events := countEvents(span)
	for _, name := range []string{
		model.TimestampRequestReceived,
		model.TimestampRequestForwarded,
		model.TimestampFirstUpstreamResponse,
		model.TimestampFirstClientResponse,
		model.TimestampUpstreamCompleted,
		model.TimestampRequestCompleted,
	} {
		require.Equal(t, 1, events[name], "lifecycle event %q must appear exactly once", name)
	}
	require.Equal(t, 1, events["one_api.external_call"])
}
