package tracing

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/metrics"
	"github.com/Laisky/one-api/model"
)

// countingRecorder counts trace-pipeline outcomes. It embeds NoOpRecorder so it
// satisfies metrics.MetricsRecorder without restating every unrelated method.
type countingRecorder struct {
	*metrics.NoOpRecorder

	mu       sync.Mutex
	outcomes map[string]int
}

// newCountingRecorder builds an outcome-counting metrics recorder.
//
// Parameters: none.
//
// Return values:
//   - *countingRecorder: a recorder with an empty outcome tally.
func newCountingRecorder() *countingRecorder {
	return &countingRecorder{NoOpRecorder: &metrics.NoOpRecorder{}, outcomes: map[string]int{}}
}

// lockedWriteObserver observes actual SQLite statement failures so a contention
// test can release its lock only after the sink has attempted a write.
type lockedWriteObserver struct {
	glogger.Interface
	locked chan struct{}
}

// Trace implements gorm's SQL tracing hook and reports SQLite lock failures.
//
// Parameters:
//   - ctx: the statement context.
//   - begin: when the statement began.
//   - fc: lazily supplies SQL text and row count.
//   - err: the statement result.
//
// Return values: none.
func (l *lockedWriteObserver) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	l.Interface.Trace(ctx, begin, fc, err)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "database is locked") {
		return
	}
	select {
	case l.locked <- struct{}{}:
	default:
	}
}

// RecordTraceRecord tallies one trace-pipeline outcome.
//
// Parameters:
//   - outcome: a compile-time constant from common/metrics/trace_pipeline.go.
//   - count: how many records the outcome applies to.
//
// Return values: none.
func (c *countingRecorder) RecordTraceRecord(outcome string, count int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.outcomes[outcome] += count
}

// count returns the tally for one outcome.
//
// Parameters:
//   - outcome: the outcome to read.
//
// Return values:
//   - int: how many records carried that outcome.
func (c *countingRecorder) count(outcome string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.outcomes[outcome]
}

// installCountingRecorder swaps in an outcome-counting recorder for one test.
//
// Parameters:
//   - t: the test, used to register cleanup.
//
// Return values:
//   - *countingRecorder: the installed recorder.
func installCountingRecorder(t *testing.T) *countingRecorder {
	t.Helper()
	rec := newCountingRecorder()
	prev := metrics.Recorder()
	metrics.SetRecorder(rec)
	t.Cleanup(func() { metrics.SetRecorder(prev) })
	return rec
}

// useIsolatedTraceDB points model.DB at a fresh in-memory database for one test.
//
// Parameters:
//   - t: the test, used to register cleanup.
//
// Return values:
//   - *gorm.DB: the isolated handle, for direct assertions.
func useIsolatedTraceDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Trace{}))
	require.NoError(t, db.Exec("DELETE FROM traces").Error)

	prev := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = prev })
	return db
}

// withSinkConfig installs trace sink configuration for one test.
//
// Parameters:
//   - t: the test, used to register cleanup.
//   - queueSize, batchSize, writerCount: sink sizing.
//   - flushIntervalMs: maximum age of a partially filled batch.
//
// Return values: none.
func withSinkConfig(t *testing.T, queueSize, batchSize, writerCount, flushIntervalMs int) {
	t.Helper()
	prevQueue, prevBatch, prevWriters, prevInterval :=
		config.TraceQueueSize, config.TraceBatchSize, config.TraceWriterCount, config.TraceFlushIntervalMs
	config.TraceQueueSize, config.TraceBatchSize, config.TraceWriterCount, config.TraceFlushIntervalMs =
		queueSize, batchSize, writerCount, flushIntervalMs
	t.Cleanup(func() {
		config.TraceQueueSize, config.TraceBatchSize, config.TraceWriterCount, config.TraceFlushIntervalMs =
			prevQueue, prevBatch, prevWriters, prevInterval
	})
}

// newTestRow builds a trace row for sink tests.
//
// Parameters:
//   - t: the test, used to fail fast on a builder error.
//   - traceID: the row's unique trace identifier.
//
// Return values:
//   - *model.Trace: the row ready for submission.
func newTestRow(t *testing.T, traceID string) *model.Trace {
	t.Helper()
	row, _, err := model.NewTraceRow(model.TraceRowInput{
		TraceId: traceID,
		URL:     "/v1/chat/completions",
		Method:  "POST",
		Status:  200,
	})
	require.NoError(t, err)
	return row
}

