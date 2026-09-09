package tracing

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/metrics"
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

// withRecorderLimits installs the recorder memory and admission bounds for one
// test.
//
// Parameters:
//   - t: the test, used to register cleanup.
//   - maxRecordBytes: TRACE_MAX_RECORD_BYTES for the duration of the test.
//   - maxExternalCalls: TRACE_MAX_EXTERNAL_CALLS for the duration of the test.
//   - maxActive: TRACE_MAX_ACTIVE_RECORDERS; 0 means unlimited.
//
// Return values: none.
func withRecorderLimits(t *testing.T, maxRecordBytes, maxExternalCalls, maxActive int) {
	t.Helper()
	prevBytes, prevCalls, prevActive :=
		config.TraceMaxRecordBytes, config.TraceMaxExternalCalls, config.TraceMaxActiveRecorders
	config.TraceMaxRecordBytes, config.TraceMaxExternalCalls, config.TraceMaxActiveRecorders =
		maxRecordBytes, maxExternalCalls, maxActive

	prevActiveCount := activeRecorders.Swap(0)
	t.Cleanup(func() {
		config.TraceMaxRecordBytes, config.TraceMaxExternalCalls, config.TraceMaxActiveRecorders =
			prevBytes, prevCalls, prevActive
		activeRecorders.Store(prevActiveCount)
	})
}

// TestRecorderAdmissionBudgetIsEnforced is the active-request admission gate.
//
// The completed-record queue bounds only FINISHED traces, so nothing observed
// the ACTIVE side: at 10000 RPS with 60-second streaming lifetimes roughly
// 600000 requests hold a recorder at once. At the bound a request must run
// untraced rather than allocate, and its slot must come back on Finish.
func TestRecorderAdmissionBudgetIsEnforced(t *testing.T) {
	recorder := installCountingRecorder(t)
	withRecorderLimits(t, 1<<20, 1024, 2)

	first := NewRecorder("admit-1", "/v1/chat/completions", "POST", 0)
	require.NotNil(t, first)
	second := NewRecorder("admit-2", "/v1/chat/completions", "POST", 0)
	require.NotNil(t, second)
	require.Equal(t, int64(2), activeRecorderCount())

	active, limit := recorder.activeGauge()
	require.Equal(t, float64(2), active, "occupancy must be published")
	require.Equal(t, float64(2), limit, "the configured bound must be published")

	denied := NewRecorder("admit-3", "/v1/chat/completions", "POST", 0)
	require.Nil(t, denied, "admission must be denied at the bound")
	require.Equal(t, 1, recorder.count(metrics.TraceOutcomeDroppedActiveLimit))
	require.Equal(t, int64(2), activeRecorderCount())

	// The denied recorder is a nil-safe no-op, which is what lets the caller
	// skip a "was this request admitted" branch on every mark.
	require.NotPanics(t, func() {
		denied.Mark(model.TimestampRequestCompleted)
		denied.AppendExternalCall(model.TraceExternalCall{Source: "mcp"})
		denied.SetStatus(200)
	})
	_, _, ok := denied.Finish()
	require.False(t, ok)
	require.Equal(t, int64(2), activeRecorderCount(), "a denied recorder must not release a slot")

	_, _, ok = first.Finish()
	require.True(t, ok)
	require.Equal(t, int64(1), activeRecorderCount(), "Finish must release the slot")

	_, _, ok = first.Finish()
	require.False(t, ok)
	require.Equal(t, int64(1), activeRecorderCount(), "a second Finish must not release twice")

	third := NewRecorder("admit-4", "/v1/chat/completions", "POST", 0)
	require.NotNil(t, third, "a released slot must be reusable")

	_, _, ok = second.Finish()
	require.True(t, ok)
	_, _, ok = third.Finish()
	require.True(t, ok)
	require.Zero(t, activeRecorderCount())
}

