package tracing

// Asynchronous batched SQL trace sink (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 1 / W1.3).
//
// At 10k requests/second the pre-proposal path issued ~120,000 trace statements
// per second. This sink turns the same workload into one multi-row INSERT per
// TRACE_BATCH_SIZE records: ~20 statements/second at the default batch size.
//
// Capacity policy: the queue is bounded and a full queue drops the NEWEST
// record. Traces are best-effort telemetry, and blocking a relay request on a
// saturated trace writer would convert a storage problem into an availability
// problem. Every drop is counted so the condition is visible rather than silent.
//
// Memory policy: a writer's flush-local batch is bounded by rows, by bytes and
// by the backend's per-statement parameter ceiling (see batch.go), so a Flush
// never materializes the whole queue in one temporary slice.
//
// Loss policy: trace loss is permitted but never reported as success. A write
// failure is accounted against the sink before its records leave the pending
// count, so Flush and Close report how many accepted records did not persist
// instead of equating "no pending work" with "everything was stored".

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/common/metrics"
	"github.com/Laisky/one-api/model"
)

// flushPollInterval is how often Flush re-checks the pending counter. Flush runs
// on shutdown and in tests, never on the request path, so polling is cheaper
// than the synchronization a precise wake-up would need.
const flushPollInterval = time.Millisecond

// sqlSink buffers finished traces and writes them in batches.
//
// The queue is never closed. Shutdown is signalled through the closed channel
// while an RWMutex keeps Submit and Close from racing, so a late Submit can
// never send on a closed channel.
type sqlSink struct {
	queue     chan *model.Trace
	batchSize int
	interval  time.Duration

	// maxBatchRows and maxBatchBytes bound what one writer accumulates before
	// it must write. maxBatchRows also bounds the rows per INSERT statement, so
	// a large TRACE_BATCH_SIZE cannot bind more parameters than the backend
	// accepts.
	maxBatchRows  int
	maxBatchBytes int64

	// mu guards closedFlag against concurrent Submit calls. Submit takes it for
	// reading (uncontended in steady state); Close takes it for writing once.
	mu         sync.RWMutex
	closedFlag bool

	closeOnce sync.Once
	closed    chan struct{}

	// workerCtx belongs to this sink rather than to request submission. Close
	// cancels it after its caller's deadline so no writer can outlive database
	// shutdown; queued records are still written during a normal close.
	workerCtx   context.Context
	stopWorkers context.CancelFunc

	// flushReqs carries an explicit "write what you have now" signal to each
	// writer, one buffered slot per writer.
	//
	// Without it, Flush could only WAIT for a write to happen on its own
	// schedule: a partially filled batch is written when the flush ticker fires,
	// so a Flush with a deadline shorter than TRACE_FLUSH_INTERVAL_MS would time
	// out even though the writers were healthy and idle. Shutdown flushes with
	// a deadline far shorter than the 1 s default ticker, and so do tests that
	// must observe an accepted trace without sleeping for a ticker period.
	//
	// The trace-lookup path in controller/tracing.go no longer flushes at all:
	// an untrusted caller could otherwise defeat batching on demand, so it
	// returns an explicit "not retained locally" availability contract instead.
	flushReqs []chan struct{}

	wg sync.WaitGroup

	// droppedSinceReport counts queue-full drops not yet reported to the log,
	// and lastDropReport is when the last report went out. Before this change a
	// database problem produced a per-request error line in the operator's log;
	// with a buffered writer the same problem is silent apart from a metric, so
	// the condition is surfaced at a bounded rate.
	droppedSinceReport atomic.Int64
	lastDropReport     atomic.Int64

	// pending counts records accepted but not yet written, so Flush can wait
	// for a real drain instead of sleeping.
	//
	// It is an atomic counter rather than a sync.WaitGroup on purpose: Submit
	// increments it concurrently with a Flush that may be waiting on zero, and
	// sync.WaitGroup explicitly forbids a positive Add that races a Wait when
	// the counter is zero.
	pending atomic.Int64

	// unpersisted counts accepted records a write attempt failed to store.
	//
	// It moves with pending under persistenceMu, so a Flush that observes zero
	// pending work cannot conclude success while a write failure is in flight.
	unpersisted atomic.Int64

	// persistenceMu makes pending and unpersisted one consistent state snapshot.
	// A failed row moves from pending to unpersisted while holding this lock, so
	// a deadline report cannot count it twice.
	persistenceMu sync.Mutex
}

