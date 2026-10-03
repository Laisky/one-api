package model

// Retention cleaner shutdown (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 1 / W1, row
// "Shutdown": "Stop admissions, drain HTTP and background/billing producers,
// close consuming sinks, flush exporters, then close databases; deadlines report
// unfinished work").
//
// The defect these tests pin: the cleaners were started with a context that was
// never cancelled, so their tickers kept firing while the process shut down and
// after CloseDB, issuing DELETEs against a closed pool. Cancelling alone is not
// enough either -- a shutdown that does not JOIN the cleaner races the last
// in-flight statement to the database close.

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"

	"github.com/Laisky/one-api/common/config"
)

// statementRecorder records when statements issued through a handle finished.
type statementRecorder struct {
	mu sync.Mutex
	// count is how many statements completed.
	count int
	// lastEnd is when the most recent statement completed.
	lastEnd time.Time
	// first is closed once the first statement has been observed.
	first     chan struct{}
	firstOnce sync.Once
}

// observeStart records the beginning of a statement.
//
// Parameters: none.
//
// Return values: none.
func (r *statementRecorder) observeStart() {
	r.firstOnce.Do(func() { close(r.first) })
}

// observeEnd records the completion of a statement.
//
// Parameters: none.
//
// Return values: none.
func (r *statementRecorder) observeEnd() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.count++
	r.lastEnd = time.Now().UTC()
}

// snapshot returns the recorded statement count and last completion instant.
//
// Parameters: none.
//
// Return values:
//   - int: statements completed so far.
//   - time.Time: when the most recent one completed.
func (r *statementRecorder) snapshot() (int, time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.count, r.lastEnd
}

// newSlowRetentionDB opens a seeded traces database whose statements are slow
// enough that a cancellation lands in the middle of a sweep rather than between
// two sweeps.
//
// Parameters:
//   - t: the running test; the handle and globals are restored on cleanup.
//   - rows: how many expired trace rows to seed.
//   - perStatement: how long every statement is delayed.
//
// Return values:
//   - *gorm.DB: the installed handle.
//   - *statementRecorder: the recorder wired to that handle.
func newSlowRetentionDB(t *testing.T, rows int, perStatement time.Duration) (*gorm.DB, *statementRecorder) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/retention-shutdown.db"),
		&gorm.Config{Logger: glogger.Discard})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Trace{}))

	// Every seeded row is far older than the one-day window used below.
	expired := time.Now().UTC().Add(-72 * time.Hour).UnixMilli()
	for i := range rows {
		suffix := strconv.Itoa(i)
		require.NoError(t, db.Exec(
			"INSERT INTO traces (uuid, trace_id, url, method, body_size, status, timestamps, created_at, updated_at) "+
				"VALUES (?,?,?,?,?,?,?,?,?)",
			"uuid-"+suffix, "trace-"+suffix, "/v1/chat/completions", "POST", 0, 200, "{}",
			expired+int64(i), expired+int64(i)).Error)
	}

	recorder := &statementRecorder{first: make(chan struct{})}
	require.NoError(t, db.Callback().Raw().Before("gorm:raw").Register(
		"test:slow-retention-statement", func(*gorm.DB) {
			recorder.observeStart()
			time.Sleep(perStatement)
		}))
	require.NoError(t, db.Callback().Raw().After("gorm:raw").Register(
		"test:record-retention-statement", func(*gorm.DB) {
			recorder.observeEnd()
		}))

	prevDB := DB
	prevBatch := config.RetentionDeleteBatchSize
	DB = db
	// A small batch forces many chunks, so the sweep cannot finish before the
	// cancellation arrives and must stop at a chunk boundary.
	config.RetentionDeleteBatchSize = 2
	t.Cleanup(func() {
		DB = prevDB
		config.RetentionDeleteBatchSize = prevBatch
	})

	return db, recorder
}

