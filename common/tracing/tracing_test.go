package tracing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
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

func TestGetOpenTelemetryIDsFromContext(t *testing.T) {
	t.Parallel()
	tp := sdktrace.NewTracerProvider()
	tracer := tp.Tracer("test")

	ctx, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	require.Equal(t, span.SpanContext().TraceID().String(), GetOpenTelemetryTraceIDFromContext(ctx))
	require.Equal(t, span.SpanContext().SpanID().String(), GetOpenTelemetrySpanIDFromContext(ctx))
	require.Empty(t, GetOpenTelemetryTraceIDFromContext(context.Background()))
	require.Empty(t, GetOpenTelemetrySpanIDFromContext(context.Background()))

	zeroCtx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{}))
	require.Empty(t, GetOpenTelemetryTraceIDFromContext(zeroCtx))
	require.Empty(t, GetOpenTelemetrySpanIDFromContext(zeroCtx))
}

func TestGenAIClientSpanNameAndExcludedInputDetails(t *testing.T) {
	t.Parallel()

	require.Equal(t, "chat gpt-4.1", GenAIClientSpanName("chat", "gpt-4.1"))
	require.Equal(t, "chat", GenAIClientSpanName("chat", ""))
	require.Equal(t, "gpt-4.1", GenAIClientSpanName("", "gpt-4.1"))
	require.True(t, IsExcludedGenAIInputDetailAttribute("gen_ai.input.messages"))
	require.True(t, IsExcludedGenAIInputDetailAttribute("gen_ai.output.messages"))
	require.False(t, IsExcludedGenAIInputDetailAttribute(GenAIRequestModelAttr))
	require.False(t, IsExcludedGenAIInputDetailAttribute(OneAPIChannelIDAttr))
}
