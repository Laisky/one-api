package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/model"
)

// recordingLogger returns a logger that appends every emitted message to a slice.
//
// Parameters:
//   - t: the running test, used to fail when the logger cannot be built.
//
// Return values:
//   - glog.Logger: the logger to hand to runShutdownSequence.
//   - func() []string: snapshot of the messages logged so far, safe to call
//     concurrently with logging.
func recordingLogger(t *testing.T) (glog.Logger, func() []string) {
	t.Helper()

	var (
		mu       sync.Mutex
		messages []string
	)
	lg, err := glog.NewConsoleWithName("shutdown-test", glog.LevelDebug,
		zap.Hooks(func(entry zapcore.Entry) error {
			mu.Lock()
			defer mu.Unlock()
			messages = append(messages, entry.Message)
			return nil
		}))
	require.NoError(t, err)

	return lg, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), messages...)
	}
}

// stepNames extracts the ordered step names of a shutdown sequence.
//
// Parameters:
//   - steps: the sequence to inspect.
//
// Return values:
//   - []string: the step names in order.
func stepNames(steps []shutdownStep) []string {
	names := make([]string, 0, len(steps))
	for _, step := range steps {
		names = append(names, step.name)
	}
	return names
}

// indexOf reports the position of a step name within a sequence.
//
// Parameters:
//   - t: the running test, failed when the name is absent.
//   - names: the ordered step names.
//   - name: the step to locate.
//
// Return values:
//   - int: the zero-based position of name in names.
func indexOf(t *testing.T, names []string, name string) int {
	t.Helper()
	for i, candidate := range names {
		if candidate == name {
			return i
		}
	}
	require.Failf(t, "shutdown step missing", "step %q is not part of the sequence %v", name, names)
	return -1
}

// TestNewShutdownSequenceOrder pins the shutdown ordering contract: admissions
// stop first, producers drain before the consuming sinks are closed, exporters
// flush after the sinks, and the database closes last.
func TestNewShutdownSequenceOrder(t *testing.T) {
	names := stepNames(newShutdownSequence(nil, nil, nil, true, nil))

	require.Equal(t, []string{
		"stop_admissions",
		"http_server",
		"pprof_server",
		"batch_updater",
		"background_tasks",
		"retention_workers",
		"trace_sinks",
		"otel_providers",
		"database",
	}, names)

	admissions := indexOf(t, names, "stop_admissions")
	httpSrv := indexOf(t, names, "http_server")
	batchUpdater := indexOf(t, names, "batch_updater")
	background := indexOf(t, names, "background_tasks")
	retention := indexOf(t, names, "retention_workers")
	sinks := indexOf(t, names, "trace_sinks")
	exporters := indexOf(t, names, "otel_providers")
	database := indexOf(t, names, "database")

	require.Less(t, admissions, httpSrv, "admissions must stop before HTTP drains")
	require.Less(t, httpSrv, batchUpdater, "HTTP must drain before producers are stopped")
	require.Less(t, batchUpdater, background,
		"the batch updater must be signalled before the critical-task drain joins its final flush")
	require.Less(t, background, retention,
		"the periodic retention workers stop after the critical producers, but still as producers")
	require.Less(t, retention, sinks,
		"retention sweeps must be joined before the trace sinks close, so no sweep is "+
			"still deleting rows when the sinks and then the database close under it")
	require.Less(t, retention, database,
		"retention sweeps must stop well before the database handles are closed")
	require.Less(t, background, sinks,
		"trace sinks must close only after background/billing producers drained, "+
			"otherwise their traces are counted dropped_closed")
	require.Less(t, sinks, exporters, "exporters flush after the sinks handed their data over")
	require.Less(t, exporters, database, "the database closes after the exporters flushed")
	require.Equal(t, len(names)-1, database, "the database must be the last step")
}

// TestNewShutdownSequenceKeepsOrderWithDisabledComponents ensures optional
// components do not change the order or disappear from the sequence, so the
// deadline report names the same steps in every deployment shape.
func TestNewShutdownSequenceKeepsOrderWithDisabledComponents(t *testing.T) {
	enabled := stepNames(newShutdownSequence(nil, nil, nil, true, nil))
	disabled := stepNames(newShutdownSequence(nil, nil, nil, false, nil))

	require.Equal(t, enabled, disabled)
}

