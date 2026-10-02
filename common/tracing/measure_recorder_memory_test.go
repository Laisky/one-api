package tracing

// MEASUREMENT (not correctness) for the W1 recorder memory bounds and
// active-recorder admission -- proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 1 / W1,
// "Recorder memory" and "Active requests":
//
//	"Bound per-record bytes, external calls/events and retained string lengths"
//	"At 10,000 RPS and 60 seconds average lifetime there are approximately
//	 600,000 active requests; multiply by bounded recorder bytes and include
//	 runtime/queue/batch overhead."
//
// This file measures the per-recorder figure that arithmetic needs. It does NOT
// run 600000 recorders and does not claim to; the projection is arithmetic over
// a measured per-record cost and is labelled as such in the report.
//
// The acceptance assertions for these bounds are in recorder_test.go.

import (
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
)

// recorderShape describes one in-flight request whose recorder is measured.
type recorderShape struct {
	// name identifies the shape in the report.
	name string
	// urlBytes is the length of the raw request URL the recorder is handed.
	urlBytes int
	// externalCalls is how many external-call entries the request appends.
	externalCalls int
	// stringBytes is the length of each string on an external-call entry.
	stringBytes int
}

// buildMeasuredRecorder constructs one recorder the way the middleware does and
// drives it to the state a long-lived streaming request holds it in.
//
// bounded=false reconstructs the pre-remediation recorder: the struct is built
// directly with the untrimmed URL (NewRecorder now clips it against a constant,
// not a setting, so configuration alone cannot restore the old behavior) and the
// caller has already raised the byte and call ceilings out of the way.
//
// Parameters:
//   - shape: the request shape to build.
//   - bounded: whether to go through the bounded NewRecorder entry point.
//
// Return values:
//   - *Recorder: the live recorder, still unfinished, as an in-flight request
//     would hold it.
func buildMeasuredRecorder(shape recorderShape, bounded bool) *Recorder {
	url := "/v1/chat/completions?" + strings.Repeat("a", max(shape.urlBytes-22, 0))

	var rec *Recorder
	if bounded {
		rec = NewRecorder("0af7651916cd43dd8448eb211c80319c", url, "POST", 4096)
	} else {
		// Pre-remediation shape: the whole URL retained for the request's life.
		now := int64(1)
		rec = &Recorder{traceID: "0af7651916cd43dd8448eb211c80319c", url: url, method: "POST", bodySize: 4096, createdAt: now}
		rec.timestamps.RequestReceived = &now
	}
	if rec == nil {
		return nil
	}

	rec.Mark(model.TimestampRequestForwarded)
	rec.Mark(model.TimestampFirstUpstreamResponse)
	rec.Mark(model.TimestampFirstClientResponse)

	label := strings.Repeat("t", shape.stringBytes)
	for i := 0; i < shape.externalCalls; i++ {
		rec.AppendExternalCall(model.TraceExternalCall{
			Key:         label,
			Source:      "mcp",
			Tool:        label,
			ServerLabel: label,
			StartedAt:   int64(i),
			EndedAt:     int64(i) + 1,
		})
	}
	return rec
}

// measureRecorderHeapBytes measures the heap a population of live recorders
// retains, divided by the population.
//
// Parameters:
//   - tb: the test or benchmark.
//   - count: how many recorders to hold live at once.
//   - shape: the request shape each one represents.
//   - bounded: whether the bounded entry point is used.
//
// Return values:
//   - int64: measured heap bytes retained per live recorder.
//   - int64: total heap bytes retained by the whole population.
func measureRecorderHeapBytes(tb testing.TB, count int, shape recorderShape, bounded bool) (int64, int64) {
	tb.Helper()

	settle := func() uint64 {
		runtime.GC()
		runtime.GC()
		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		return stats.HeapAlloc
	}

	held := make([]*Recorder, 0, count)
	before := settle()
	for range count {
		held = append(held, buildMeasuredRecorder(shape, bounded))
	}
	after := settle()

	total := int64(after-before) - int64(cap(held))*pointerSizeBytes
	runtime.KeepAlive(held)
	if count <= 0 || total <= 0 {
		return 0, 0
	}
	return total / int64(count), total
}

