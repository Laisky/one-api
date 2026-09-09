package tracing

// MEASUREMENT (not correctness) for the W1 bounded flush-local batch of the SQL
// trace writer -- proposal docs/proposals/20260905_observability-data-tiering.md,
// Phase 1 / W1, "SQL batching": "Bound rows **and bytes**, database parameters
// and flush-local buffers; do not drain the entire queue into an unbounded
// temporary slice".
//
// Everything in this file reports numbers; the acceptance assertions live in
// sink_sql_bounds_test.go. The names are prefixed Measure so a reader can tell
// at a glance that a failure here means the harness broke, not the product.
//
// HOW THE "BEFORE" ARM IS RECONSTRUCTED
//
// The pre-remediation writer drained with takeQueued, which looped the entire
// queue into one slice with no ceiling of any kind:
//
//	func (s *sqlSink) takeQueued(batch []*model.Trace) []*model.Trace {
//		for {
//			select {
//			case row := <-s.queue:
//				batch = append(batch, row)
//			default:
//				return batch
//			}
//		}
//	}
//
// The shipped fill() is the same loop with a bound: `for !batch.full()`. Setting
// maxRows to the queue capacity and maxBytes to MaxInt64 makes full() never true
// before the queue empties, so the bounded code executes the identical drain.
// The "unbounded" arm below is therefore the real pre-remediation behavior
// running through the shipped code path, not a re-implementation of it.

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/model"
)

// measurementQueueSize is the scaled-profile TRACE_QUEUE_SIZE the proposal names
// when it describes "a 50000-record slice materialized in one writer".
const measurementQueueSize = 50000

// newMeasurementRow builds one realistically sized finished trace row.
//
// The shape matters: the flush-local buffer retains whatever the row retains, so
// a row carrying only a bare path would understate the footprint. This one has a
// query string, the five lifecycle timestamps a relay actually marks, and two
// external-call entries, which is an ordinary tool-using relay request.
//
// Parameters:
//   - tb: the test or benchmark, used to fail fast on a builder error.
//   - seq: a sequence number, so trace ids stay unique within a run.
//
// Return values:
//   - *model.Trace: the row as the sink would receive it.
func newMeasurementRow(tb testing.TB, seq int) *model.Trace {
	tb.Helper()

	base := time.Now().UnixMilli()
	mark := func(offset int64) *int64 {
		v := base + offset
		return &v
	}

	row, _, err := model.NewTraceRow(model.TraceRowInput{
		TraceId:  fmt.Sprintf("0af7651916cd43dd8448eb211c80319c%06d", seq),
		URL:      "/v1/chat/completions?model=gpt-4.1-mini&stream=true&user=measurement",
		Method:   "POST",
		BodySize: 4096,
		Status:   200,
		Timestamps: &model.TraceTimestamps{
			RequestReceived:       mark(0),
			RequestForwarded:      mark(3),
			FirstUpstreamResponse: mark(180),
			FirstClientResponse:   mark(182),
			UpstreamCompleted:     mark(1420),
			RequestCompleted:      mark(1425),
			ExternalCalls: []model.TraceExternalCall{
				{Key: "upstream:1", Source: "upstream", StartedAt: base + 3, EndedAt: base + 1420, DurationMs: 1417},
				{Key: "mcp:search", Source: "mcp", Tool: "web_search", ServerLabel: "search-server",
					StartedAt: base + 200, EndedAt: base + 640, DurationMs: 440},
			},
		},
		CreatedAt: base,
	})
	require.NoError(tb, err)
	return row
}

// measureHeapBytesPerRow measures what one queued trace row actually retains on
// the heap, so a row count can be converted into memory.
//
// It is a MEASUREMENT, not an estimate: the rows are built, the heap is settled
// with two collections, and the delta is divided by the row count. Go's
// allocator rounds to size classes, so the figure includes real allocator
// overhead, which is what a memory model needs.
//
// Parameters:
//   - tb: the test or benchmark.
//   - rows: how many rows to allocate; larger values dilute measurement noise.
//
// Return values:
//   - int64: measured heap bytes retained per row.
//   - int64: total heap bytes retained by all of them.
func measureHeapBytesPerRow(tb testing.TB, rows int) (int64, int64) {
	tb.Helper()

	settle := func() uint64 {
		runtime.GC()
		runtime.GC()
		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		return stats.HeapAlloc
	}

	before := settle()

	held := make([]*model.Trace, 0, rows)
	for i := range rows {
		held = append(held, newMeasurementRow(tb, i))
	}

	after := settle()
	// The slice header itself is subtracted so the figure is the ROW cost; the
	// buffer's own pointer array is reported separately by the caller.
	sliceBytes := int64(cap(held)) * int64(pointerSizeBytes)
	total := int64(after-before) - sliceBytes

	runtime.KeepAlive(held)
	if rows <= 0 || total <= 0 {
		return 0, 0
	}
	return total / int64(rows), total
}

// pointerSizeBytes is the size of one *model.Trace in a flush-local slice.
const pointerSizeBytes = 8