// TestSQLSinkFlushesOnBatchSize verifies a full batch is written without waiting
// for the flush interval.
func TestSQLSinkFlushesOnBatchSize(t *testing.T) {
	db := useIsolatedTraceDB(t)
	installCountingRecorder(t)
	// One hour interval: only the batch-size trigger can fire.
	withSinkConfig(t, 100, 2, 1, int(time.Hour/time.Millisecond))

	sink := newSQLSink(context.Background())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, sink.Close(ctx))
	})

	require.NoError(t, sink.Submit(context.Background(), newTestRow(t, "batch-a")))
	require.NoError(t, sink.Submit(context.Background(), newTestRow(t, "batch-b")))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, sink.Flush(ctx))

	var count int64
	require.NoError(t, db.Model(&model.Trace{}).Count(&count).Error)
	require.Equal(t, int64(2), count)
}

// TestSQLSinkFlushesOnInterval verifies a partially filled batch is written once
// the flush interval elapses, so a low-traffic deployment never strands traces.
func TestSQLSinkFlushesOnInterval(t *testing.T) {
	db := useIsolatedTraceDB(t)
	installCountingRecorder(t)
	// Batch size far above the submitted volume: only the interval can fire.
	withSinkConfig(t, 100, 1000, 1, 20)

	sink := newSQLSink(context.Background())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, sink.Close(ctx))
	})

	require.NoError(t, sink.Submit(context.Background(), newTestRow(t, "interval-a")))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, sink.Flush(ctx))

	var count int64
	require.NoError(t, db.Model(&model.Trace{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

// TestSQLSinkDropsWhenQueueIsFull verifies the capacity policy: a saturated
// queue drops the newest record and counts the drop instead of blocking the
// request goroutine.
func TestSQLSinkDropsWhenQueueIsFull(t *testing.T) {
	useIsolatedTraceDB(t)
	rec := installCountingRecorder(t)

	// Construct the sink with no writers so nothing drains the queue: the
	// capacity policy is then observable without racing a writer.
	sink := &sqlSink{
		queue:     make(chan *model.Trace, 2),
		batchSize: 10,
		interval:  time.Hour,
		closed:    make(chan struct{}),
	}

	require.NoError(t, sink.Submit(context.Background(), newTestRow(t, "full-a")))
	require.NoError(t, sink.Submit(context.Background(), newTestRow(t, "full-b")))
	require.NoError(t, sink.Submit(context.Background(), newTestRow(t, "full-c")))
	require.NoError(t, sink.Submit(context.Background(), newTestRow(t, "full-d")))

	require.Equal(t, 2, rec.count(metrics.TraceOutcomeQueued))
	require.Equal(t, 2, rec.count(metrics.TraceOutcomeDroppedQueueFull),
		"a saturated queue must drop and count, never block")
}

// TestSQLSinkCloseFlushesEverything verifies graceful shutdown loses nothing
// that was already accepted.
func TestSQLSinkCloseFlushesEverything(t *testing.T) {
	db := useIsolatedTraceDB(t)
	installCountingRecorder(t)
	// One hour interval and a batch size above the volume: only the shutdown
	// drain can write these rows.
	withSinkConfig(t, 100, 1000, 2, int(time.Hour/time.Millisecond))

	sink := newSQLSink(context.Background())

	const total = 25
	for i := range total {
		require.NoError(t, sink.Submit(context.Background(), newTestRow(t, "close-"+string(rune('a'+i)))))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, sink.Close(ctx))

	var count int64
	require.NoError(t, db.Model(&model.Trace{}).Count(&count).Error)
	require.Equal(t, int64(total), count, "graceful shutdown must not lose accepted traces")
}

// TestSQLSinkRetriesTransientSQLiteLock verifies an accepted trace survives a
// short-lived SQLite write lock. This reproduces a real contention condition
// rather than injecting an error: the first INSERT is attempted while a second
// connection owns an exclusive transaction, then the lock is released.
func TestSQLSinkRetriesTransientSQLiteLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "traces.db")
	const dsnSuffix = "?_busy_timeout=1"
	writeAttempted := make(chan struct{}, 1)
	observer := &lockedWriteObserver{Interface: glogger.Discard, locked: writeAttempted}
	db, err := gorm.Open(sqlite.Open(path+dsnSuffix), &gorm.Config{Logger: observer})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Trace{}))

	locker, err := gorm.Open(sqlite.Open(path+dsnSuffix), &gorm.Config{})
	require.NoError(t, err)
	lockerSQL, err := locker.DB()
	require.NoError(t, err)
	lockConn, err := lockerSQL.Conn(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, lockConn.Close()) })

	prevDB := model.DB
	prevSQLite := common.UsingSQLite.Load()
	model.DB = db
	common.UsingSQLite.Store(true)
	t.Cleanup(func() {
		model.DB = prevDB
		common.UsingSQLite.Store(prevSQLite)
	})
	model.ResetTraceColumnProbe()
	warmRow := newTestRow(t, "transient-lock-warmup")
	written, err := model.InsertTraces(context.Background(), []*model.Trace{warmRow}, 1)
	require.NoError(t, err)
	require.Equal(t, 1, written)
	require.NoError(t, db.Where("trace_id = ?", warmRow.TraceId).Delete(&model.Trace{}).Error)

	lockCtx, lockCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer lockCancel()
	_, err = lockConn.ExecContext(lockCtx, "BEGIN EXCLUSIVE")
	require.NoError(t, err)
	locked := true
	defer func() {
		if locked {
			_, rollbackErr := lockConn.ExecContext(context.Background(), "ROLLBACK")
			require.NoError(t, rollbackErr)
		}
	}()

	installCountingRecorder(t)
	withSinkConfig(t, 10, 1, 1, int(time.Hour/time.Millisecond))
	sink := newSQLSink(context.Background())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, sink.Close(ctx))
	})

	require.NoError(t, sink.Submit(context.Background(), newTestRow(t, "transient-lock")))
	select {
	case <-writeAttempted:
	case <-time.After(5 * time.Second):
		t.Fatal("trace sink did not attempt its write while SQLite was locked")
	}

	_, err = lockConn.ExecContext(lockCtx, "COMMIT")
	require.NoError(t, err)
	locked = false

	flushCtx, flushCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer flushCancel()
	require.NoError(t, sink.Flush(flushCtx))

	var count int64
	require.NoError(t, db.Model(&model.Trace{}).Where("trace_id = ?", "transient-lock").Count(&count).Error)
	require.Equal(t, int64(1), count, "accepted trace must survive transient SQLite contention")
}