// TestRunShutdownSequenceRunsInOrder verifies the runner executes every step in
// the declared order and reports no unfinished work on a healthy shutdown.
func TestRunShutdownSequenceRunsInOrder(t *testing.T) {
	lg, messages := recordingLogger(t)

	var executed []string
	record := func(name string) shutdownStep {
		return shutdownStep{
			name:       name,
			failureLog: name + " failed",
			run: func(context.Context) error {
				executed = append(executed, name)
				return nil
			},
		}
	}

	unfinished := runShutdownSequence(context.Background(), lg, []shutdownStep{
		record("first"), record("second"), record("third"),
	})

	require.Empty(t, unfinished)
	require.Equal(t, []string{"first", "second", "third"}, executed)
	require.Contains(t, messages(), "graceful shutdown complete")
}

// TestRunShutdownSequenceReportsUnfinishedWorkOnDeadline verifies that an
// expired deadline is reported and attributed to the steps that did not
// complete, while later steps (the database close above all) still run.
func TestRunShutdownSequenceReportsUnfinishedWorkOnDeadline(t *testing.T) {
	lg, messages := recordingLogger(t)

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().UTC().Add(-time.Second))
	defer cancel()

	var executed []string
	step := func(name string, err error) shutdownStep {
		return shutdownStep{
			name:       name,
			failureLog: "failed to shut down " + name,
			run: func(context.Context) error {
				executed = append(executed, name)
				return err
			},
		}
	}

	unfinished := runShutdownSequence(ctx, lg, []shutdownStep{
		step("http_server", nil),
		step("background_tasks", context.DeadlineExceeded),
		step("trace_sinks", errors.Wrap(context.DeadlineExceeded, "shutdown trace sinks")),
		step("database", nil),
	})

	require.Equal(t, []string{"background_tasks", "trace_sinks"}, unfinished,
		"only the steps that did not complete are attributed to the deadline")
	require.Equal(t, []string{"http_server", "background_tasks", "trace_sinks", "database"}, executed,
		"later steps must still run after the deadline expired, or their resources leak")

	logged := messages()
	require.Contains(t, logged, "failed to shut down background_tasks")
	require.Contains(t, logged, "failed to shut down trace_sinks")
	require.Contains(t, logged, "graceful shutdown deadline expired with unfinished work")
	require.NotContains(t, logged, "graceful shutdown complete")
}

// TestRunShutdownSequenceLogsPlainFailuresWithoutDeadlineAttribution verifies a
// failure unrelated to the deadline is still logged with its own message but is
// not reported as unfinished work.
func TestRunShutdownSequenceLogsPlainFailuresWithoutDeadlineAttribution(t *testing.T) {
	lg, messages := recordingLogger(t)

	unfinished := runShutdownSequence(context.Background(), lg, []shutdownStep{
		{
			name:       "database",
			failureLog: "failed to close database",
			run: func(context.Context) error {
				return errors.New("disk failure")
			},
		},
	})

	require.Empty(t, unfinished)

	logged := messages()
	require.Contains(t, logged, "failed to close database")
	require.NotContains(t, logged, "graceful shutdown deadline expired with unfinished work")
}

// TestRunShutdownSequenceSkipsNilSteps guards the runner against a sequence with
// an unset run function rather than panicking during shutdown.
func TestRunShutdownSequenceSkipsNilSteps(t *testing.T) {
	lg, _ := recordingLogger(t)

	executed := false
	unfinished := runShutdownSequence(context.Background(), lg, []shutdownStep{
		{name: "noop"},
		{
			name: "real",
			run: func(context.Context) error {
				executed = true
				return nil
			},
		},
	})

	require.Empty(t, unfinished)
	require.True(t, executed)
}

// runStepsRecordingWorkerContext runs the real shutdown sequence and reports,
// for each step, whether the background-worker context was already cancelled
// when that step finished.
//
// Parameters:
//   - t: the running test, failed when a step reports an error.
//   - steps: the sequence to run.
//   - workerCtx: the context the sequence is expected to cancel.
//
// Return values:
//   - map[string]bool: step name -> worker context cancelled after that step.
func runStepsRecordingWorkerContext(
	t *testing.T,
	steps []shutdownStep,
	workerCtx context.Context,
) map[string]bool {
	t.Helper()

	cancelled := make(map[string]bool, len(steps))
	for _, step := range steps {
		require.NotNil(t, step.run, "step %q has no run function", step.name)
		require.NoError(t, step.run(context.Background()), "step %q failed", step.name)
		cancelled[step.name] = workerCtx.Err() != nil
	}
	return cancelled
}

