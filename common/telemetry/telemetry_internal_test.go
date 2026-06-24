package telemetry

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	apimetric "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/trace"
)

// TestMetricExportInterval verifies the export interval defaults to 60s and
// honors the standard OTEL_METRIC_EXPORT_INTERVAL (milliseconds) override.
func TestMetricExportInterval(t *testing.T) {
	t.Run("default is 60s", func(t *testing.T) {
		t.Setenv("OTEL_METRIC_EXPORT_INTERVAL", "")
		if got := metricExportInterval(); got != 60*time.Second {
			t.Fatalf("default interval = %s, want 60s", got)
		}
	})
	t.Run("env override", func(t *testing.T) {
		t.Setenv("OTEL_METRIC_EXPORT_INTERVAL", "30000")
		if got := metricExportInterval(); got != 30*time.Second {
			t.Fatalf("interval = %s, want 30s", got)
		}
	})
	t.Run("invalid falls back to default", func(t *testing.T) {
		t.Setenv("OTEL_METRIC_EXPORT_INTERVAL", "not-a-number")
		if got := metricExportInterval(); got != defaultMetricExportInterval {
			t.Fatalf("interval = %s, want %s", got, defaultMetricExportInterval)
		}
	})
	t.Run("non-positive falls back to default", func(t *testing.T) {
		t.Setenv("OTEL_METRIC_EXPORT_INTERVAL", "0")
		if got := metricExportInterval(); got != defaultMetricExportInterval {
			t.Fatalf("interval = %s, want %s", got, defaultMetricExportInterval)
		}
	})
}

func sampledTraceCtx() context.Context {
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SpanID:     trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8},
		TraceFlags: trace.FlagsSampled, // makes the default TraceBasedFilter store exemplars
	})
	return trace.ContextWithSpanContext(context.Background(), sc)
}

// collectExemplarCount builds a ManualReader MeterProvider with the given
// options, records a counter and a histogram under a sampled trace context, and
// returns the total number of exemplars retained across all data points.
func collectExemplarCount(t *testing.T, opts ...sdkmetric.Option) int {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	opts = append(opts, sdkmetric.WithReader(reader))
	mp := sdkmetric.NewMeterProvider(opts...)
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	meter := mp.Meter("telemetry-test")
	ctr, err := meter.Int64Counter("test_counter")
	if err != nil {
		t.Fatalf("Int64Counter: %v", err)
	}
	hist, err := meter.Float64Histogram("test_hist")
	if err != nil {
		t.Fatalf("Float64Histogram: %v", err)
	}

	ctx := sampledTraceCtx()
	for i := 0; i < 10; i++ {
		a := apimetric.WithAttributes(attribute.Int("series", i))
		ctr.Add(ctx, 1, a)
		hist.Record(ctx, 1.5, a)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}

	total := 0
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			switch d := m.Data.(type) {
			case metricdata.Sum[int64]:
				for _, dp := range d.DataPoints {
					total += len(dp.Exemplars)
				}
			case metricdata.Sum[float64]:
				for _, dp := range d.DataPoints {
					total += len(dp.Exemplars)
				}
			case metricdata.Histogram[float64]:
				for _, dp := range d.DataPoints {
					total += len(dp.Exemplars)
				}
			}
		}
	}
	return total
}

// TestZeroExemplarReservoirViewSuppressesExemplars locks the fix: with the
// zero-capacity reservoir view, no exemplars are retained even under a sampled
// trace context, whereas the default configuration does retain them. This is
// the per-collect allocation that dominated the heap profile.
func TestZeroExemplarReservoirViewSuppressesExemplars(t *testing.T) {
	withView := collectExemplarCount(t, sdkmetric.WithView(newZeroExemplarReservoirView()))
	if withView != 0 {
		t.Fatalf("zero-capacity reservoir view retained %d exemplars, want 0", withView)
	}

	// Sanity contrast: the default configuration DOES retain exemplars under the
	// same sampled context, proving the view is what suppresses them.
	baseline := collectExemplarCount(t)
	if baseline == 0 {
		t.Fatalf("baseline retained 0 exemplars; the contrast assertion is meaningless " +
			"(expected the default TraceBasedFilter to store exemplars under a sampled context)")
	}
	t.Logf("exemplars retained: default=%d, zero-reservoir-view=%d", baseline, withView)
}