// measurementActiveRequests is the active-request population the proposal
// derives for 10000 RPS at a 60-second mean streaming lifetime. It is used only
// to render an arithmetic projection beside the measured per-recorder cost.
const measurementActiveRequests = 600000

// TestMeasureActiveRecorderMemory reports what one in-flight request's recorder
// retains, with and without the W1 bounds, for an ordinary request and for the
// pathological one the bounds exist for.
// requireMeasurementRun skips a measurement whose cost makes it unsuitable for
// the ordinary pre-merge suite.
//
// The repository convention is that long fixtures are opt-in (see
// ONEAPI_W24_PLANS in model/log_cursor_plan_test.go), so the scenario stays in
// the repository -- the proposal forbids ad-hoc one-off scripts -- without
// putting a multi-gigabyte allocation or a multi-second storm on every
// `go test ./...`.
//
// Parameters:
//   - t: the test to skip.
//
// Return values: none.
func requireMeasurementRun(t *testing.T) {
	t.Helper()
	if os.Getenv("ONEAPI_MEASURE") != "1" {
		t.Skip("set ONEAPI_MEASURE=1 to run this measurement")
	}
}

func TestMeasureActiveRecorderMemory(t *testing.T) {
	requireMeasurementRun(t)
	const population = 20000

	// The population is per shape, because the unbounded arms are genuinely
	// large: 20000 tool-loop recorders retain over 10 GB, which measures the
	// machine's swap rather than the recorder.
	shapes := []struct {
		recorderShape
		population int
	}{
		{recorderShape{name: "ordinary relay request (72 B URL, 2 external calls)",
			urlBytes: 72, externalCalls: 2, stringBytes: 12}, population},
		{recorderShape{name: "tool loop (72 B URL, 5000 external calls)",
			urlBytes: 72, externalCalls: 5000, stringBytes: 32}, 4000},
		{recorderShape{name: "hostile input (256 KiB URL, 5000 calls with 4 KiB strings)",
			urlBytes: 256 << 10, externalCalls: 5000, stringBytes: 4 << 10}, 2000},
	}

	profiles := []struct {
		name             string
		bounded          bool
		maxRecordBytes   int
		maxExternalCalls int
	}{
		{name: "unbounded (pre-remediation)", bounded: false, maxRecordBytes: 1 << 30, maxExternalCalls: 1 << 30},
		{name: "bounded standalone (TRACE_MAX_RECORD_BYTES=262144, TRACE_MAX_EXTERNAL_CALLS=1024)",
			bounded: true, maxRecordBytes: 262144, maxExternalCalls: 1024},
		{name: "bounded scaled (TRACE_MAX_RECORD_BYTES=65536, TRACE_MAX_EXTERNAL_CALLS=256)",
			bounded: true, maxRecordBytes: 65536, maxExternalCalls: 256},
	}

	for _, shape := range shapes {
		for _, profile := range profiles {
			count := shape.population

			withRecorderLimits(t, profile.maxRecordBytes, profile.maxExternalCalls, 0)
			perRecorder, total := measureRecorderHeapBytes(t, count, shape.recorderShape, profile.bounded)

			t.Logf("shape=%q profile=%q population=%d bytes_per_active_recorder=%d total_heap_bytes=%d "+
				"projected_bytes_at_%d_active=%d",
				shape.name, profile.name, count, perRecorder, total,
				measurementActiveRequests, perRecorder*measurementActiveRequests)
		}
	}
}

