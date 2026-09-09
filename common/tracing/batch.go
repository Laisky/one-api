package tracing

// Bounded flush-local batches for the SQL trace writer (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 1 / W1,
// "SQL batching").
//
// TRACE_BATCH_SIZE bounded rows per INSERT but nothing bounded what a single
// drain materialized: a Flush pulled the ENTIRE queue into one temporary slice,
// so at TRACE_QUEUE_SIZE=50000 the writer built 50000 records -- every one of
// them holding its serialized timestamp document and URL -- before writing any
// of them. This file gives the writer a batch that is bounded by rows, by
// bytes, and by what a single INSERT statement may bind.

import (
	"github.com/Laisky/one-api/model"
)

const (
	// traceRowOverheadBytes is the fixed per-row cost charged against the byte
	// budget: the model.Trace struct itself, its six timestamp pointers, the
	// slice element, and the driver's per-row bookkeeping. It is deliberately
	// generous, because the budget exists to bound worst-case memory rather
	// than to predict allocator behavior precisely.
	traceRowOverheadBytes = 256

	// traceRowParameterCount is how many parameters one trace row binds in a
	// multi-row INSERT: fifteen persisted columns plus the auto-increment key,
	// rounded up so a future column cannot silently push a batch past a
	// database's per-statement parameter ceiling.
	traceRowParameterCount = 24
)

// maxRowsPerStatement returns how many trace rows one INSERT may carry without
// exceeding the backend's parameter ceiling.
//
// At the default TRACE_BATCH_SIZE this bound never binds; it exists so an
// operator raising TRACE_BATCH_SIZE cannot produce a statement the database
// refuses to prepare, which would fail every trace write instead of only
// slowing it down.
//
// The ceiling comes from the dialect of the handle that actually writes traces
// rather than from a process-global engine flag, so a deployment that moved the
// `traces` table onto a different database cannot silently get the ceiling of
// the primary one.
//
// Parameters: none.
//
// Return values:
//   - int: the maximum rows per statement; always at least 1.
func maxRowsPerStatement() int {
	return model.TraceMaxRowsPerStatement(traceRowParameterCount)
}

// estimateTraceRowBytes estimates the memory a queued trace row retains.
//
// The estimate reads only lengths of fields that are ALREADY serialized -- the
// timestamp document was marshalled when the row was built, and the URL was
// sanitized then too -- so bounding a batch never serializes a row twice.
//
// Parameters:
//   - row: the queued row; nil contributes nothing.
//
// Return values:
//   - int64: the estimated retained size in bytes.
func estimateTraceRowBytes(row *model.Trace) int64 {
	if row == nil {
		return 0
	}
	return int64(traceRowOverheadBytes +
		len(row.TraceId) +
		len(row.UUID) +
		len(row.URL) +
		len(row.Method) +
		len(row.Timestamps))
}

// traceBatch is one writer's flush-local buffer, bounded by rows and bytes.
//
// It is owned by a single writer goroutine and needs no synchronization.
type traceBatch struct {
	rows  []*model.Trace
	bytes int64

	maxRows  int
	maxBytes int64
}

// newTraceBatch builds an empty bounded batch.
//
// Parameters:
//   - maxRows: row ceiling; values below 1 become 1.
//   - maxBytes: byte ceiling; values below 1 become 1.
//
// Return values:
//   - *traceBatch: the empty batch, pre-allocated for maxRows entries.
func newTraceBatch(maxRows int, maxBytes int64) *traceBatch {
	if maxRows < 1 {
		maxRows = 1
	}
	if maxBytes < 1 {
		maxBytes = 1
	}
	return &traceBatch{
		rows:     make([]*model.Trace, 0, maxRows),
		maxRows:  maxRows,
		maxBytes: maxBytes,
	}
}

// add appends one row to the batch.
//
// A row is always accepted even when it alone exceeds the byte ceiling: the
// bound governs how much a writer accumulates before writing, and dropping an
// accepted record here would lose a trace the sink already promised to write.
//
// Parameters:
//   - row: the row to accumulate; nil is ignored.
//
// Return values: none.
func (b *traceBatch) add(row *model.Trace) {
	if row == nil {
		return
	}
	b.rows = append(b.rows, row)
	b.bytes += estimateTraceRowBytes(row)
}

// full reports whether the batch has reached either bound.
//
// Parameters: none.
//
// Return values:
//   - bool: true when the batch must be written before accepting more rows.
func (b *traceBatch) full() bool {
	return len(b.rows) >= b.maxRows || b.bytes >= b.maxBytes
}

// len reports how many rows the batch holds.
//
// Parameters: none.
//
// Return values:
//   - int: the row count.
func (b *traceBatch) len() int {
	return len(b.rows)
}

// reset empties the batch, keeping its allocated capacity for reuse and
// clearing the row pointers so a written batch cannot pin trace records.
//
// Parameters: none.
//
// Return values: none.
func (b *traceBatch) reset() {
	clear(b.rows)
	b.rows = b.rows[:0]
	b.bytes = 0
}