// TestRecorderAdmissionUnlimitedByDefault verifies the standalone default, where
// trace coverage is expected to be complete: an unset bound must not start
// dropping traces on an upgrade.
func TestRecorderAdmissionUnlimitedByDefault(t *testing.T) {
	recorder := installCountingRecorder(t)
	withRecorderLimits(t, 1<<20, 1024, 0)

	const total = 64
	recorders := make([]*Recorder, 0, total)
	for i := range total {
		r := NewRecorder("unlimited-"+strconv.Itoa(i), "/v1/chat/completions", "POST", 0)
		require.NotNil(t, r)
		recorders = append(recorders, r)
	}
	require.Equal(t, int64(total), activeRecorderCount())
	require.Zero(t, recorder.count(metrics.TraceOutcomeDroppedActiveLimit))

	for _, r := range recorders {
		_, _, ok := r.Finish()
		require.True(t, ok)
	}
	require.Zero(t, activeRecorderCount())
}

// TestRecorderAdmissionIsConcurrencySafe verifies the admission counter never
// admits past the bound and never leaks a slot under concurrent requests.
func TestRecorderAdmissionIsConcurrencySafe(t *testing.T) {
	installCountingRecorder(t)
	const limit = 8
	withRecorderLimits(t, 1<<20, 1024, limit)

	var admitted atomic.Int64
	var wg sync.WaitGroup
	for i := range 64 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := NewRecorder("race-"+strconv.Itoa(i), "/v1/messages", "POST", 0)
			if r == nil {
				return
			}
			admitted.Add(1)
			require.LessOrEqual(t, activeRecorderCount(), int64(limit))
			r.Finish()
		}(i)
	}
	wg.Wait()

	require.Zero(t, activeRecorderCount(), "every admitted slot must be released")
	require.Positive(t, admitted.Load())
}

// TestRecorderBoundsExternalCallCount is the recorder-truncation gate for the
// entry count: a relay retrying across channels, or a tool loop, appended
// without any bound for the whole request lifetime.
func TestRecorderBoundsExternalCallCount(t *testing.T) {
	recorder := installCountingRecorder(t)
	withRecorderLimits(t, 1<<20, 4, 0)

	r := NewRecorder("truncate-calls", "/v1/responses", "POST", 0)
	require.NotNil(t, r)
	for i := range 200 {
		r.AppendExternalCall(model.TraceExternalCall{
			Source: "mcp",
			Tool:   "web_search_" + strconv.Itoa(i),
		})
	}

	in, _, ok := r.Finish()
	require.True(t, ok)
	require.Len(t, in.Timestamps.ExternalCalls, 4, "the entry count must be bounded")
	require.True(t, r.Truncated())
	require.Equal(t, 1, recorder.count(metrics.TraceOutcomeTruncated),
		"truncation is counted once per record, not once per discarded entry")
}

// TestRecorderBoundsRetainedBytes verifies the per-record byte bound, which is
// what makes the active-recorder memory model provable.
func TestRecorderBoundsRetainedBytes(t *testing.T) {
	recorder := installCountingRecorder(t)
	const maxBytes = 2048
	withRecorderLimits(t, maxBytes, 100000, 0)

	r := NewRecorder("truncate-bytes", "/v1/responses", "POST", 0)
	require.NotNil(t, r)
	for i := range 500 {
		r.AppendExternalCall(model.TraceExternalCall{
			Source:      "mcp",
			Tool:        "web_search_" + strconv.Itoa(i),
			ServerLabel: "server-" + strconv.Itoa(i),
		})
	}

	in, _, ok := r.Finish()
	require.True(t, ok)
	require.NotEmpty(t, in.Timestamps.ExternalCalls, "the bound truncates, it does not empty the trace")
	require.Less(t, len(in.Timestamps.ExternalCalls), 500)
	require.LessOrEqual(t, r.retainedBytes, int64(maxBytes))
	require.Equal(t, 1, recorder.count(metrics.TraceOutcomeTruncated))
}

// TestRecorderBoundsBaseFields verifies the byte limit applies before external
// calls are appended, including an oversized caller-provided trace id.
func TestRecorderBoundsBaseFields(t *testing.T) {
	recorder := installCountingRecorder(t)
	withRecorderLimits(t, 1024, 1024, 0)

	r := NewRecorder(strings.Repeat("trace-id-", 2048), strings.Repeat("/path", 4096),
		strings.Repeat("M", 128), 0)
	require.NotNil(t, r)
	require.LessOrEqual(t, r.retainedBytes, int64(config.TraceMaxRecordBytes))
	require.True(t, r.Truncated())
	require.Equal(t, 1, recorder.count(metrics.TraceOutcomeTruncated))

	_, _, ok := r.Finish()
	require.True(t, ok)
}

