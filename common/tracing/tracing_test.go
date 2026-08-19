package tracing

import (
	"context"
	"net/http/httptest"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	gutils "github.com/Laisky/go-utils/v6"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	relayctx "github.com/Laisky/one-api/common/relayctx"
)

func TestGetTraceIDFromContextPrefersOpenTelemetrySpan(t *testing.T) {
	t.Parallel()
	tp := sdktrace.NewTracerProvider()
	tracer := tp.Tracer("test")

	ctx, span := tracer.Start(context.Background(), "test-span")
	traceID := span.SpanContext().TraceID().String()
	span.End()

	require.NotEmpty(t, traceID)
	require.Equal(t, traceID, GetTraceIDFromContext(ctx))
}

func TestGenerateChatCompletionIDFromContextPrefersOpenTelemetryTraceID(t *testing.T) {
	t.Parallel()
	tp := sdktrace.NewTracerProvider()
	tracer := tp.Tracer("test")

	ctx, span := tracer.Start(context.Background(), "test-span")
	traceID := span.SpanContext().TraceID().String()
	span.End()

	require.NotEmpty(t, traceID)
	require.Equal(t, "chatcmpl-oneapi-"+traceID, GenerateChatCompletionIDFromContext(ctx))
}

// TestGetTraceIDFromContext_ReadsTracingKey verifies that the trace id snapshotted
// under gutils.TracingKey (the relayctx.Detach pattern) is resolved.
//
// This fails before the fix: GetTraceIDFromContext ignored gutils.TracingKey and a
// detached background context (no embedded gin, no OTel span) silently lost its trace
// id, returning "". It passes after the new middle branch reads gutils.TracingKey.
func TestGetTraceIDFromContext_ReadsTracingKey(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), gutils.TracingKey, "trace-xyz-123")
	require.Equal(t, "trace-xyz-123", GetTraceIDFromContext(ctx))
}

// TestGetTraceIDFromContext_RoundTripsDetach is an end-to-end test proving the
// detached context produced by relayctx.Detach carries the request trace id through
// GetTraceIDFromContext.
func TestGetTraceIDFromContext_RoundTripsDetach(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/x", nil)

	tid, err := gmw.TraceID(c)
	require.NoError(t, err)
	expected := tid.String()
	require.NotEmpty(t, expected)

	detached := relayctx.Detach(c)
	require.Equal(t, expected, GetTraceIDFromContext(detached))
}
