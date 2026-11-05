package telemetry

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	apimetric "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/Laisky/one-api/common/metrics"
)

// TestChannelHealthOutcomesSurviveExport reproduces a gap in PR #610: the health-check
// sweep counter was declared in monitor/otel but never registered at the privacy
// boundary, so the exporter dropped the whole series, and its outcome values were not
// an allowed vocabulary, so even a registered series would have lost the label. Every
// outcome must reach OTLP as its own series.
// Parameters: t is the test handle. Returns: no values.
func TestChannelHealthOutcomesSurviveExport(t *testing.T) {
	outcomes := []string{
		metrics.ChannelHealthCheckApplied,
		metrics.ChannelHealthCheckStatusChanged,
		metrics.ChannelHealthCheckTransitionRefused,
		metrics.ChannelHealthCheckOptedOut,
		metrics.ChannelHealthCheckNotApplicable,
		metrics.ChannelHealthCheckConfigChanged,
		metrics.ChannelHealthCheckApplyFailed,
		metrics.ChannelHealthCheckUnreadable,
	}
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader), sdkmetric.WithView(newPrivateMetricView()))
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	counter, err := provider.Meter("one-api").Int64Counter("oneapi_channel_health_checks_total")
	require.NoError(t, err)
	for _, outcome := range outcomes {
		counter.Add(context.Background(), 1, apimetric.WithAttributes(attribute.String("outcome", outcome)))
	}

	var original metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &original))
	capture := &metricPrivacyCapture{}
	require.NoError(t, (privacyMetricExporter{Exporter: capture}).Export(context.Background(), &original))

	exported := map[string]int64{}
	for _, scope := range capture.result.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != "oneapi_channel_health_checks_total" {
				continue
			}
			for _, point := range metric.Data.(metricdata.Sum[int64]).DataPoints {
				value, ok := point.Attributes.Value("outcome")
				require.True(t, ok, "every point must keep its outcome label")
				exported[value.AsString()] += point.Value
			}
		}
	}
	want := make(map[string]int64, len(outcomes))
	for _, outcome := range outcomes {
		want[outcome] = 1
	}
	require.Equal(t, want, exported)
}