// sqlSinkWorkerCount tracks live SQL writer goroutines. It is intentionally
// package-private so lifecycle tests can prove failed initialization and close
// do not leak workers.
var sqlSinkWorkerCount atomic.Int64

// unpersistedTracesError reports accepted trace records that were not stored.
//
// It is a distinct type so a caller -- or a test asserting the acceptance gate
// -- can read the counts instead of parsing a message, and it unwraps to the
// deadline error when a flush or shutdown ran out of time.
type unpersistedTracesError struct {
	// pending is how many records were still queued or in flight.
	pending int64
	// failed is how many records a write attempt rejected.
	failed int64
	// cause is the deadline error, when a deadline was what stopped the wait.
	cause error
}

// Error implements the error interface without exposing any record content.
//
// Parameters: none.
//
// Return values:
//   - string: a message carrying only counts.
func (e *unpersistedTracesError) Error() string {
	msg := "trace sink did not persist " + strconv.FormatInt(e.pending+e.failed, 10) +
		" accepted records (" + strconv.FormatInt(e.pending, 10) + " queued or in flight, " +
		strconv.FormatInt(e.failed, 10) + " rejected by the database)"
	if e.cause != nil {
		msg += ": " + e.cause.Error()
	}
	return msg
}

// Unwrap exposes the deadline error so errors.Is(err, context.DeadlineExceeded)
// keeps working for callers that distinguish a timeout from a write failure.
//
// Parameters: none.
//
// Return values:
//   - error: the wrapped cause, or nil when persistence failed without a
//     deadline being involved.
func (e *unpersistedTracesError) Unwrap() error { return e.cause }

// total reports how many accepted records the error accounts for.
//
// Parameters: none.
//
// Return values:
//   - int64: pending plus failed records.
func (e *unpersistedTracesError) total() int64 { return e.pending + e.failed }

// newSQLSink starts the writer goroutines for the batched SQL sink.
//
// Parameters:
//   - ctx: lifetime scope handed to each database write.
//
// Return values:
//   - TraceSink: the running sink.
func newSQLSink(ctx context.Context) TraceSink {
	if ctx == nil {
		ctx = context.Background()
	}
	workerCtx, stopWorkers := context.WithCancel(ctx)
	s := &sqlSink{
		queue:         make(chan *model.Trace, config.TraceQueueSize),
		batchSize:     config.TraceBatchSize,
		maxBatchRows:  min(config.TraceBatchSize, maxRowsPerStatement()),
		maxBatchBytes: int64(config.TraceBatchMaxBytes),
		interval:      time.Duration(config.TraceFlushIntervalMs) * time.Millisecond,
		closed:        make(chan struct{}),
		workerCtx:     workerCtx,
		stopWorkers:   stopWorkers,
	}

	for range config.TraceWriterCount {
		req := make(chan struct{}, 1)
		s.flushReqs = append(s.flushReqs, req)
		s.wg.Add(1)
		sqlSinkWorkerCount.Add(1)
		go s.run(workerCtx, req)
	}

	return s
}

