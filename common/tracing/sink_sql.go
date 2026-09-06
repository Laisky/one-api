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

import (
	"context"
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

	// mu guards closedFlag against concurrent Submit calls. Submit takes it for
	// reading (uncontended in steady state); Close takes it for writing once.
	mu         sync.RWMutex
	closedFlag bool

	closeOnce sync.Once
	closed    chan struct{}

	// flushReqs carries an explicit "write what you have now" signal to each
	// writer, one buffered slot per writer.
	//
	// Without it, Flush could only WAIT for a write to happen on its own
	// schedule: a partially filled batch is written when the flush ticker fires,
	// so a Flush with a deadline shorter than TRACE_FLUSH_INTERVAL_MS would time
	// out even though the writers were healthy and idle. The trace-lookup path
	// in controller/tracing.go flushes with a 500 ms deadline, which the 1 s
	// default ticker would routinely miss.
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
}

// newSQLSink starts the writer goroutines for the batched SQL sink.
//
// Parameters:
//   - ctx: lifetime scope handed to each database write.
//
// Return values:
//   - TraceSink: the running sink.
func newSQLSink(ctx context.Context) TraceSink {
	s := &sqlSink{
		queue:     make(chan *model.Trace, config.TraceQueueSize),
		batchSize: config.TraceBatchSize,
		interval:  time.Duration(config.TraceFlushIntervalMs) * time.Millisecond,
		closed:    make(chan struct{}),
	}

	for range config.TraceWriterCount {
		req := make(chan struct{}, 1)
		s.flushReqs = append(s.flushReqs, req)
		s.wg.Add(1)
		go s.run(ctx, req)
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

	s.pending.Add(1)
	select {
	case s.queue <- row:
		recordSubmitOutcome(metrics.TraceOutcomeQueued)
	default:
		s.pending.Add(-1)
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
// written or for ctx to expire.
//
// Parameters:
//   - ctx: deadline for the wait.
//
// Return values:
//   - error: wrapped ctx.Err() when the deadline passed before the drain.
func (s *sqlSink) Flush(ctx context.Context) error {
	if s.pending.Load() == 0 {
		return nil
	}

	// Ask every writer to write what it is holding, rather than waiting for its
	// ticker.
	for _, req := range s.flushReqs {
		select {
		case req <- struct{}{}:
		default: // a flush is already pending for this writer
		}
	}

	ticker := time.NewTicker(flushPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return errors.Wrap(ctx.Err(), "flush trace sink")
		case <-ticker.C:
			if s.pending.Load() == 0 {
				return nil
			}
		}
	}
}

// Close implements TraceSink.Close: stop accepting, drain, and join writers.
//
// Parameters:
//   - ctx: deadline for the final drain.
//
// Return values:
//   - error: wrapped failure when the drain did not complete in time.
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
		return nil
	case <-ctx.Done():
		return errors.Wrap(ctx.Err(), "close trace sink")
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

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	batch := make([]*model.Trace, 0, s.batchSize)

	for {
		select {
		case row := <-s.queue:
			batch = append(batch, row)
			if len(batch) >= s.batchSize {
				s.write(ctx, batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				s.write(ctx, batch)
				batch = batch[:0]
			}
			metrics.UpdateTraceQueue(len(s.queue), cap(s.queue))
		case <-flushReq:
			// Pull everything queued as well, so one Flush drains the sink
			// rather than only the batch this writer had already accumulated.
			batch = s.takeQueued(batch)
			s.write(ctx, batch)
			batch = batch[:0]
		case <-s.closed:
			s.drain(ctx, batch)
			return
		}
	}
}

// takeQueued moves everything currently in the queue into batch without
// blocking.
//
// Parameters:
//   - batch: the writer's current batch.
//
// Return values:
//   - []*model.Trace: batch extended with whatever was queued.
func (s *sqlSink) takeQueued(batch []*model.Trace) []*model.Trace {
	for {
		select {
		case row := <-s.queue:
			batch = append(batch, row)
		default:
			return batch
		}
	}
}

// drain writes everything still queued once shutdown has been signalled.
//
// Parameters:
//   - ctx: cancellation scope handed to each write.
//   - batch: rows already accumulated by this writer.
//
// Return values: none.
func (s *sqlSink) drain(ctx context.Context, batch []*model.Trace) {
	for {
		select {
		case row := <-s.queue:
			batch = append(batch, row)
			if len(batch) >= s.batchSize {
				s.write(ctx, batch)
				batch = batch[:0]
			}
		default:
			s.write(ctx, batch)
			return
		}
	}
}

// write persists one batch and releases the pending counter for its rows.
//
// Parameters:
//   - ctx: cancellation scope for the write.
//   - batch: rows to persist; an empty batch is a no-op.
//
// Return values: none; failures are logged and counted, never returned to the
// request path.
func (s *sqlSink) write(ctx context.Context, batch []*model.Trace) {
	if len(batch) == 0 {
		return
	}
	defer s.pending.Add(-int64(len(batch)))

	written, err := model.InsertTraces(ctx, batch, s.batchSize)
	if written > 0 {
		metrics.RecordTraceOutcome(metrics.TraceOutcomeWritten, written)
	}
	if err != nil {
		failed := len(batch) - written
		metrics.RecordTraceOutcome(metrics.TraceOutcomeWriteFailed, failed)
		logger.Logger.Warn("failed to write trace batch",
			zap.Error(err),
			zap.Int("batch_size", len(batch)),
			zap.Int("written", written),
			zap.Int("failed", failed))
	}
}
