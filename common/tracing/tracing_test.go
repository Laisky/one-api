package tracing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
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