// Submit implements TraceSink.Submit by enqueueing the row without blocking.
//
// Parameters:
//   - ctx: unused; the write happens on a writer goroutine with its own scope.
//   - row: the finished trace row.
//
// Return values:
//   - error: always nil; capacity and shutdown failures are recorded as drops
//     so a telemetry problem never becomes a relay problem.
func (s *sqlSink) Submit(_ context.Context, row *model.Trace) error {
	if row == nil {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closedFlag {
		recordSubmitOutcome(metrics.TraceOutcomeDroppedClosed)
		return nil
	}

	s.persistenceMu.Lock()
	s.pending.Add(1)
	s.persistenceMu.Unlock()
	select {
	case s.queue <- row:
		recordSubmitOutcome(metrics.TraceOutcomeQueued)
	default:
		s.persistenceMu.Lock()
		s.pending.Add(-1)
		s.persistenceMu.Unlock()
		recordSubmitOutcome(metrics.TraceOutcomeDroppedQueueFull)
		metrics.UpdateTraceQueue(len(s.queue), cap(s.queue))
		s.reportDrop()
	}
	return nil
}

// dropReportInterval bounds how often a saturated queue is reported, so an
// incident cannot turn into a log flood of its own.
const dropReportInterval = 30 * time.Second

// reportDrop emits a rate-limited warning that traces are being discarded.
//
// Parameters: none.
//
// Return values: none.
func (s *sqlSink) reportDrop() {
	count := s.droppedSinceReport.Add(1)

	now := time.Now().UnixNano()
	last := s.lastDropReport.Load()
	if last != 0 && now-last < int64(dropReportInterval) {
		return
	}
	if !s.lastDropReport.CompareAndSwap(last, now) {
		return
	}

	s.droppedSinceReport.Store(0)
	logger.Logger.Warn("trace writer queue is full, discarding traces",
		zap.Int64("dropped_since_last_report", count),
		zap.Int("queue_capacity", cap(s.queue)),
		zap.String("hint", "the database cannot keep pace; raise TRACE_QUEUE_SIZE or TRACE_WRITER_COUNT, "+
			"lower TRACE_SAMPLE_RATE, or investigate database latency"))
}

// Flush implements TraceSink.Flush by waiting for every accepted record to be
// persisted, or for ctx to expire.
//
// A nil return means the records this sink accepted are in the database. It is
// NOT "the pending counter reached zero": a failed write releases its records
// from the pending count too, and reporting that as success made a total
// database outage indistinguishable from a healthy flush.
//
// Parameters:
//   - ctx: deadline for the wait.
//
// Return values:
//   - error: an *unpersistedTracesError, wrapped, when records were rejected by
//     the database or when the deadline passed before the drain completed. It
//     unwraps to ctx.Err() in the deadline case.
func (s *sqlSink) Flush(ctx context.Context) error {
	if pending, _ := s.persistenceSnapshot(); pending == 0 {
		return s.reportUnpersisted("flush trace sink")
	}

	// Ask every writer to write what it is holding, rather than waiting for its
	// ticker.
	s.requestFlush()

	ticker := time.NewTicker(flushPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return s.reportDeadline("flush trace sink", ctx.Err())
		case <-ticker.C:
			if pending, _ := s.persistenceSnapshot(); pending == 0 {
				return s.reportUnpersisted("flush trace sink")
			}
			// Re-signal: each writer drains a BOUNDED batch per request, so a
			// deep queue needs more than one nudge to converge. The send is
			// non-blocking and a writer only observes it while idle.
			s.requestFlush()
		}
	}
}

// requestFlush asks every writer to write what it currently holds.
//
// Parameters: none.
//
// Return values: none.
func (s *sqlSink) requestFlush() {
	for _, req := range s.flushReqs {
		select {
		case req <- struct{}{}:
		default: // a flush is already pending for this writer
		}
	}
}

// reportUnpersisted reports every persistence failure accepted by this sink.
//
// The counter is deliberately not consumed: Flush is an observation API, and
// allowing the first observer to erase loss makes concurrent callers believe
// records persisted when they did not.
//
// Parameters:
//   - op: the operation name used to wrap the error.
//
// Return values:
//   - error: nil when every accepted record persisted; otherwise a wrapped
//     *unpersistedTracesError carrying the count.
func (s *sqlSink) reportUnpersisted(op string) error {
	_, failed := s.persistenceSnapshot()
	if failed == 0 {
		return nil
	}
	return errors.Wrap(&unpersistedTracesError{failed: failed}, op)
}

// reportDeadline builds the error for a wait that ran out of time.
//
// Parameters:
//   - op: the operation name used to wrap the error.
//   - cause: the deadline error to unwrap to.
//
// Return values:
//   - error: a wrapped *unpersistedTracesError counting both the records still
//     queued or in flight and those already rejected by the database.
func (s *sqlSink) reportDeadline(op string, cause error) error {
	pending, failed := s.persistenceSnapshot()
	return errors.Wrap(&unpersistedTracesError{
		pending: pending,
		failed:  failed,
		cause:   cause,
	}, op)
}