// TestShutdownSequenceStopsBackgroundWorkersBeforeSinksAndDatabase verifies the
// periodic background workers are cancelled by the sequence itself, at a point
// where the trace sinks and the database handles are still open.
//
// Before this step existed, main started the retention cleaners with
// context.Background(): their tickers kept firing during and after shutdown, so
// a sweep could issue DELETEs through a pool CloseDB had already closed.
func TestShutdownSequenceStopsBackgroundWorkersBeforeSinksAndDatabase(t *testing.T) {
	workerCtx, stopWorkers := context.WithCancel(context.Background())
	defer stopWorkers()

	steps := newShutdownSequence(nil, nil, nil, false, stopWorkers)
	cancelled := runStepsRecordingWorkerContext(t, steps, workerCtx)

	require.False(t, cancelled["http_server"],
		"background workers must keep running while in-flight HTTP requests drain")
	require.True(t, cancelled["retention_workers"],
		"the retention_workers step must cancel the background worker context")
	require.True(t, cancelled["trace_sinks"],
		"no producer may still be running when the consuming sinks close")
	require.True(t, cancelled["database"],
		"no producer may still be running when the database handles close")
}

// TestRetentionStepJoinsAllDatabaseBackgroundWorkers verifies cancellation is
// followed by a join for periodic producers beyond retention cleaners.
//
// Parameters:
//   - t: the running test.
//
// Return values: none.
func TestRetentionStepJoinsAllDatabaseBackgroundWorkers(t *testing.T) {
	workerCtx, stopWorkers := context.WithCancel(context.Background())
	defer stopWorkers()

	finished := make(chan struct{})
	model.StartBackgroundWorker(workerCtx, func(ctx context.Context) {
		<-ctx.Done()
		time.Sleep(50 * time.Millisecond)
		close(finished)
	})

	steps := newShutdownSequence(nil, nil, nil, false, stopWorkers)
	retention := steps[indexOf(t, stepNames(steps), "retention_workers")]
	require.NoError(t, retention.run(context.Background()))

	select {
	case <-finished:
	default:
		t.Fatal("retention step returned before a periodic database producer stopped")
	}
}

// TestShutdownSequenceToleratesMissingWorkerCanceller verifies the step is safe
// when no background workers were started, which is how the sequence is built in
// the ordering tests and in any embedder that starts none.
func TestShutdownSequenceToleratesMissingWorkerCanceller(t *testing.T) {
	lg, messages := recordingLogger(t)

	unfinished := runShutdownSequence(context.Background(), lg,
		newShutdownSequence(nil, nil, nil, false, nil))

	require.Empty(t, unfinished)
	require.Contains(t, messages(), "graceful shutdown complete")
}

// TestRetentionWorkersStepReportsUnfinishedWorkOnDeadline verifies unfinished
// retention work reaches the same deadline report every other shutdown step
// uses, rather than being dropped because the workers were merely cancelled and
// never joined.
func TestRetentionWorkersStepReportsUnfinishedWorkOnDeadline(t *testing.T) {
	lg, messages := recordingLogger(t)

	// A cleaner that is still sweeping when the deadline expires: the step's
	// join cannot complete, so the sequence must name it as unfinished work.
	release := make(chan struct{})
	defer close(release)

	expired, cancel := context.WithDeadline(context.Background(), time.Now().UTC().Add(-time.Second))
	defer cancel()

	unfinished := runShutdownSequence(expired, lg, []shutdownStep{
		{
			name:       "retention_workers",
			failureLog: "retention workers did not stop before the deadline",
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return errors.Wrap(ctx.Err(), "stop retention workers")
			},
		},
	})

	require.Equal(t, []string{"retention_workers"}, unfinished)
	require.Contains(t, messages(), "retention workers did not stop before the deadline")
	require.Contains(t, messages(), "graceful shutdown deadline expired with unfinished work")
}

// TestDatabaseShutdownStepDoesNotCloseAfterDeadline verifies a timed-out
// producer or sink cannot be raced by closing its database underneath it.
//
// Parameters:
//   - t: the running test.
//
// Return values: none.
func TestDatabaseShutdownStepDoesNotCloseAfterDeadline(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/shutdown-deadline.db"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)

	previousDB, previousLogDB := model.DB, model.LOG_DB
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		_ = sqlDB.Close()
	})

	expired, cancel := context.WithDeadline(context.Background(), time.Now().UTC().Add(-time.Second))
	defer cancel()

	steps := newShutdownSequence(nil, nil, nil, false, nil)
	database := steps[len(steps)-1]
	require.Equal(t, "database", database.name)

	err = database.run(expired)
	require.ErrorIs(t, err, context.DeadlineExceeded,
		"the close must report that it was skipped because the shutdown deadline expired")
	require.NoError(t, sqlDB.Ping(),
		"the process must leave cleanup to process exit instead of closing a database under unfinished workers")
}
