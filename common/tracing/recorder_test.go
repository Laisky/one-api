package tracing

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
)

// TestRecorderMarksAreInMemoryAndOrdered verifies every lifecycle key lands in
// the accumulated document and that unknown keys are reported rather than
// silently dropped.
func TestRecorderMarksAreInMemoryAndOrdered(t *testing.T) {
	rec := NewRecorder("trace-1", "/v1/chat/completions", "POST", 42)

	require.True(t, rec.Mark(model.TimestampRequestForwarded))
	require.True(t, rec.Mark(model.TimestampFirstUpstreamResponse))
	require.True(t, rec.Mark(model.TimestampFirstClientResponse))
	require.True(t, rec.Mark(model.TimestampUpstreamCompleted))
	require.True(t, rec.Mark(model.TimestampRequestCompleted))
	require.False(t, rec.Mark("not_a_real_key"))

	in, _, ok := rec.Finish()
	require.True(t, ok)
	require.Equal(t, "trace-1", in.TraceId)
	require.Equal(t, "/v1/chat/completions", in.URL)
	require.Equal(t, "POST", in.Method)
	require.Equal(t, int64(42), in.BodySize)

	require.NotNil(t, in.Timestamps.RequestReceived)
	require.NotNil(t, in.Timestamps.RequestForwarded)
	require.NotNil(t, in.Timestamps.FirstUpstreamResponse)
	require.NotNil(t, in.Timestamps.FirstClientResponse)
	require.NotNil(t, in.Timestamps.UpstreamCompleted)
	require.NotNil(t, in.Timestamps.RequestCompleted)
}

// TestRecorderFinishIsSingleShot verifies one request can never produce two
// trace rows, which is what keeps a panicking handler or a double middleware
// registration from doubling trace volume.
func TestRecorderFinishIsSingleShot(t *testing.T) {
	rec := NewRecorder("trace-2", "/api/status", "GET", 0)

	_, _, ok := rec.Finish()
	require.True(t, ok)

	_, _, ok = rec.Finish()
	require.False(t, ok)
}

// TestRecorderConcurrentMarks verifies the recorder tolerates the concurrency a
// streaming relay produces: the response writer marks the first client byte
// while the adaptor marks upstream completion.
func TestRecorderConcurrentMarks(t *testing.T) {
	rec := NewRecorder("trace-3", "/v1/messages", "POST", 0)

	keys := []string{
		model.TimestampRequestForwarded,
		model.TimestampFirstUpstreamResponse,
		model.TimestampFirstClientResponse,
		model.TimestampUpstreamCompleted,
	}

	var wg sync.WaitGroup
	for range 8 {
		for _, key := range keys {
			wg.Add(1)
			go func(k string) {
				defer wg.Done()
				rec.Mark(k)
			}(key)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec.AppendExternalCall(model.TraceExternalCall{Source: "mcp", Tool: "web_search"})
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec.SetStatus(200)
		}()
	}
	wg.Wait()

	in, _, ok := rec.Finish()
	require.True(t, ok)
	require.Len(t, in.Timestamps.ExternalCalls, 8)
	require.Equal(t, 200, in.Status)
}

// TestRecorderSnapshotIsolatesLateMarks verifies the document handed to a sink
// does not alias the recorder's slice, so a straggling goroutine appending an
// external call after Finish cannot mutate a row already queued for writing.
func TestRecorderSnapshotIsolatesLateMarks(t *testing.T) {
	rec := NewRecorder("trace-4", "/v1/responses", "POST", 0)
	rec.AppendExternalCall(model.TraceExternalCall{Source: "mcp", Tool: "first"})

	in, _, ok := rec.Finish()
	require.True(t, ok)
	require.Len(t, in.Timestamps.ExternalCalls, 1)

	rec.AppendExternalCall(model.TraceExternalCall{Source: "mcp", Tool: "late"})
	require.Len(t, in.Timestamps.ExternalCalls, 1, "snapshot must not observe post-Finish appends")
}

// TestRecorderExternalCallDefaults verifies the recorder applies the same
// defaults the synchronous AppendTraceExternalCall path applied.
func TestRecorderExternalCallDefaults(t *testing.T) {
	rec := NewRecorder("trace-5", "/v1/responses", "POST", 0)
	rec.AppendExternalCall(model.TraceExternalCall{Tool: "web_search", StartedAt: 100, EndedAt: 220})

	in, _, ok := rec.Finish()
	require.True(t, ok)
	require.Len(t, in.Timestamps.ExternalCalls, 1)

	call := in.Timestamps.ExternalCalls[0]
	require.Equal(t, "external", call.Source, "missing source must default")
	require.Equal(t, int64(120), call.DurationMs, "duration must be derived from the interval")
	require.Equal(t, "external:100", call.Key, "missing key must be derived from source and start")
}

// TestRecorderDurationUsesCompletionMark verifies the sampler's duration input
// comes from the recorded completion instant rather than from wall clock drift.
func TestRecorderDurationUsesCompletionMark(t *testing.T) {
	rec := NewRecorder("trace-6", "/v1/chat/completions", "POST", 0)

	rec.mu.Lock()
	rec.createdAt = 1_000
	completed := int64(6_500)
	rec.timestamps.RequestCompleted = &completed
	rec.mu.Unlock()

	_, durationMs, ok := rec.Finish()
	require.True(t, ok)
	require.Equal(t, int64(5_500), durationMs)
}

// TestRecorderNilSafety verifies every method tolerates a nil receiver, which
// is what lets call sites skip a "is this request traced" branch.
func TestRecorderNilSafety(t *testing.T) {
	var rec *Recorder

	require.False(t, rec.Mark(model.TimestampRequestCompleted))
	require.NotPanics(t, func() { rec.AppendExternalCall(model.TraceExternalCall{}) })
	require.NotPanics(t, func() { rec.SetStatus(200) })
	require.NotPanics(t, func() { rec.ForceSample() })
	require.Equal(t, "", rec.TraceID())
	require.Equal(t, 0, rec.Status())
	require.Equal(t, int64(0), rec.DurationMs())
	require.False(t, rec.Forced())

	_, _, ok := rec.Finish()
	require.False(t, ok)
}

// BenchmarkRecorderRequestLifecycle measures the whole in-memory cost the trace
// pipeline adds to one request: allocate the recorder, apply the five lifecycle
// marks a relay emits, set the status, and snapshot for the sink. The
// pre-proposal path spent one INSERT plus five SELECT/UPDATE round trips on the
// same work.
func BenchmarkRecorderRequestLifecycle(b *testing.B) {
	b.ReportAllocs()
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
}

// BenchmarkNewTraceRow measures building the persistable row from a finished
// recorder, which is the only serialization work left on the request goroutine.
func BenchmarkNewTraceRow(b *testing.B) {
	rec := NewRecorder("bench-trace", "/v1/chat/completions?model=gpt-4.1", "POST", 1024)
	rec.Mark(model.TimestampRequestForwarded)
	rec.Mark(model.TimestampRequestCompleted)
	rec.SetStatus(200)
	in, _, ok := rec.Finish()
	if !ok {
		b.Fatal("Finish must succeed once")
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, _, err := model.NewTraceRow(in); err != nil {
			b.Fatal(err)
		}
	}
}
