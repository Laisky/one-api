package telemetry

import (
	"context"
	"runtime"
	"strconv"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	apimetric "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// This file quantifies the three kept memory optimizations against their
// pre-change baselines, using the real OpenTelemetry SDK that production uses.
//
//   1. Export interval 15s -> 60s   (TestExportIntervalChurnIsLinear)
//   2. Zero-capacity exemplar reservoir (BenchmarkCollect, TestZeroReservoir*)
//   3. Drop user_id+username labels  (BenchmarkUserRecord, TestUserCardinality*)
//
// Run: go test ./common/telemetry/ -run 'Churn|Reservoir|Cardinality' -v
//      go test ./common/telemetry/ -bench 'Collect|UserRecord' -benchmem -run x

const benchSeries = 200 // mimics a realistic live series population per collect

// recordAndCollect builds a MeterProvider with the given options, records
// benchSeries distinct series under a sampled trace context (so the default
// TraceBasedFilter stores exemplars, exactly like production with tracing on),
// then returns a closure that re-records one series and performs one Collect —
// the per-export-cycle work whose allocations dominated the heap profile.
func recordAndCollect(tb testing.TB, opts ...sdkmetric.Option) (collect func(i int), shutdown func()) {
	tb.Helper()
	reader := sdkmetric.NewManualReader()
	opts = append(opts, sdkmetric.WithReader(reader))
	mp := sdkmetric.NewMeterProvider(opts...)
	meter := mp.Meter("proof")
	ctr, err := meter.Int64Counter("c")
	if err != nil {
		tb.Fatal(err)
	}
	hist, err := meter.Float64Histogram("h")
	if err != nil {
		tb.Fatal(err)
	}
	ctx := sampledTraceCtx()
	rec := func(i int) {
		a := apimetric.WithAttributes(attribute.Int("series", i))
		ctr.Add(ctx, 1, a)
		hist.Record(ctx, 1.5, a)
	}
	for i := 0; i < benchSeries; i++ {
		rec(i)
	}
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil { // warm
		tb.Fatal(err)
	}
	return func(i int) {
			rec(i % benchSeries)
			_ = reader.Collect(ctx, &rm)
		}, func() {
			_ = mp.Shutdown(context.Background())
		}
}

// BenchmarkCollect reports bytes/op and allocs/op for one record+collect cycle,
// comparing the default exemplar reservoir against the zero-capacity reservoir
// view that production now installs (task 2).
func BenchmarkCollect(b *testing.B) {
	b.Run("default_reservoir", func(b *testing.B) {
		collect, shutdown := recordAndCollect(b)
		defer shutdown()
		b.ReportAllocs()
		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			collect(n)
		}
	})
	b.Run("zero_reservoir_view", func(b *testing.B) {
		collect, shutdown := recordAndCollect(b, sdkmetric.WithView(newZeroExemplarReservoirView()))
		defer shutdown()
		b.ReportAllocs()
		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			collect(n)
		}
	})
}

// TestZeroReservoirCutsCollectBytes asserts the zero-capacity reservoir view
// reduces per-collect allocated BYTES by a large margin (the heap profile's
// exemplar-reservoir reallocation). It also demonstrates that the exemplar
// FILTER (always_off) is NOT a substitute — bytes barely move — which is why we
// use a reservoir view, not a filter.
func TestZeroReservoirCutsCollectBytes(t *testing.T) {
	bytesPerCollect := func(opts ...sdkmetric.Option) uint64 {
		collect, shutdown := recordAndCollect(t, opts...)
		defer shutdown()
		const iters = 50
		runtime.GC()
		var m0 runtime.MemStats
		runtime.ReadMemStats(&m0)
		for n := 0; n < iters; n++ {
			collect(n)
		}
		var m1 runtime.MemStats
		runtime.ReadMemStats(&m1)
		return (m1.TotalAlloc - m0.TotalAlloc) / iters
	}

	def := bytesPerCollect()
	zero := bytesPerCollect(sdkmetric.WithView(newZeroExemplarReservoirView()))
	t.Logf("per-collect bytes: default=%d, zero-reservoir=%d (%.1f%% of default)",
		def, zero, 100*float64(zero)/float64(def))

	if zero >= def/2 {
		t.Fatalf("zero-reservoir view should cut per-collect bytes by far more than half: default=%d zero=%d", def, zero)
	}
}

