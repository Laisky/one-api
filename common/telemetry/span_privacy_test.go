package telemetry

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// TestProductionSpanExporterSuppressesUntrustedText exercises the actual
// provider boundary with arbitrary names, attributes, events, links, resources,
// status descriptions, scope metadata, and propagated tracestate.
func TestProductionSpanExporterSuppressesUntrustedText(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	resource := sdkresource.NewSchemaless(
		attribute.String("service.name", "one-api"), attribute.String("host.name", "b1"),
		attribute.String("process.command_args", "sentinel-argv-key"),
		attribute.String("process.owner", "sentinel-owner"),
		attribute.String("unknown", "sentinel-resource-secret"))
	provider := newTracerProvider(exporter, resource)
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	state, err := oteltrace.ParseTraceState("vendor=sentinel-tracestate-secret")
	require.NoError(t, err)
	parent := oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
		TraceID: oteltrace.TraceID{1, 2, 3}, SpanID: oteltrace.SpanID{4, 5, 6}, TraceState: state,
		TraceFlags: oteltrace.FlagsSampled})
	ctx := oteltrace.ContextWithRemoteSpanContext(context.Background(), parent)
	_, span := provider.Tracer("sentinel-scope-secret").Start(ctx, "sentinel-name-secret",
		oteltrace.WithAttributes(attribute.String("url.path", "/sentinel-path-key"),
			attribute.String("one_api.url", "/?prompt=sentinel-prompt"),
			attribute.String("user_agent.original", "sentinel-cookie"),
			attribute.String("one_api.method", "POST"),
			attribute.Int("one_api.body_size", 123)),
		oteltrace.WithLinks(oteltrace.Link{SpanContext: parent,
			Attributes: []attribute.KeyValue{attribute.String("cookie", "sentinel-link-secret")}}))
	span.SetStatus(codes.Error, "sentinel-status-secret")
	span.RecordError(errors.New("sentinel-exception-secret"))
	span.AddEvent("sentinel-event-secret", oteltrace.WithAttributes(attribute.String("body", "sentinel-body")))
	span.AddEvent("first_upstream_response")
	span.AddEvent("one_api.external_call", oteltrace.WithAttributes(
		attribute.String("tool", "sentinel-tool-secret"), attribute.String("server_label", "sentinel-server-secret"),
		attribute.String("source", "mcp"), attribute.Int("server_id", 7),
		attribute.Int64("duration_ms", 10), attribute.Bool("is_error", true)))
	id := span.SpanContext()
	span.End()
	require.NoError(t, provider.ForceFlush(context.Background()))
	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	got := spans[0]
	require.Equal(t, "one-api request", got.Name)
	require.Equal(t, id.TraceID(), got.SpanContext.TraceID())
	require.Equal(t, id.SpanID(), got.SpanContext.SpanID())
	require.Empty(t, got.SpanContext.TraceState().String())
	require.Empty(t, got.Parent.TraceState().String())
	require.Equal(t, parent.SpanID(), got.Parent.SpanID())
	require.Equal(t, codes.Error, got.Status.Code)
	require.Empty(t, got.Status.Description)
	require.Equal(t, "github.com/Laisky/one-api", got.InstrumentationScope.Name)
	require.ElementsMatch(t, []attribute.KeyValue{attribute.String("one_api.method", "POST"), attribute.Int("one_api.body_size", 123)}, got.Attributes)
	require.Len(t, got.Events, 2)
	require.Equal(t, "first_upstream_response", got.Events[0].Name)
	require.ElementsMatch(t, []attribute.KeyValue{attribute.String("source", "mcp"), attribute.Int("server_id", 7), attribute.Int64("duration_ms", 10), attribute.Bool("is_error", true)}, got.Events[1].Attributes)
	require.Len(t, got.Links, 1)
	require.Empty(t, got.Links[0].Attributes)
	require.Empty(t, got.Links[0].SpanContext.TraceState().String())
	require.ElementsMatch(t, []attribute.KeyValue{attribute.String("service.name", "one-api"), attribute.String("host.name", "b1")}, got.Resource.Attributes())
	require.Empty(t, privateSpanAttributes([]attribute.KeyValue{
		attribute.Int("server_id", -1), attribute.Int64("server_id", 1<<40),
		attribute.Int("one_api.status", 999), attribute.Int("duration_ms", -1),
	}))
}
