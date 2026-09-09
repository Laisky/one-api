package middleware

// End-to-end proof of application-log to trace correlation (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 3 / W3.2:
// "Test actual exported trace/span IDs from gmw.GetLogger(c) in the request
// path", and gate G3's "real trace/span correlation").
//
// The unit tests in common/logger/otelbridge prove the bridge carries a span
// context it is handed. That is not the claim W3.2 makes. The claim is about
// the REQUEST PATH: that a handler which calls gmw.GetLogger(c) -- with no
// awareness of OpenTelemetry, no ctx argument, and no edit at the call site --
// produces a log record a backend can join to that request's span.
//
// Nothing short of the real middleware order can demonstrate that, because
// every link in the chain can break it independently: otelgin has to have
// started the span before RequestId runs, RequestId has to bind the span
// context onto the base logger, identity.BindBase has to preserve it through
// its rebuild, the fork's zapcore has to carry a skip-typed field through
// Logger.With, and the SDK has to lift the span context out of the emit
// context. This test drives all of it.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel/attribute"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/logger/otelbridge"
)

// logRecordSink captures every log record the bridge exports.
type logRecordSink struct {
	mu      sync.Mutex
	records []sdklog.Record
}

// Export implements sdklog.Exporter by retaining a clone of each record.
//
// Parameters:
//   - ctx: unused.
//   - records: the batch to retain.
//
// Return values:
//   - error: always nil.
func (s *logRecordSink) Export(_ context.Context, records []sdklog.Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range records {
		s.records = append(s.records, records[i].Clone())
	}
	return nil
}

// Shutdown implements sdklog.Exporter as a no-op.
//
// Parameters:
//   - ctx: unused.
//
// Return values:
//   - error: always nil.
func (s *logRecordSink) Shutdown(context.Context) error { return nil }

// ForceFlush implements sdklog.Exporter as a no-op.
//
// Parameters:
//   - ctx: unused.
//
// Return values:
//   - error: always nil.
func (s *logRecordSink) ForceFlush(context.Context) error { return nil }

// find returns the single record whose body matches message.
//
// Parameters:
//   - t: the test handle.
//   - message: the log message to look for.
//
// Return values:
//   - sdklog.Record: the matching record.
func (s *logRecordSink) find(t *testing.T, message string) sdklog.Record {
	t.Helper()

	s.mu.Lock()
	defer s.mu.Unlock()
	var found []sdklog.Record
	for _, record := range s.records {
		if record.Body().AsString() == message {
			found = append(found, record)
		}
	}
	require.Len(t, found, 1, "expected exactly one %q record", message)
	return found[0]
}

// newBridgedLogger builds a glog logger whose core is teed to the OTLP bridge,
// exactly as common/logger/otlp_sink.go wires it in production.
//
// Parameters:
//   - t: the test handle; the provider is shut down on cleanup.
//
// Return values:
//   - glog.Logger: the logger to hand to the gin logger middleware.
//   - *logRecordSink: the sink holding the exported records.
func newBridgedLogger(t *testing.T) (glog.Logger, *logRecordSink) {
	t.Helper()

	sink := &logRecordSink{}
	provider := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(sink)))
	t.Cleanup(func() {
		require.NoError(t, provider.Shutdown(context.Background()))
	})

	holder := otelbridge.NewProviderHolder()
	require.NoError(t, holder.Install(provider))

	base, err := glog.NewConsoleWithName("correlation-test", glog.LevelInfo)
	require.NoError(t, err)

	bridged := base.WithOptions(zap.WrapCore(func(core zapcore.Core) zapcore.Core {
		return zapcore.NewTee(core,
			otelbridge.NewCore(holder, otelbridge.DefaultScopeName, zapcore.DebugLevel))
	}))
	return bridged, sink
}

