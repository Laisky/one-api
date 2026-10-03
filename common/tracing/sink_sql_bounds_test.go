package tracing

// G1 acceptance gates for the batched SQL trace sink (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 1 / W1):
// multiple writers, database outage, bounded drain and shutdown timeout.
//
// These are the gates a healthy-shutdown test cannot substitute for: each one
// reproduces a failure mode -- a partially filled batch on another writer, a
// database that rejects everything, a saturated queue, a deadline that expires
// with work outstanding -- and asserts what the sink reports about it.

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/metrics"
	"github.com/Laisky/one-api/model"
)

// insertObserver records the row count of every INSERT the sink issues, which
// is how a test can see the shape of the batches a writer actually produced
// rather than only their end result.
type insertObserver struct {
	glogger.Interface

	mu   sync.Mutex
	rows []int64
}

// Trace implements gorm's statement hook and records multi-row INSERT sizes.
//
// Parameters:
//   - ctx: the statement context.
//   - begin: when the statement began.
//   - fc: lazily supplies SQL text and affected rows.
//   - err: the statement result.
//
// Return values: none.
func (o *insertObserver) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, affected := fc()
	if !strings.Contains(strings.ToLower(sql), "insert into") {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.rows = append(o.rows, affected)
}

// statements returns the row count of every observed INSERT, in order.
//
// Parameters: none.
//
// Return values:
//   - []int64: rows affected by each INSERT statement.
func (o *insertObserver) statements() []int64 {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]int64, len(o.rows))
	copy(out, o.rows)
	return out
}

// useIsolatedTraceFileDB points model.DB at a private file-backed SQLite
// database for one test.
//
// A file database is used instead of the shared in-memory one because these
// tests take the database down or run several writers against it, neither of
// which may leak into another test's process-wide shared cache. The pool is
// limited to a single connection because SQLite serializes writers anyway and a
// larger pool only adds lock contention.
//
// Parameters:
//   - t: the test, used to register cleanup.
//   - observer: optional statement observer; nil installs a silent logger.
//
// Return values:
//   - *gorm.DB: the isolated handle, for direct assertions.
func useIsolatedTraceFileDB(t *testing.T, observer *insertObserver) *gorm.DB {
	t.Helper()

	var logging glogger.Interface = glogger.Discard
	if observer != nil {
		observer.Interface = glogger.Discard
		logging = observer
	}

	path := filepath.Join(t.TempDir(), "traces.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logging})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, db.AutoMigrate(&model.Trace{}))

	prev := model.DB
	model.DB = db
	model.ResetTraceColumnProbe()
	t.Cleanup(func() {
		model.DB = prev
		model.ResetTraceColumnProbe()
	})
	return db
}

// slowInserts makes every INSERT take at least d, so a Flush that returned
// before its writers finished would be observable as missing rows.
//
// Parameters:
//   - t: the test, used to fail fast when the callback cannot be registered.
//   - db: the handle to instrument.
//   - d: the artificial per-statement latency.
//
// Return values: none.
func slowInserts(t *testing.T, db *gorm.DB, d time.Duration) {
	t.Helper()
	require.NoError(t, db.Callback().Create().Before("gorm:create").
		Register("trace_test_slow_insert", func(tx *gorm.DB) { time.Sleep(d) }))
}

// withBatchByteLimit installs TRACE_BATCH_MAX_BYTES for one test.
//
// Parameters:
//   - t: the test, used to register cleanup.
//   - limit: the byte ceiling for one flush-local batch.
//
// Return values: none.
func withBatchByteLimit(t *testing.T, limit int) {
	t.Helper()
	prev := config.TraceBatchMaxBytes
	config.TraceBatchMaxBytes = limit
	t.Cleanup(func() { config.TraceBatchMaxBytes = prev })
}

// waitForQueueDrain blocks until every submitted row has been taken by a
// writer, so the next submission is handed to the next parked writer.
//
// Parameters:
//   - t: the test handle.
//   - s: the sink to observe.
//
// Return values: none.
func waitForQueueDrain(t *testing.T, s *sqlSink) {
	t.Helper()
	require.Eventually(t, func() bool { return len(s.queue) == 0 },
		5*time.Second, 100*time.Microsecond, "writers must consume the queued row")
}

// TestSQLSinkFlushWaitsForEveryWriterInFlight is the multi-writer barrier gate:
// several writers each hold a partial batch, and Flush must not return until
// every one of them has persisted what it held.
//
// A single-writer test cannot show this. The barrier is a process-wide property
// of the pending counter, so a Flush that only signalled writers -- or that
// waited on one writer's batch -- would pass every other test in this package.
func TestSQLSinkFlushWaitsForEveryWriterInFlight(t *testing.T) {
	observer := &insertObserver{}
	db := useIsolatedTraceFileDB(t, observer)
	slowInserts(t, db, 10*time.Millisecond)
	installCountingRecorder(t)
	// Four writers, a batch ceiling far above the volume and an hour-long
	// ticker: nothing but Flush can cause a write, and every writer holds rows.
	withSinkConfig(t, 1000, 100000, 4, int(time.Hour/time.Millisecond))

	sink := newSQLSink(context.Background()).(*sqlSink)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		require.NoError(t, sink.Close(ctx))
	})

	const total = 200
	for i := range total {
		require.NoError(t, sink.Submit(context.Background(), newTestRow(t, fmt.Sprintf("barrier-%03d", i))))
		waitForQueueDrain(t, sink)
	}

	var before int64
	require.NoError(t, db.Model(&model.Trace{}).Count(&before).Error)
	require.Zero(t, before, "no writer may write before the flush is requested")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	require.NoError(t, sink.Flush(ctx))

	var after int64
	require.NoError(t, db.Model(&model.Trace{}).Count(&after).Error)
	require.Equal(t, int64(total), after,
		"Flush must not return until every writer's in-flight batch persisted")
	require.GreaterOrEqual(t, len(observer.statements()), 2,
		"the rows must have been spread over several writers for this gate to mean anything")
	require.Zero(t, sink.pending.Load())
}