// TestRetentionCleanerIsJoinedBeforeItsDatabaseCloses verifies the two halves of
// the fix together: cancelling the workers' context stops the sweep at its next
// chunk boundary, and WaitForRetentionCleaners does not return until the cleaner
// goroutine has actually finished, so no statement can reach the database after
// the shutdown sequence believes the producers stopped.
func TestRetentionCleanerIsJoinedBeforeItsDatabaseCloses(t *testing.T) {
	db, recorder := newSlowRetentionDB(t, 40, 120*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	StartTraceRetentionCleaner(ctx, 1)

	select {
	case <-recorder.first:
	case <-time.After(5 * time.Second):
		t.Fatal("the retention sweep never issued a statement")
	}

	// Cancel while a statement is in flight: this is the shutdown case, not a
	// cleaner sitting idle between sweeps.
	cancel()

	waitCtx, waitCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer waitCancel()
	require.NoError(t, WaitForRetentionCleaners(waitCtx),
		"the cleaner must be joinable and must stop promptly after cancellation")
	joinedAt := time.Now().UTC()

	// Give any statement the join failed to wait for time to complete and be
	// recorded; without the join, the sweep is still running at this point.
	time.Sleep(500 * time.Millisecond)

	count, lastEnd := recorder.snapshot()
	require.Positive(t, count, "the sweep must have issued at least one statement")
	require.True(t, lastEnd.Before(joinedAt),
		"a statement completed %v after the cleaners were reported joined: the shutdown "+
			"sequence would have closed the database under a running sweep",
		lastEnd.Sub(joinedAt))

	var remaining int64
	require.NoError(t, db.Model(&Trace{}).Count(&remaining).Error)
	require.Positive(t, remaining,
		"the sweep must stop at a chunk boundary rather than finish an unbounded sweep")
}

// TestWaitForRetentionCleanersReportsDeadline verifies the join reports
// unfinished work instead of returning success when the cleaners are still
// running at the shutdown deadline.
func TestWaitForRetentionCleanersReportsDeadline(t *testing.T) {
	_, recorder := newSlowRetentionDB(t, 40, 120*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	StartTraceRetentionCleaner(ctx, 1)
	t.Cleanup(func() {
		cancel()
		joinCtx, joinCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer joinCancel()
		require.NoError(t, WaitForRetentionCleaners(joinCtx))
	})

	select {
	case <-recorder.first:
	case <-time.After(5 * time.Second):
		t.Fatal("the retention sweep never issued a statement")
	}

	// The cleaner's context is NOT cancelled here: this is the deadline case,
	// where a sweep is still running when the shutdown budget runs out.
	deadlineCtx, deadlineCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer deadlineCancel()

	err := WaitForRetentionCleaners(deadlineCtx)
	require.Error(t, err, "a cleaner still running at the deadline is unfinished work")
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

// TestWaitForRetentionCleanersReturnsWithoutWorkers verifies the join is a no-op
// when retention is disabled, so a deployment that starts no cleaners never has
// retention named in the shutdown deadline report.
func TestWaitForRetentionCleanersReturnsWithoutWorkers(t *testing.T) {
	// Earlier tests in this package stop their cleaners on cleanup, but a
	// goroutine takes a moment to actually return; wait for the process to be
	// genuinely worker-free before asserting what happens without workers.
	require.Eventually(t, func() bool { return retentionWorkersActive.Load() == 0 },
		10*time.Second, 10*time.Millisecond, "a retention cleaner is still running")

	expired, cancel := context.WithDeadline(context.Background(), time.Now().UTC().Add(-time.Second))
	defer cancel()

	require.NoError(t, WaitForRetentionCleaners(expired),
		"an expired deadline with no workers must not be reported as unfinished work")
}

// TestBackgroundWorkerLifecycleJoinsCancelledProducer verifies the generic
// registry used by option, channel-cache, and MCP workers cannot report a
// cancelled producer as stopped before its goroutine has returned.
func TestBackgroundWorkerLifecycleJoinsCancelledProducer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	returned := make(chan struct{})
	StartBackgroundWorker(ctx, func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		close(returned)
	})

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("background worker did not start")
	}
	cancel()

	joinCtx, joinCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer joinCancel()
	require.NoError(t, WaitForBackgroundWorkers(joinCtx))
	select {
	case <-returned:
	default:
		t.Fatal("background worker join returned before its function returned")
	}
}