// newCorrelationEngine builds the production middleware order with a bridged
// logger installed.
//
// Parameters:
//   - t: the test handle.
//   - withRequestID: whether middleware.RequestId (which binds the correlation
//     field) is registered.
//   - handler: the route handler.
//
// Return values:
//   - *gin.Engine: the configured engine.
//   - *logRecordSink: the sink holding the exported records.
func newCorrelationEngine(t *testing.T, withRequestID bool, handler gin.HandlerFunc) (*gin.Engine, *logRecordSink) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	// The trace pipeline must run in batched mode with a non-SQL sink: these
	// tests have no database, and the legacy synchronous path writes trace rows
	// while the request is in flight.
	useOTLPTraceSink(t)

	// RequestId binds the correlation field only when the bridge is configured,
	// and it reads that once when the middleware is constructed, so the flag has
	// to be set before the engine is built.
	previousBridge := config.AppLogOTLPEnabled
	config.AppLogOTLPEnabled = true
	t.Cleanup(func() { config.AppLogOTLPEnabled = previousBridge })

	bridged, sink := newBridgedLogger(t)

	engine := gin.New()
	engine.Use(otelgin.Middleware("one-api-test"))
	engine.Use(gmw.NewLoggerMiddleware(
		gmw.WithLevel(glog.LevelInfo.String()),
		gmw.WithLogger(bridged),
	))
	if withRequestID {
		engine.Use(RequestId())
	}
	engine.Use(TracingMiddleware())
	engine.POST("/v1/chat/completions", handler)
	return engine, sink
}

// TestRequestLoggerRecordsCarryTheRequestSpanIDs is the W3.2 acceptance test:
// an ordinary gmw.GetLogger(c) call inside a handler must produce an exported
// log record carrying the request span's trace and span ids.
//
// The assertion is against the record's PROTOCOL fields, not its attributes.
// Trace correlation in OTLP lives in the LogRecord's trace_id/span_id/flags
// fields; a bridge that wrote them as attributes would satisfy a naive test and
// join against nothing in Loki, Tempo or ClickHouse.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestRequestLoggerRecordsCarryTheRequestSpanIDs(t *testing.T) {
	exporter := installInMemoryTracerProvider(t)

	engine, sink := newCorrelationEngine(t, true, func(c *gin.Context) {
		gmw.GetLogger(c).Info("relay finished")
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", http.NoBody)
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	engine.ServeHTTP(httptest.NewRecorder(), req)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	span := spans[0]

	record := sink.find(t, "relay finished")
	require.Equal(t, span.SpanContext.TraceID(), record.TraceID(),
		"the exported log record must carry the request's trace id")
	require.Equal(t, span.SpanContext.SpanID(), record.SpanID(),
		"the exported log record must carry the request's span id")
	require.True(t, record.TraceFlags().IsSampled())

	// The remote parent's trace id is what a caller propagated, so a distributed
	// query joins the gateway's logs to the caller's trace.
	require.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", record.TraceID().String())
}

// TestRequestLoggerRecordsKeepIdentityFields proves correlation is additive:
// the fields the request logger already carried must still be exported as
// attributes.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestRequestLoggerRecordsKeepIdentityFields(t *testing.T) {
	installInMemoryTracerProvider(t)

	engine, sink := newCorrelationEngine(t, true, func(c *gin.Context) {
		gmw.GetLogger(c).Info("relay finished", zap.String("model", "gpt-5"))
		c.String(http.StatusOK, "ok")
	})

	engine.ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequest(http.MethodPost, "/v1/chat/completions", http.NoBody))

	record := sink.find(t, "relay finished")

	var keys []string
	record.WalkAttributes(func(kv attribute.KeyValue) bool {
		keys = append(keys, string(kv.Key))
		return true
	})
	joined := strings.Join(keys, ",")

	require.Contains(t, joined, "request_id", "the request id must survive as an attribute")
	require.Contains(t, joined, "model", "call-site fields must survive as attributes")
	require.NotContains(t, joined, otelbridge.SpanContextFieldKey,
		"the correlation field is consumed by the bridge, never emitted as an attribute")
}