// TestSQLSinkFlushReportsSustainedDatabaseOutage is the database-outage gate
// and the regression test for the defect where write() released the pending
// count unconditionally: pending reached zero although every INSERT had failed,
// so Flush returned nil after a total outage and shutdown reported success
// while losing every accepted trace.
func TestSQLSinkFlushReportsSustainedDatabaseOutage(t *testing.T) {
	db := useIsolatedTraceFileDB(t, nil)
	recorder := installCountingRecorder(t)
	withSinkConfig(t, 100, 1000, 1, int(time.Hour/time.Millisecond))

	sink := newSQLSink(context.Background()).(*sqlSink)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = sink.Close(ctx) // the database is down; Close reports the loss
	})

	const total = 3
	for i := range total {
		require.NoError(t, sink.Submit(context.Background(), newTestRow(t, fmt.Sprintf("outage-%d", i))))
	}

	// Take the database down for good, the way a failed failover or an
	// exhausted connection pool does.
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = sink.Flush(ctx)
	require.Error(t, err, "Flush must NOT return nil when no accepted record persisted")

	var unpersisted *unpersistedTracesError
	require.ErrorAs(t, err, &unpersisted)
	require.Equal(t, int64(total), unpersisted.failed)
	require.Equal(t, int64(total), unpersisted.total())
	require.Equal(t, total, recorder.count(metrics.TraceOutcomeWriteFailed))
	require.Zero(t, recorder.count(metrics.TraceOutcomeWritten))

	// Flush is observational: a later caller must still see the accepted rows
	// that were lost, rather than receiving false confirmation from a previous
	// caller consuming the failure counter.
	require.Error(t, sink.Flush(ctx))
}