// drainArm describes one flush-local drain configuration.
type drainArm struct {
	// name identifies the arm in the report.
	name string
	// maxRows is the batch row ceiling.
	maxRows int
	// maxBytes is the batch byte ceiling.
	maxBytes int64
}

// TestMeasureFlushLocalDrainFootprint reports the peak flush-local buffer a
// single writer materializes from a saturated queue, unbounded versus bounded.
//
// This is the quantification the proposal asks for when it prohibits draining
// "the entire queue into an unbounded temporary slice".
func TestMeasureFlushLocalDrainFootprint(t *testing.T) {
	perRow, totalHeap := measureHeapBytesPerRow(t, measurementQueueSize)
	t.Logf("measured: %d bytes retained per queued trace row (%d rows, %d bytes total heap)",
		perRow, measurementQueueSize, totalHeap)

	arms := []drainArm{
		{
			name:     "unbounded (pre-remediation takeQueued: whole queue in one slice)",
			maxRows:  measurementQueueSize,
			maxBytes: math.MaxInt64,
		},
		{
			name:     "bounded default (TRACE_BATCH_SIZE=500, TRACE_BATCH_MAX_BYTES=8MiB)",
			maxRows:  min(500, maxRowsPerStatement()),
			maxBytes: 8 << 20,
		},
	}

	for _, arm := range arms {
		s := &sqlSink{
			queue:         make(chan *model.Trace, measurementQueueSize),
			batchSize:     measurementQueueSize,
			maxBatchRows:  arm.maxRows,
			maxBatchBytes: arm.maxBytes,
			interval:      time.Hour,
			closed:        make(chan struct{}),
		}
		for i := range measurementQueueSize {
			s.queue <- newMeasurementRow(t, i)
		}
		require.Len(t, s.queue, measurementQueueSize, "the measurement needs a saturated queue")

		batch := newTraceBatch(s.rowsPerStatement(), s.batchBytesLimit())
		start := time.Now()
		s.fill(batch)
		elapsed := time.Since(start)

		pinned := int64(batch.len()) * perRow
		sliceBytes := int64(cap(batch.rows)) * pointerSizeBytes

		t.Logf("arm=%q peak_rows=%d accounted_bytes=%d pinned_row_heap_bytes=%d "+
			"slice_bytes=%d left_queued=%d drain_wall=%s",
			arm.name, batch.len(), batch.bytes, pinned, sliceBytes, len(s.queue), elapsed)
	}
}

// TestMeasureStatementParameterCeiling reports the per-statement parameter bound
// and what an operator-raised TRACE_BATCH_SIZE did before it existed.
//
// The pre-remediation writer passed TRACE_BATCH_SIZE straight to
// model.InsertTraces as the rows-per-statement, so a large configured batch
// produced a statement the driver refuses to prepare. The bound converts that
// from a total write failure into a split.
func TestMeasureStatementParameterCeiling(t *testing.T) {
	db := useIsolatedTraceFileDB(t, nil)
	installCountingRecorder(t)

	// The bound reads common.UsingSQLite, which model.InitDB sets for the
	// primary handle -- the handle traces are written through. The measurement
	// database here is SQLite, so the flag has to say so or the harness would
	// measure the MySQL/PostgreSQL ceiling against a SQLite driver.
	prevSQLite := common.UsingSQLite.Load()
	common.UsingSQLite.Store(true)
	t.Cleanup(func() { common.UsingSQLite.Store(prevSQLite) })

	t.Logf("measured: maxRowsPerStatement()=%d rows on SQLite, %d rows on MySQL/PostgreSQL "+
		"(%d parameters charged per row; driver ceilings %d and %d)",
		model.MaxStatementParametersSQLite/traceRowParameterCount,
		model.MaxStatementParametersDefault/traceRowParameterCount,
		traceRowParameterCount,
		model.MaxStatementParametersSQLite, model.MaxStatementParametersDefault)

	const oversized = 5000
	rows := make([]*model.Trace, 0, oversized)
	for i := range oversized {
		rows = append(rows, newMeasurementRow(t, i))
	}

	// Pre-remediation arm: rows-per-statement is whatever TRACE_BATCH_SIZE said.
	written, err := model.InsertTraces(context.Background(), rows, oversized)
	t.Logf("arm=%q rows_per_statement=%d written=%d err=%v",
		"pre-remediation (TRACE_BATCH_SIZE used verbatim)", oversized, written, err)

	require.NoError(t, db.Exec("DELETE FROM traces").Error)

	// Post-remediation arm: the same submission, capped by the parameter ceiling.
	capped := min(oversized, maxRowsPerStatement())
	written, err = model.InsertTraces(context.Background(), rows, capped)
	t.Logf("arm=%q rows_per_statement=%d written=%d err=%v",
		"bounded (min(TRACE_BATCH_SIZE, parameter ceiling))", capped, written, err)
	require.NoError(t, err)
	require.Equal(t, oversized, written)
}