// TestRecordsAreUncorrelatedWithoutTheBoundField is the mutation check for the
// test above: with the binding removed, the very same handler produces records
// with no trace id at all.
//
// Without this, TestRequestLoggerRecordsCarryTheRequestSpanIDs could pass for
// the wrong reason -- for example if some other layer happened to correlate --
// and the binding in RequestId could be deleted without a test noticing.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestRecordsAreUncorrelatedWithoutTheBoundField(t *testing.T) {
	installInMemoryTracerProvider(t)

	engine, sink := newCorrelationEngine(t, false, func(c *gin.Context) {
		gmw.GetLogger(c).Info("relay finished")
		c.String(http.StatusOK, "ok")
	})

	engine.ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequest(http.MethodPost, "/v1/chat/completions", http.NoBody))

	record := sink.find(t, "relay finished")
	require.False(t, record.TraceID().IsValid(),
		"correlation must come from the bound span context, not from anywhere else")
}

// TestCallSiteSpanContextOverridesTheBoundOne proves a detached goroutine can
// log on behalf of a different span by passing the field at the call site,
// which is what relayctx.Detach'd billing work needs.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestCallSiteSpanContextOverridesTheBoundOne(t *testing.T) {
	installInMemoryTracerProvider(t)

	other := oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
		TraceID:    oteltrace.TraceID{0x09, 0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01},
		SpanID:     oteltrace.SpanID{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
		TraceFlags: oteltrace.FlagsSampled,
	})

	engine, sink := newCorrelationEngine(t, true, func(c *gin.Context) {
		gmw.GetLogger(c).Info("detached work", otelbridge.SpanContextFieldFrom(other))
		c.String(http.StatusOK, "ok")
	})

	engine.ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequest(http.MethodPost, "/v1/chat/completions", http.NoBody))

	record := sink.find(t, "detached work")
	require.Equal(t, other.TraceID(), record.TraceID())
	require.Equal(t, other.SpanID(), record.SpanID())
}

// TestCorrelationFieldIsNotBoundWhenTheBridgeIsOff proves the gate in RequestId
// is real: a deployment that never enabled the bridge must not pay for the
// binding at all.
//
// The measurement that motivated the gate is +192 B/op and +1 alloc/op on the
// logger rebuild, on a path every request takes. A test that only checked the
// enabled case would let the gate be deleted without notice.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestCorrelationFieldIsNotBoundWhenTheBridgeIsOff(t *testing.T) {
	installInMemoryTracerProvider(t)
	useOTLPTraceSink(t)

	previousBridge := config.AppLogOTLPEnabled
	config.AppLogOTLPEnabled = false
	t.Cleanup(func() { config.AppLogOTLPEnabled = previousBridge })

	sink := &logRecordSink{}
	provider := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(sink)))
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })

	holder := otelbridge.NewProviderHolder()
	require.NoError(t, holder.Install(provider))

	base, err := glog.NewConsoleWithName("correlation-off-test", glog.LevelInfo)
	require.NoError(t, err)
	bridged := base.WithOptions(zap.WrapCore(func(core zapcore.Core) zapcore.Core {
		return zapcore.NewTee(core,
			otelbridge.NewCore(holder, otelbridge.DefaultScopeName, zapcore.DebugLevel))
	}))

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(otelgin.Middleware("one-api-test"))
	engine.Use(gmw.NewLoggerMiddleware(
		gmw.WithLevel(glog.LevelInfo.String()),
		gmw.WithLogger(bridged),
	))
	engine.Use(RequestId())
	engine.Use(TracingMiddleware())
	engine.POST("/v1/chat/completions", func(c *gin.Context) {
		gmw.GetLogger(c).Info("relay finished")
		c.String(http.StatusOK, "ok")
	})

	engine.ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequest(http.MethodPost, "/v1/chat/completions", http.NoBody))

	record := sink.find(t, "relay finished")
	require.False(t, record.TraceID().IsValid(),
		"with the bridge disabled RequestId must not bind the correlation field")
}