// TestSQLSinkCloseReportsUnfinishedWork is the shutdown-timeout gate: a Close
// whose deadline expires with work outstanding must say how many accepted
// records were not persisted, not merely that it timed out.
func TestSQLSinkCloseReportsUnfinishedWork(t *testing.T) {
	useIsolatedTraceDB(t)
	installCountingRecorder(t)

	sink := &sqlSink{
		queue:     make(chan *model.Trace, 8),
		batchSize: 4,
		interval:  time.Hour,
		closed:    make(chan struct{}),
	}

	// A writer that outlives the shutdown deadline, standing in for a database
	// call that has not returned yet.
	release := make(chan struct{})
	sink.wg.Add(1)
	go func() {
		defer sink.wg.Done()
		<-release
	}()
	t.Cleanup(func() { close(release) })

	const total = 3
	for i := range total {
		require.NoError(t, sink.Submit(context.Background(), newTestRow(t, fmt.Sprintf("shutdown-%d", i))))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := sink.Close(ctx)
	require.Error(t, err)

	var unfinished *unpersistedTracesError
	require.ErrorAs(t, err, &unfinished)
	require.Equal(t, int64(total), unfinished.pending, "the deadline must report unfinished work")
	require.Equal(t, int64(total), unfinished.total())
	require.ErrorIs(t, err, context.DeadlineExceeded, "the deadline cause must stay inspectable")
	require.Contains(t, err.Error(), "3 accepted records")
}

// TestSQLSinkDrainIsBoundedByRowsAndBytes is the bounded-drain gate: a FULL
// queue must never be pulled into one flush-local slice.
//
// The prohibited behavior is exactly what the previous takeQueued did -- loop
// the whole queue into one batch -- which at TRACE_QUEUE_SIZE=50000 built
// 50000 records, each holding its serialized document, before writing any.
func TestSQLSinkDrainIsBoundedByRowsAndBytes(t *testing.T) {
	useIsolatedTraceDB(t)
	installCountingRecorder(t)

	const queueSize = 500

	fillQueue := func(t *testing.T, s *sqlSink) {
		t.Helper()
		for i := range queueSize {
			s.queue <- newTestRow(t, fmt.Sprintf("bounded-%03d", i))
		}
		require.Equal(t, queueSize, len(s.queue), "the gate needs a saturated queue")
	}

	t.Run("row bound", func(t *testing.T) {
		const maxRows = 25
		s := &sqlSink{
			queue:         make(chan *model.Trace, queueSize),
			batchSize:     queueSize,
			maxBatchRows:  maxRows,
			maxBatchBytes: 1 << 30, // only the row bound may bind here
			interval:      time.Hour,
			closed:        make(chan struct{}),
		}
		fillQueue(t, s)

		batch := newTraceBatch(s.rowsPerStatement(), s.batchBytesLimit())
		s.fill(batch)

		require.Equal(t, maxRows, batch.len(), "one drain must stop at the row bound")
		require.Equal(t, queueSize-maxRows, len(s.queue),
			"the drain must leave the rest queued instead of materializing it")
	})

	t.Run("byte bound", func(t *testing.T) {
		const maxBytes = 4096
		s := &sqlSink{
			queue:         make(chan *model.Trace, queueSize),
			batchSize:     queueSize,
			maxBatchRows:  queueSize, // only the byte bound may bind here
			maxBatchBytes: maxBytes,
			interval:      time.Hour,
			closed:        make(chan struct{}),
		}
		fillQueue(t, s)

		batch := newTraceBatch(s.rowsPerStatement(), s.batchBytesLimit())
		s.fill(batch)

		require.Positive(t, batch.len())
		require.Less(t, batch.len(), queueSize, "one drain must not absorb a full queue")
		rowBytes := estimateTraceRowBytes(newTestRow(t, "bounded-000"))
		require.LessOrEqual(t, batch.bytes, int64(maxBytes)+rowBytes,
			"a batch may exceed the byte bound by at most the row that crossed it")
		require.Equal(t, queueSize-batch.len(), len(s.queue))
	})

	t.Run("a saturated queue still drains completely, in bounded statements", func(t *testing.T) {
		observer := &insertObserver{}
		db := useIsolatedTraceFileDB(t, observer)
		withBatchByteLimit(t, 4096)
		// One writer, an hour-long ticker and a row ceiling above the volume:
		// only the byte bound can decide the batch shape.
		withSinkConfig(t, queueSize, queueSize, 1, int(time.Hour/time.Millisecond))

		sink := newSQLSink(context.Background()).(*sqlSink)
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			require.NoError(t, sink.Close(ctx))
		})

		const total = 200
		for i := range total {
			require.NoError(t, sink.Submit(context.Background(), newTestRow(t, fmt.Sprintf("drain-%03d", i))))
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		require.NoError(t, sink.Flush(ctx))

		var count int64
		require.NoError(t, db.Model(&model.Trace{}).Count(&count).Error)
		require.Equal(t, int64(total), count, "bounding the drain must not lose accepted records")

		statements := observer.statements()
		require.Greater(t, len(statements), 1,
			"a bounded drain must produce several statements, not one oversized batch")
		rowBytes := estimateTraceRowBytes(newTestRow(t, "drain-000"))
		maxRowsPerBatch := int64(4096)/rowBytes + 1
		for _, rows := range statements {
			require.LessOrEqual(t, rows, maxRowsPerBatch,
				"every INSERT must respect the flush-local byte bound")
		}
	})
}

