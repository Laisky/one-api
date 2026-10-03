package telemetry

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	metric "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// TestPrivateMetricViewPreservesCountsWithoutUserStrings exercises the real
// production aggregation view and proves distinct private labels merge safely.
func TestPrivateMetricViewPreservesCountsWithoutUserStrings(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader), sdkmetric.WithView(newPrivateMetricView()))
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	counter, err := provider.Meter("one-api").Int64Counter("one_api_relay_requests_total")
	require.NoError(t, err)
	for _, secret := range []string{"sentinel-first-prompt", "sentinel-second-key"} {
		counter.Add(sampledTraceCtx(), 1, metric.WithAttributes(
			attribute.String("model", secret), attribute.String("path", secret),
			attribute.String("channel_name", secret), attribute.String("identifier", secret),
			attribute.String("cookie", secret), attribute.String("group", secret),
			attribute.String("channel_id", "7"), attribute.String("status_code", "200"),
			attribute.String("method", "POST"), attribute.String("success", "true")))
	}
	var result metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &result))
	require.Len(t, result.ScopeMetrics, 1)
	require.Len(t, result.ScopeMetrics[0].Metrics, 1)
	sum, ok := result.ScopeMetrics[0].Metrics[0].Data.(metricdata.Sum[int64])
	require.True(t, ok)
	require.Len(t, sum.DataPoints, 1)
	require.Equal(t, int64(2), sum.DataPoints[0].Value)
	require.Empty(t, sum.DataPoints[0].Exemplars)
	require.ElementsMatch(t, []attribute.KeyValue{
		attribute.String("channel_id", "7"), attribute.String("status_code", "200"),
		attribute.String("method", "POST"), attribute.String("success", "true"),
	}, sum.DataPoints[0].Attributes.ToSlice())
	for _, kv := range []attribute.KeyValue{attribute.String("outcome", "sentinel-secret"),
		attribute.String("method", "sentinel-key"), attribute.String("status_code", "200-sentinel"),
		attribute.String("channel_id", "7-sentinel"), attribute.String("channel_id", "0007"),
		attribute.String("channel_type", "sentinel-provider-key"), attribute.Int("unknown", 1)} {
		require.False(t, privateMetricAttribute(kv), "field %s must fail closed", kv.Key)
	}
	for _, target := range []string{"logs", "traces", "app_log_files", "async_task_bindings"} {
		require.True(t, privateMetricAttribute(attribute.String("target", target)))
	}
}
