package telemetry

import (
	"context"
	"testing"

	"github.com/Laisky/one-api/common/metrics"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	apimetric "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// TestLogExportOutcomeMetricsKeepKnownLabels uses the actual production SDK
// metric view to catch dropped operational outcome labels before export.
func TestLogExportOutcomeMetricsKeepKnownLabels(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader), sdkmetric.WithView(newPrivateMetricView()))
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	counter, err := provider.Meter("one-api").Int64Counter("oneapi_app_log_export_records_total")
	require.NoError(t, err)
	want := map[string]int64{
		metrics.AppLogExportOutcomeDroppedEncoding:      2,
		metrics.AppLogExportOutcomeDroppedOperatorLimit: 3,
		metrics.AppLogExportOutcomeEmitted:              5,
	}
	for outcome, count := range want {
		counter.Add(context.Background(), count, apimetric.WithAttributes(attribute.String("outcome", outcome)))
	}
	var collected metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &collected))
	require.Len(t, collected.ScopeMetrics, 1)
	require.Len(t, collected.ScopeMetrics[0].Metrics, 1)
	sum, ok := collected.ScopeMetrics[0].Metrics[0].Data.(metricdata.Sum[int64])
	require.True(t, ok)
	got := make(map[string]int64, len(sum.DataPoints))
	for _, point := range sum.DataPoints {
		attrs := point.Attributes.ToSlice()
		require.Len(t, attrs, 1, "a known outcome must retain its label")
		require.Equal(t, attribute.Key("outcome"), attrs[0].Key)
		got[attrs[0].Value.AsString()] = point.Value
	}
	require.Equal(t, want, got)
	require.False(t, privateMetricAttribute(attribute.String("outcome", "synthetic-untrusted-outcome")))
}