// TestMaxRowsPerStatementBoundsParameters verifies the per-statement parameter
// ceiling: at the default batch size it must not bind, and it must always leave
// room for the parameters one row binds.
func TestMaxRowsPerStatementBoundsParameters(t *testing.T) {
	rows := maxRowsPerStatement()
	require.Positive(t, rows)
	require.LessOrEqual(t, rows*traceRowParameterCount, model.MaxStatementParametersDefault)

	s := &sqlSink{batchSize: 1_000_000}
	require.Equal(t, rows, s.rowsPerStatement(),
		"an oversized TRACE_BATCH_SIZE must be capped by the parameter ceiling")

	s = &sqlSink{batchSize: 7}
	require.Equal(t, 7, s.rowsPerStatement(), "a small batch size must be honored as configured")
}

// TestSQLSinkFlushErrorIsWrapped verifies the reported failure keeps the
// project's error-wrapping contract and never carries record content.
func TestSQLSinkFlushErrorIsWrapped(t *testing.T) {
	s := &sqlSink{
		queue:    make(chan *model.Trace, 1),
		closed:   make(chan struct{}),
		interval: time.Hour,
	}
	s.unpersisted.Store(5)

	err := s.reportUnpersisted("flush trace sink")
	require.Error(t, err)
	require.Contains(t, err.Error(), "flush trace sink")
	require.Contains(t, err.Error(), "5 accepted records")

	var unpersisted *unpersistedTracesError
	require.True(t, errors.As(err, &unpersisted))
	require.Equal(t, int64(5), unpersisted.failed)
	require.Error(t, s.reportUnpersisted("flush trace sink"),
		"a later flush must continue reporting accepted records that were lost")
}

// TestSQLSinkFlushDoesNotConsumeFailure verifies every flush observer sees an
// accepted record that failed to persist; one caller must not hide loss from a
// later or concurrent caller.
func TestSQLSinkFlushDoesNotConsumeFailure(t *testing.T) {
	s := &sqlSink{queue: make(chan *model.Trace), closed: make(chan struct{}), interval: time.Hour}
	s.unpersisted.Store(5)

	first := s.reportUnpersisted("flush trace sink")
	second := s.reportUnpersisted("flush trace sink")
	require.Error(t, first)
	require.Error(t, second, "a previous flush must not erase persistence loss")
}