// TestSQLSinkSubmitAfterCloseIsDropped verifies a late submission is counted as
// a drop rather than panicking on a closed channel.
func TestSQLSinkSubmitAfterCloseIsDropped(t *testing.T) {
	useIsolatedTraceDB(t)
	rec := installCountingRecorder(t)
	withSinkConfig(t, 10, 10, 1, 50)

	sink := newSQLSink(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, sink.Close(ctx))

	require.NotPanics(t, func() {
		require.NoError(t, sink.Submit(context.Background(), newTestRow(t, "after-close")))
	})
	require.Equal(t, 1, rec.count(metrics.TraceOutcomeDroppedClosed))
}

// TestSQLSinkSubmitNilRow verifies the sink tolerates a nil row.
func TestSQLSinkSubmitNilRow(t *testing.T) {
	useIsolatedTraceDB(t)
	installCountingRecorder(t)
	withSinkConfig(t, 10, 10, 1, 50)

	sink := newSQLSink(context.Background())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, sink.Close(ctx))
	})

	require.NoError(t, sink.Submit(context.Background(), nil))
}

// TestSQLSinkFlushRespectsDeadline verifies Flush surfaces a timeout instead of
// blocking shutdown forever when writers cannot drain.
func TestSQLSinkFlushRespectsDeadline(t *testing.T) {
	useIsolatedTraceDB(t)
	installCountingRecorder(t)

	sink := &sqlSink{
		queue:     make(chan *model.Trace, 4),
		batchSize: 10,
		interval:  time.Hour,
		closed:    make(chan struct{}),
	}
	require.NoError(t, sink.Submit(context.Background(), newTestRow(t, "stuck")))

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	require.Error(t, sink.Flush(ctx))
}

// TestSQLSinkFlushTriggersWriteBeforeTicker verifies Flush makes the writers
// write, rather than merely waiting for their own schedule.
//
// This is the property the trace-lookup path depends on: it flushes with a
// 500 ms deadline, which the 1 s default flush ticker would otherwise miss on
// a partially filled batch, leaving a completed request's trace unfindable.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestSQLSinkFlushTriggersWriteBeforeTicker(t *testing.T) {
	db := useIsolatedTraceDB(t)
	installCountingRecorder(t)
	// A batch far larger than the submitted volume and an hour-long ticker: the
	// only thing that can cause a write is Flush itself.
	withSinkConfig(t, 100, 1000, 2, int(time.Hour/time.Millisecond))

	sink := newSQLSink(context.Background())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, sink.Close(ctx))
	})

	for i := range 5 {
		require.NoError(t, sink.Submit(context.Background(), newTestRow(t, "flush-trigger-"+string(rune('a'+i)))))
	}

	// A deadline far shorter than the flush interval: it can only be met if
	// Flush actively signals the writers.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, sink.Flush(ctx), "Flush must trigger a write, not wait for the ticker")

	var count int64
	require.NoError(t, db.Model(&model.Trace{}).Count(&count).Error)
	require.Equal(t, int64(5), count)
}