// TestMeasureAdmissionCeiling reports the memory ceiling admission converts an
// unbounded active set into, using the per-recorder cost measured above.
//
// The ceiling is arithmetic: TRACE_MAX_ACTIVE_RECORDERS multiplied by a measured
// worst-case per-recorder figure. Nothing here runs 200000 concurrent requests.
func TestMeasureAdmissionCeiling(t *testing.T) {
	requireMeasurementRun(t)
	withRecorderLimits(t, 65536, 256, 0)
	worst := recorderShape{name: "worst case at the scaled bound", urlBytes: 256 << 10,
		externalCalls: 5000, stringBytes: 4 << 10}
	perRecorder, _ := measureRecorderHeapBytes(t, 2000, worst, true)

	t.Logf("measured: worst-case bounded recorder = %d bytes", perRecorder)
	for _, limit := range []int64{0, 200000, 600000} {
		if limit == 0 {
			t.Logf("TRACE_MAX_ACTIVE_RECORDERS=0 (pre-remediation / opt-out): no ceiling, "+
				"memory is whatever concurrency the gateway reaches x %d bytes", perRecorder)
			continue
		}
		t.Logf("TRACE_MAX_ACTIVE_RECORDERS=%d: ceiling %d bytes (%.2f GiB) [ARITHMETIC, not measured]",
			limit, limit*perRecorder, float64(limit*perRecorder)/float64(1<<30))
	}
}

// BenchmarkMeasureRecorderLifecycle is the per-request cost of the whole
// in-memory trace pipeline with the W1 bounds engaged, in the arms that isolate
// what each bound costs.
//
// Under b.N iterations on one goroutine this is service time, not the
// reciprocal-throughput figure a RunParallel benchmark reports, and it is not a
// p99 of anything.
func BenchmarkMeasureRecorderLifecycle(b *testing.B) {
	arms := []struct {
		name             string
		maxActive        int
		maxRecordBytes   int
		maxExternalCalls int
	}{
		{name: "admission_unlimited", maxActive: 0, maxRecordBytes: 262144, maxExternalCalls: 1024},
		{name: "admission_bounded_200000", maxActive: 200000, maxRecordBytes: 262144, maxExternalCalls: 1024},
		{name: "admission_bounded_scaled", maxActive: 200000, maxRecordBytes: 65536, maxExternalCalls: 256},
	}

	for _, arm := range arms {
		b.Run(arm.name, func(b *testing.B) {
			prevBytes, prevCalls, prevActive :=
				config.TraceMaxRecordBytes, config.TraceMaxExternalCalls, config.TraceMaxActiveRecorders
			config.TraceMaxRecordBytes = arm.maxRecordBytes
			config.TraceMaxExternalCalls = arm.maxExternalCalls
			config.TraceMaxActiveRecorders = arm.maxActive
			prevCount := activeRecorders.Swap(0)
			b.Cleanup(func() {
				config.TraceMaxRecordBytes, config.TraceMaxExternalCalls, config.TraceMaxActiveRecorders =
					prevBytes, prevCalls, prevActive
				activeRecorders.Store(prevCount)
			})

			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				rec := NewRecorder("bench-trace", "/v1/chat/completions", "POST", 1024)
				rec.Mark(model.TimestampRequestForwarded)
				rec.Mark(model.TimestampFirstUpstreamResponse)
				rec.Mark(model.TimestampFirstClientResponse)
				rec.Mark(model.TimestampUpstreamCompleted)
				rec.Mark(model.TimestampRequestCompleted)
				rec.SetStatus(200)
				if _, _, ok := rec.Finish(); !ok {
					b.Fatal("Finish must succeed once")
				}
			}
		})
	}
}

// BenchmarkMeasureRecorderAdmissionParallel reports the contention cost of the
// admission counter, which is the one new hot-path synchronization point W1
// introduces on the request path.
//
// Under RunParallel ns/op is reciprocal throughput, not per-request latency.
func BenchmarkMeasureRecorderAdmissionParallel(b *testing.B) {
	for _, limit := range []int{0, 200000} {
		name := "unlimited"
		if limit > 0 {
			name = "bounded_200000"
		}
		b.Run(name, func(b *testing.B) {
			prevActive := config.TraceMaxActiveRecorders
			config.TraceMaxActiveRecorders = limit
			prevCount := activeRecorders.Swap(0)
			b.Cleanup(func() {
				config.TraceMaxActiveRecorders = prevActive
				activeRecorders.Store(prevCount)
			})

			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					if !admitRecorder() {
						b.Fatal("admission must not refuse below the bound")
					}
					releaseRecorder()
				}
			})
		})
	}
}