// TestSQLSinkDeadlineCountsFailedRowsOnce verifies a write failure moves a row
// atomically from pending to failed, so a deadline error never reports it twice.
func TestSQLSinkDeadlineCountsFailedRowsOnce(t *testing.T) {
	s := &sqlSink{queue: make(chan *model.Trace), closed: make(chan struct{}), interval: time.Hour}
	s.pending.Store(1)
	s.complete(1, 1)

	err := s.reportDeadline("close trace sink", context.DeadlineExceeded)
	var unpersisted *unpersistedTracesError
	require.ErrorAs(t, err, &unpersisted)
	require.Zero(t, unpersisted.pending)
	require.Equal(t, int64(1), unpersisted.failed)
	require.Equal(t, int64(1), unpersisted.total())
}

// TestSQLSinkCloseCancelsWritersBeforeReturning verifies a shutdown deadline
// cancels a context-honoring blocked database write, whose worker then exits
// promptly after Close returns its unfinished-work error.
func TestSQLSinkCloseCancelsWritersBeforeReturning(t *testing.T) {
	db := useIsolatedTraceFileDB(t, nil)
	installCountingRecorder(t)
	withSinkConfig(t, 8, 8, 1, int(time.Hour/time.Millisecond))

	started := make(chan struct{})
	var once sync.Once
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(
		"trace_test_wait_for_sink_cancellation", func(tx *gorm.DB) {
			once.Do(func() { close(started) })
			<-tx.Statement.Context.Done()
		}))

	before := sqlSinkWorkerCount.Load()
	sink := newSQLSink(context.Background()).(*sqlSink)
	require.NoError(t, sink.Submit(context.Background(), newTestRow(t, "close-cancels-worker")))
	sink.requestFlush()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the sink never reached the blocked database write")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := sink.Close(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded)

	require.Eventually(t, func() bool { return sqlSinkWorkerCount.Load() == before },
		5*time.Second, time.Millisecond,
		"a context-honoring writer must exit promptly after Close cancels it")
}

// TestSQLSinkCloseDeadlineReportsNonCooperativeWriter verifies Close preserves
// its deadline when a database driver ignores context cancellation. The worker
// remains visible as unfinished work until the driver returns, so shutdown can
// skip closing the database underneath it.
func TestSQLSinkCloseDeadlineReportsNonCooperativeWriter(t *testing.T) {
	db := useIsolatedTraceFileDB(t, nil)
	installCountingRecorder(t)
	withSinkConfig(t, 8, 8, 1, int(time.Hour/time.Millisecond))

	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(
		"trace_test_ignore_sink_cancellation", func(*gorm.DB) {
			once.Do(func() { close(started) })
			<-release
		}))

	before := sqlSinkWorkerCount.Load()
	sink := newSQLSink(context.Background()).(*sqlSink)
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		require.Eventually(t, func() bool { return sqlSinkWorkerCount.Load() == before },
			5*time.Second, time.Millisecond)
	})
	require.NoError(t, sink.Submit(context.Background(), newTestRow(t, "close-non-cooperative-worker")))
	sink.requestFlush()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the sink never reached the non-cooperative database write")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	startedAt := time.Now()
	err := sink.Close(ctx)
	require.Less(t, time.Since(startedAt), time.Second,
		"Close must return at its deadline instead of waiting for an uncooperative driver")
	require.ErrorIs(t, err, context.DeadlineExceeded)

	var unfinished *unpersistedTracesError
	require.ErrorAs(t, err, &unfinished)
	require.Equal(t, int64(1), unfinished.pending)
	require.Equal(t, before+1, sqlSinkWorkerCount.Load(),
		"the live worker must remain visible until the driver releases it")

	close(release)
	require.Eventually(t, func() bool { return sqlSinkWorkerCount.Load() == before },
		5*time.Second, time.Millisecond,
		"the worker must exit once the non-cooperative driver returns")
}