// Close implements TraceSink.Close: stop accepting, drain, and join writers
// while its deadline permits. A deadline returns unfinished work promptly after
// cancelling workers; callers must keep their database open until those workers
// have exited.
//
// A deadline that expires with work outstanding reports how many accepted
// records were not persisted, so a shutdown that lost traces says so instead of
// returning a bare timeout (W1, "Shutdown": deadlines report unfinished work).
//
// Parameters:
//   - ctx: deadline for the final drain.
//
// Return values:
//   - error: a wrapped *unpersistedTracesError when records were rejected by
//     the database or when the drain did not finish in time.
func (s *sqlSink) Close(ctx context.Context) error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closedFlag = true
		s.mu.Unlock()
		close(s.closed)
	})

	joined := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(joined)
	}()

	select {
	case <-joined:
		return s.reportUnpersisted("close trace sink")
	case <-ctx.Done():
		if s.stopWorkers != nil {
			s.stopWorkers()
		}
		// Drivers normally honor the cancelled worker context and join quickly.
		// A driver is nevertheless allowed to ignore cancellation, and waiting
		// without another bound here would convert a shutdown deadline into an
		// unbounded hang. The caller receives a typed unfinished-work error and
		// owns the decision not to close the database under a live writer.
		err := s.reportDeadline("close trace sink", ctx.Err())
		var unfinished *unpersistedTracesError
		if errors.As(err, &unfinished) {
			logger.Logger.Warn("trace sink shutdown deadline expired with unfinished work",
				zap.Int64("unpersisted_records", unfinished.total()),
				zap.Int64("queued_or_in_flight", unfinished.pending),
				zap.Int64("write_failed", unfinished.failed))
		}
		return err
	}
}

// run drains the queue, writing whenever the batch is full or the flush
// interval elapses, and performs a final drain on shutdown.
//
// Parameters:
//   - ctx: cancellation scope handed to each write.
//   - flushReq: signalled by Flush to write the current batch immediately.
//
// Return values: none.
func (s *sqlSink) run(ctx context.Context, flushReq <-chan struct{}) {
	defer s.wg.Done()
	defer sqlSinkWorkerCount.Add(-1)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	batch := newTraceBatch(s.rowsPerStatement(), s.batchBytesLimit())

	for {
		select {
		case row := <-s.queue:
			batch.add(row)
			if batch.full() {
				s.write(ctx, batch.rows)
				batch.reset()
			}
		case <-ticker.C:
			if batch.len() > 0 {
				s.write(ctx, batch.rows)
				batch.reset()
			}
			metrics.UpdateTraceQueue(len(s.queue), cap(s.queue))
		case <-flushReq:
			// Pull what is queued as well, so one Flush moves the sink forward
			// rather than writing only the rows this writer had accumulated.
			//
			// The fill is BOUNDED: draining the whole queue here is exactly the
			// unbounded temporary slice the proposal prohibits. Flush re-signals
			// on every poll tick, so a deep queue still converges.
			s.fill(batch)
			s.write(ctx, batch.rows)
			batch.reset()
		case <-s.closed:
			s.drain(ctx, batch)
			return
		case <-ctx.Done():
			// Close cancelled the worker after its drain deadline elapsed. Count
			// every row this writer owned as failed and let Close join it before the
			// database can close.
			s.abandon(batch.rows)
			batch.reset()
			s.abandonQueued()
			return
		}
	}
}

// rowsPerStatement returns the row bound for one batch and one INSERT.
//
// Parameters: none.
//
// Return values:
//   - int: the bound; always at least 1, even for a sink built by a test
//     without sizing.
func (s *sqlSink) rowsPerStatement() int {
	if s.maxBatchRows > 0 {
		return s.maxBatchRows
	}
	if s.batchSize > 0 {
		return min(s.batchSize, maxRowsPerStatement())
	}
	return 1
}

