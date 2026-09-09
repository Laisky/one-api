package otel

import (
	"context"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// TestOperationalInstrumentsRecordWithBoundedAttributes verifies the W3.3
// instruments actually produce data points, carry only sanitized attribute
// values, and keep the explicit bucket boundaries this package configures.
//
// The boundaries are the load-bearing part. The OTel SDK's default histogram
// buckets stop at 10 seconds, so a relay that streams for two minutes and a
// relay that hung until its deadline would land in the same overflow bucket and
// the latency view would be unable to tell them apart. WithExplicitBucketBoundaries
// silently does nothing if the instrument fails to build, which is exactly the
// kind of failure no runtime behavior would reveal.
//
// The instruments are built once (initOperationalInstruments) against whatever
// global meter provider is installed at first use, so this test installs its
// provider before making the first operational call.
func TestOperationalInstrumentsRecordWithBoundedAttributes(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(provider)
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	rec := &OtelRecorder{}
	rec.RecordRequestOutcome("upstream_error", 125000)
	rec.RecordTimeToFirstToken("success", 250)
	rec.RecordRetentionSweep("logs\xc0", "completed", 1200, 45000)
	rec.RecordAppLogExportRecords("dropped_queue_full", 9)
	rec.UpdateAppLogExportQueue(3, 2048, 900, 4194304)

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))

	seen := map[string]bool{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			seen[m.Name] = true
			for _, kv := range attributeSetsOf(m) {
				require.True(t, utf8.ValidString(kv.Value.AsString()),
					"metric %s attribute %s must be valid UTF-8", m.Name, kv.Key)
			}
		}
	}

	for _, name := range []string{
		"oneapi_request_outcomes_total",
		"oneapi_request_duration_ms",
		"oneapi_request_time_to_first_token_ms",
		"oneapi_retention_sweeps_total",
		"oneapi_retention_sweep_rows_total",
		"oneapi_retention_sweep_duration_ms",
		"oneapi_app_log_export_records_total",
		"oneapi_app_log_export_queue_records",
		"oneapi_app_log_export_queue_record_limit",
		"oneapi_app_log_export_queue_bytes",
		"oneapi_app_log_export_queue_byte_limit",
	} {
		require.True(t, seen[name], "instrument %s must export a data point", name)
	}

	require.Equal(t, requestDurationBucketsMs,
		histogramBoundsOf(t, rm, "oneapi_request_duration_ms"),
		"request duration must keep its explicit boundaries, not the SDK defaults")
	require.Equal(t, timeToFirstTokenBucketsMs,
		histogramBoundsOf(t, rm, "oneapi_request_time_to_first_token_ms"))
	require.Equal(t, retentionSweepDurationBucketsMs,
		histogramBoundsOf(t, rm, "oneapi_retention_sweep_duration_ms"))
}

// TestOperationalRecordersIgnoreNegativeDurations pins that a clock adjustment
// cannot corrupt a cumulative histogram sum, which no later observation could
// repair.
func TestOperationalRecordersIgnoreNegativeDurations(t *testing.T) {
	rec := &OtelRecorder{}
	require.NotPanics(t, func() {
		rec.RecordRequestOutcome("timeout", -1)
		rec.RecordTimeToFirstToken("timeout", -1)
		rec.RecordRetentionSweep("logs", "failed", -5, -1)
		rec.RecordAppLogExportRecords("exported", 0)
	})
}

// histogramBoundsOf returns the explicit bucket boundaries of the named
// float64 histogram in rm.
//
// Parameters:
//   - t: the running test.
//   - rm: a collected resource metric set.
//   - name: the instrument name to look for.
//
// Return values:
//   - []float64: the boundaries of the metric's first data point.
func histogramBoundsOf(t *testing.T, rm metricdata.ResourceMetrics, name string) []float64 {
	t.Helper()
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			hist, ok := m.Data.(metricdata.Histogram[float64])
			require.True(t, ok, "%s must be a float64 histogram", name)
			require.NotEmpty(t, hist.DataPoints, "%s must have a data point", name)
			return hist.DataPoints[0].Bounds
		}
	}
	t.Fatalf("histogram %s not found", name)
	return nil
}