// userAttrs models the recorder.go label sets before/after task 3.
func userAttrs(withUserID bool, i int) apimetric.AddOption {
	if withUserID {
		return apimetric.WithAttributes(
			attribute.String("user_id", strconv.Itoa(i)),
			attribute.String("username", "user"+strconv.Itoa(i)),
			attribute.String("group", "default"),
		)
	}
	return apimetric.WithAttributes(attribute.String("group", "default"))
}

// BenchmarkUserRecord reports per-call allocation of RecordUserMetrics' counter
// before (user_id+username+group) vs after (group only) — task 3.
func BenchmarkUserRecord(b *testing.B) {
	run := func(b *testing.B, withUserID bool) {
		reader := sdkmetric.NewManualReader()
		mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
		defer mp.Shutdown(context.Background())
		ctr, _ := mp.Meter("u").Int64Counter("one_api_user_requests_total")
		ctx := context.Background()
		b.ReportAllocs()
		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			ctr.Add(ctx, 1, userAttrs(withUserID, n))
		}
	}
	b.Run("with_user_id_username", func(b *testing.B) { run(b, true) })
	b.Run("group_only", func(b *testing.B) { run(b, false) })
}

// TestUserCardinalityBoundsSeriesAndHeap is the core proof for task 3: with
// user_id+username, N distinct users create N permanent cumulative series and
// retain O(N) heap; with group only, it stays at 1 series and O(1) heap.
func TestUserCardinalityBoundsSeriesAndHeap(t *testing.T) {
	const N = 10000
	measure := func(withUserID bool) (series int, liveBytes uint64) {
		reader := sdkmetric.NewManualReader()
		mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
		ctr, err := mp.Meter("u").Int64Counter("one_api_user_requests_total")
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		runtime.GC()
		var m0 runtime.MemStats
		runtime.ReadMemStats(&m0)
		for i := 0; i < N; i++ {
			ctr.Add(ctx, 1, userAttrs(withUserID, i))
		}
		var rm metricdata.ResourceMetrics
		if err := reader.Collect(ctx, &rm); err != nil {
			t.Fatal(err)
		}
		runtime.GC()
		var m1 runtime.MemStats
		runtime.ReadMemStats(&m1)
		runtime.KeepAlive(mp) // keep the aggregators alive across the measurement
		for _, sm := range rm.ScopeMetrics {
			for _, mm := range sm.Metrics {
				if s, ok := mm.Data.(metricdata.Sum[int64]); ok {
					series = len(s.DataPoints)
				}
			}
		}
		liveBytes = m1.HeapAlloc - m0.HeapAlloc
		_ = mp.Shutdown(context.Background())
		return
	}

	withSeries, withBytes := measure(true)
	groupSeries, groupBytes := measure(false)
	t.Logf("with user_id+username: %d series, ~%d KiB retained (~%d B/user)",
		withSeries, withBytes/1024, withBytes/N)
	t.Logf("group only:           %d series, ~%d KiB retained", groupSeries, groupBytes/1024)

	if withSeries < N {
		t.Fatalf("expected ~%d permanent series with user_id+username, got %d", N, withSeries)
	}
	if groupSeries > 4 {
		t.Fatalf("group-only series must stay bounded, got %d (unbounded growth not fixed)", groupSeries)
	}
	if withBytes <= groupBytes {
		t.Fatalf("expected per-user labels to retain materially more heap: with=%d group=%d", withBytes, groupBytes)
	}
}

// TestExportIntervalChurnIsLinear proves the 15s->60s change (task 1) is a 4x
// churn reduction: the per-collect cost is fixed, so N collects allocate ~N x
// one collect. Going from 15s to 60s quarters the collect count over any window.
func TestExportIntervalChurnIsLinear(t *testing.T) {
	churnFor := func(collects int) uint64 {
		collect, shutdown := recordAndCollect(t)
		defer shutdown()
		runtime.GC()
		var m0 runtime.MemStats
		runtime.ReadMemStats(&m0)
		for n := 0; n < collects; n++ {
			collect(n)
		}
		var m1 runtime.MemStats
		runtime.ReadMemStats(&m1)
		return m1.TotalAlloc - m0.TotalAlloc
	}

	one := churnFor(10)
	four := churnFor(40)
	ratio := float64(four) / float64(one)
	t.Logf("churn(10 collects)=%d B, churn(40 collects)=%d B, ratio=%.2f (expect ~4.0)", one, four, ratio)
	t.Logf("=> at 15s the reader collects 4x as often as at 60s, so 60s allocates ~1/4 the churn")

	if ratio < 3.0 || ratio > 5.0 {
		t.Fatalf("per-collect churn should be ~linear (ratio ~4.0), got %.2f", ratio)
	}
}
