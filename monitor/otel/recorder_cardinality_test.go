package otel

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// TestRelayRequestMetricCardinality verifies that RecordRelayRequest does not
// produce one metric series per (user_id, token_id) pair. With cumulative
// temporality every distinct attribute combination creates a PERMANENT
// aggregator that is never freed, so attaching high-cardinality user_id /
// token_id attributes to per-request counters leads to unbounded heap growth.
//
// This test installs a manual-reader MeterProvider as the global provider
// BEFORE constructing the recorder (NewOtelRecorder uses otel.Meter, i.e. the
// global provider), then records N requests that share every bounded attribute
// but use a DISTINCT user_id / token_id each time. After the fix the number of
// data points must stay small (bounded by the remaining cardinality), not grow
// with N.
func TestRelayRequestMetricCardinality(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(provider)
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
	})

	recorder, err := NewOtelRecorder()
	if err != nil {
		t.Fatalf("NewOtelRecorder: %v", err)
	}

	const n = 100
	start := time.Now()
	for i := 0; i < n; i++ {
		recorder.RecordRelayRequest(
			start,
			1,                       // channelId (constant)
			"openai",                // channelType (constant)
			"gpt-4o",                // model (constant)
			fmt.Sprintf("user-%d", i),  // userId (DISTINCT)
			"default",               // group (constant)
			fmt.Sprintf("token-%d", i), // tokenId (DISTINCT)
			"openai",                // apiFormat (constant)
			"chat",                  // apiType (constant)
			true,                    // success (constant)
			10, 20,                  // promptTokens, completionTokens
			1.5,                     // quotaUsed
		)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}

	count := countDataPoints(t, &rm, "one_api_relay_requests_total")
	t.Logf("one_api_relay_requests_total data points = %d (after %d distinct user/token requests)", count, n)

	// Only bounded attributes vary now (none, in this test: every bounded
	// attribute is constant), so there must be a single series. Allow a tiny
	// margin to keep the assertion robust, but it must NOT scale with n.
	if count > 4 {
		t.Fatalf("relay_requests_total produced %d data points for %d distinct user/token combinations; "+
			"high-cardinality labels (user_id/token_id) are causing unbounded series growth", count, n)
	}
}

// countDataPoints returns the number of aggregated data points recorded for the
// named metric across all scopes. It supports the Sum aggregation used by the
// relay request counter.
func countDataPoints(t *testing.T, rm *metricdata.ResourceMetrics, name string) int {
	t.Helper()
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			switch d := m.Data.(type) {
			case metricdata.Sum[int64]:
				return len(d.DataPoints)
			case metricdata.Sum[float64]:
				return len(d.DataPoints)
			default:
				t.Fatalf("metric %q has unexpected aggregation type %T", name, m.Data)
			}
		}
	}
	t.Fatalf("metric %q not found in collected metrics", name)
	return 0
}