// TestRecorderClipsRetainedStrings verifies each retained string is bounded on
// its own, so one upstream-supplied tool name cannot dominate a record, and
// that the accounting carries counts rather than content.
func TestRecorderClipsRetainedStrings(t *testing.T) {
	recorder := installCountingRecorder(t)
	withRecorderLimits(t, 1<<20, 1024, 0)

	longURL := "/v1/chat/completions?q=" + strings.Repeat("a", 4*maxRecorderURLBytes)
	r := NewRecorder("clip-strings", longURL, "POST", 0)
	require.NotNil(t, r)
	require.Len(t, r.url, maxRecorderURLBytes, "the retained URL must be bounded")
	require.True(t, r.Truncated())

	r.AppendExternalCall(model.TraceExternalCall{
		Source:      strings.Repeat("s", 4*maxRecorderStringBytes),
		Tool:        strings.Repeat("t", 4*maxRecorderStringBytes),
		ServerLabel: strings.Repeat("l", 4*maxRecorderStringBytes),
		Key:         strings.Repeat("k", 4*maxRecorderStringBytes),
	})

	in, _, ok := r.Finish()
	require.True(t, ok)
	require.Len(t, in.Timestamps.ExternalCalls, 1)
	call := in.Timestamps.ExternalCalls[0]
	require.Len(t, call.Source, maxRecorderStringBytes)
	require.Len(t, call.Tool, maxRecorderStringBytes)
	require.Len(t, call.ServerLabel, maxRecorderStringBytes)
	require.Len(t, call.Key, maxRecorderStringBytes)
	require.Equal(t, 1, recorder.count(metrics.TraceOutcomeTruncated))
}

// TestRecorderClipsOnRuneBoundary verifies a clipped string stays valid UTF-8,
// so a truncated value cannot break JSON serialization or a text column.
func TestRecorderClipsOnRuneBoundary(t *testing.T) {
	clipped, cut := clipString(strings.Repeat("世", 4*maxRecorderStringBytes), maxRecorderStringBytes)
	require.True(t, cut)
	require.True(t, utf8.ValidString(clipped))
	require.LessOrEqual(t, len(clipped), maxRecorderStringBytes)

	kept, cut := clipString("short", maxRecorderStringBytes)
	require.False(t, cut)
	require.Equal(t, "short", kept)
}

// TestTruncatedRecorderStillPersistsAValidTrace verifies truncation degrades the
// DETAIL of a trace and nothing else: the record still reaches the database and
// still parses as a complete timestamp document.
func TestTruncatedRecorderStillPersistsAValidTrace(t *testing.T) {
	db := useIsolatedTraceDB(t)
	recorder := installCountingRecorder(t)
	withRecorderLimits(t, 4096, 3, 0)
	withSinkConfig(t, 100, 10, 1, int(time.Hour/time.Millisecond))

	r := NewRecorder("truncate-persist", "/v1/responses", "POST", 17)
	require.NotNil(t, r)
	require.True(t, r.Mark(model.TimestampRequestForwarded))
	require.True(t, r.Mark(model.TimestampRequestCompleted))
	r.SetStatus(200)
	for i := range 50 {
		r.AppendExternalCall(model.TraceExternalCall{Source: "mcp", Tool: "tool_" + strconv.Itoa(i)})
	}

	in, _, ok := r.Finish()
	require.True(t, ok)
	require.Len(t, in.Timestamps.ExternalCalls, 3)
	require.Equal(t, 1, recorder.count(metrics.TraceOutcomeTruncated))

	row, _, err := model.NewTraceRow(in)
	require.NoError(t, err)

	sink := newSQLSink(context.Background())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, sink.Close(ctx))
	})
	require.NoError(t, sink.Submit(context.Background(), row))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, sink.Flush(ctx))

	var stored model.Trace
	require.NoError(t, db.Where("trace_id = ?", "truncate-persist").First(&stored).Error)
	require.Equal(t, 200, stored.Status)

	timestamps, err := stored.GetTraceTimestamps()
	require.NoError(t, err, "a truncated record must still parse")
	require.NotNil(t, timestamps.RequestReceived)
	require.NotNil(t, timestamps.RequestCompleted)
	require.Len(t, timestamps.ExternalCalls, 3)
}