// batchBytesLimit returns the byte bound for one batch.
//
// Parameters: none.
//
// Return values:
//   - int64: the bound in bytes; always strictly positive.
func (s *sqlSink) batchBytesLimit() int64 {
	if s.maxBatchBytes > 0 {
		return s.maxBatchBytes
	}
	return int64(config.TraceBatchMaxBytes)
}

// fill moves queued rows into batch until the batch is full or the queue is
// empty, never blocking.
//
// Parameters:
//   - batch: the writer's current batch, extended in place.
//
// Return values: none.
func (s *sqlSink) fill(batch *traceBatch) {
	for !batch.full() {
		select {
		case row := <-s.queue:
			batch.add(row)
		default:
			return
		}
	}
}

// drain writes everything still queued once shutdown has been signalled.
//
// Submit is refused before s.closed is closed, so the queue observed here is
// final and this loop terminates.
//
// Parameters:
//   - ctx: cancellation scope handed to each write.
//   - batch: rows already accumulated by this writer.
//
// Return values: none.
func (s *sqlSink) drain(ctx context.Context, batch *traceBatch) {
	for {
		s.fill(batch)
		if batch.len() == 0 {
			return
		}
		s.write(ctx, batch.rows)
		batch.reset()

		if len(s.queue) == 0 {
			return
		}
	}
}

// write persists one batch and releases the pending counter for its rows.
//
// The order matters: a failure is charged to the unpersisted counter BEFORE the
// rows leave pending. Otherwise a Flush polling pending could observe zero work
// and report success for records the database had just rejected.
//
// Parameters:
//   - ctx: cancellation scope for the write.
//   - batch: rows to persist; an empty batch is a no-op.
//
// Return values: none; failures are logged, counted and accumulated for the
// next Flush or Close to report, never returned to the request path.
func (s *sqlSink) write(ctx context.Context, batch []*model.Trace) {
	if len(batch) == 0 {
		return
	}
	written, err := model.InsertTraces(ctx, batch, s.rowsPerStatement())
	if written > 0 {
		metrics.RecordTraceOutcome(metrics.TraceOutcomeWritten, written)
	}
	if err != nil {
		// Duplicate trace ids are reported by InsertTraces as skipped rather
		// than failed -- the row is already stored -- so failures are counted
		// only on an actual error.
		failed := len(batch) - written
		s.complete(len(batch), failed)
		metrics.RecordTraceOutcome(metrics.TraceOutcomeWriteFailed, failed)
		logger.Logger.Warn("failed to write trace batch",
			zap.Error(err),
			zap.Int("batch_size", len(batch)),
			zap.Int("written", written),
			zap.Int("failed", failed))
	}
	if err == nil {
		s.complete(len(batch), 0)
	}

}

// abandon marks records held by a cancelled writer as failed before it exits.
//
// Parameters:
//   - batch: the records the writer had already removed from the queue.
//
// Return values: none.
func (s *sqlSink) abandon(batch []*model.Trace) {
	if len(batch) == 0 {
		return
	}
	s.complete(len(batch), len(batch))
}

// abandonQueued accounts for queue entries a cancelled writer will never send
// to the database.
//
// Parameters: none.
//
// Return values: none.
func (s *sqlSink) abandonQueued() {
	for {
		select {
		case <-s.queue:
			s.complete(1, 1)
		default:
			return
		}
	}
}

// complete atomically moves accepted records out of pending and records any
// that did not persist.
//
// Parameters:
//   - accepted: records leaving the writer.
//   - failed: subset of accepted records that were not persisted.
//
// Return values: none.
func (s *sqlSink) complete(accepted, failed int) {
	if accepted <= 0 {
		return
	}
	s.persistenceMu.Lock()
	if failed > 0 {
		s.unpersisted.Add(int64(failed))
	}
	s.pending.Add(-int64(accepted))
	s.persistenceMu.Unlock()
}

// persistenceSnapshot returns a consistent pending and failed pair.
//
// Parameters: none.
//
// Return values:
//   - int64: accepted records still queued or in flight.
//   - int64: accepted records known not to have persisted.
func (s *sqlSink) persistenceSnapshot() (int64, int64) {
	s.persistenceMu.Lock()
	defer s.persistenceMu.Unlock()
	return s.pending.Load(), s.unpersisted.Load()
}