// TestMeasureClippedStringRetention measures whether the retained-string bound
// actually releases the memory it clips.
//
// clipString returns `s[:cut]`. A Go string slice shares the backing array of
// the original, so the clipped value's LENGTH is bounded while the memory it
// keeps reachable is not. This measurement varies only the size of the input URL
// -- every other input is fixed and every bound is at its shipped default -- so
// a retained figure that grows with the input is the retention the bound was
// supposed to remove.
func TestMeasureClippedStringRetention(t *testing.T) {
	requireMeasurementRun(t)
	withRecorderLimits(t, config.TraceMaxRecordBytes, config.TraceMaxExternalCalls, 0)

	settle := func() uint64 {
		runtime.GC()
		runtime.GC()
		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		return stats.HeapAlloc
	}

	const population = 2000
	for _, urlBytes := range []int{1 << 10, 8 << 10, 64 << 10, 256 << 10, 1 << 20} {
		held := make([]*Recorder, 0, population)
		before := settle()
		for range population {
			// The source string is built per recorder and goes out of scope
			// immediately, so anything still reachable is held by the recorder.
			url := "/v1/chat/completions?q=" + strings.Repeat("a", urlBytes-22)
			held = append(held, NewRecorder("0af7651916cd43dd8448eb211c80319c", url, "POST", 4096))
		}
		after := settle()
		perRecorder := (int64(after-before) - int64(cap(held))*pointerSizeBytes) / population
		runtime.KeepAlive(held)

		t.Logf("input_url_bytes=%d clipped_url_len=%d bytes_per_active_recorder=%d "+
			"(maxRecorderURLBytes=%d, TRACE_MAX_RECORD_BYTES=%d)",
			urlBytes, len(held[0].url), perRecorder, maxRecorderURLBytes, config.TraceMaxRecordBytes)
	}
}

// TestMeasureClippedExternalCallStringRetention applies the same test to the
// external-call strings, which clipString bounds the same way.
//
// The URL measurement above cannot answer this on its own: its external-call
// entries all share one Go string, so a shared backing array would be counted
// once no matter how many entries referenced it. Here every entry gets its own
// freshly allocated value, which is what an upstream payload produces.
func TestMeasureClippedExternalCallStringRetention(t *testing.T) {
	requireMeasurementRun(t)
	withRecorderLimits(t, config.TraceMaxRecordBytes, config.TraceMaxExternalCalls, 0)

	settle := func() uint64 {
		runtime.GC()
		runtime.GC()
		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		return stats.HeapAlloc
	}

	const (
		population = 1000
		calls      = 4
	)
	for _, stringBytes := range []int{64, 4 << 10, 16 << 10} {
		held := make([]*Recorder, 0, population)
		before := settle()
		for i := range population {
			rec := NewRecorder("0af7651916cd43dd8448eb211c80319c", "/v1/chat/completions", "POST", 4096)
			for c := range calls {
				// Freshly allocated per entry, as an upstream-derived value is.
				value := strings.Repeat("t", stringBytes-1) + string(rune('a'+(i+c)%26))
				rec.AppendExternalCall(model.TraceExternalCall{
					Key: value, Source: "mcp", Tool: value, ServerLabel: value,
					StartedAt: int64(c), EndedAt: int64(c) + 1,
				})
			}
			held = append(held, rec)
		}
		after := settle()
		perRecorder := (int64(after-before) - int64(cap(held))*pointerSizeBytes) / population

		accounted := int64(0)
		for _, call := range held[0].timestamps.ExternalCalls {
			accounted += externalCallBytes(call)
		}
		runtime.KeepAlive(held)

		t.Logf("input_string_bytes=%d calls=%d clipped_string_len=%d accounted_call_bytes=%d "+
			"bytes_per_active_recorder=%d (maxRecorderStringBytes=%d)",
			stringBytes, calls, len(held[0].timestamps.ExternalCalls[0].Tool), accounted,
			perRecorder, maxRecorderStringBytes)
	}
}
