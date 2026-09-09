package logger

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	errors "github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/metrics"
)

// retentionTargetAppLogFiles names the application log FILE sweep in the
// operational retention metrics (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 3 / W3.3).
//
// It is a compile-time constant and deliberately distinct from the database
// retention targets in model.retentionTables: this sweeper removes files from a
// directory, its throughput is measured in files rather than rows, and mixing
// the two under one target name would make the series unreadable.
const retentionTargetAppLogFiles = "app_log_files"

var (
	// retentionWorkerGroup tracks the log retention sweep and the disk-pressure
	// guard, both of which already honor ctx.Done(); the group is what lets a
	// shutdown prove they stopped instead of assuming it.
	retentionWorkerGroup sync.WaitGroup
	// retentionWorkersActive mirrors the group's counter, which sync.WaitGroup
	// does not expose. Without it "nothing to wait for" and "waited and timed
	// out" are indistinguishable when the shutdown deadline has already expired:
	// Wait must run in another goroutine, so the select would pick randomly
	// between an immediately-closed done channel and an already-cancelled
	// context, and a deployment that starts no log workers at all (the
	// zero-configuration standalone default) could report their work as
	// unfinished at random.
	retentionWorkersActive atomic.Int64
)

// addRetentionWorker registers one log retention worker before it starts.
//
// Parameters: none.
//
// Return values: none.
func addRetentionWorker() {
	retentionWorkersActive.Add(1)
	retentionWorkerGroup.Add(1)
}

// retentionWorkerDone records that one log retention worker returned.
//
// Parameters: none.
//
// Return values: none.
func retentionWorkerDone() {
	retentionWorkerGroup.Done()
	retentionWorkersActive.Add(-1)
}

// WaitForRetentionWorkers blocks until the log retention sweep and the disk
// pressure guard have returned, or until ctx expires.
//
// It does NOT cancel anything: the caller owns the workers' context and must
// cancel it first. The returned error carries ctx's cause so the shutdown
// sequence can attribute the unfinished work to the expired deadline (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 1 / W1, row
// "Shutdown": "deadlines report unfinished work").
//
// Parameters:
//   - ctx: the shutdown deadline; a nil context is treated as background.
//
// Return values:
//   - error: wrapped ctx error when a worker was still running at the deadline;
//     nil when every worker had returned.
func WaitForRetentionWorkers(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if retentionWorkersActive.Load() == 0 {
		return nil
	}

	done := make(chan struct{})
	// sync.WaitGroup has no cancellable Wait, so a timed-out call abandons this
	// goroutine. That is bounded: it ends as soon as the workers do, and the
	// process is already shutting down.
	go func() {
		retentionWorkerGroup.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		// A worker that returned in the same instant the deadline expired is
		// finished work, not unfinished work; give the join that last look
		// before reporting.
		select {
		case <-done:
			return nil
		default:
			return errors.Wrap(ctx.Err(), "wait for log retention workers")
		}
	}
}

// StartLogRetentionCleaner launches the background workers that bound the log
// directory.
//
// It starts TWO independent loops, because the work they do has two different
// urgencies (W0.3):
//
//   - the slow retention sweep, at RETENTION_SWEEP_INTERVAL_MINUTES, deletes
//     data that has expired by age (LOG_RETENTION_DAYS) or by directory budget
//     (LOG_MAX_TOTAL_SIZE_MB). Expiry is inherently slow work: nothing becomes
//     eligible faster than the clock;
//   - the fast pressure guard, at LOG_DISK_CHECK_INTERVAL_SEC, watches the
//     free-space floor (LOG_MIN_FREE_DISK_MB) and the active-file ceiling
//     (LOG_MAX_ACTIVE_FILE_SIZE_MB). Disk exhaustion is a survival signal, not
//     an expiry: at a measured 16 MB/s a 1 GB reserve lasts about 62 seconds,
//     so a guard sharing the sweep cadence -- 24 hours on the standalone
//     profile -- could not protect it, which is exactly what shipped.
//
// Each loop starts only when something it enforces is configured. The
// standalone profile enables the active-file ceiling by default, so it starts
// the pressure guard; explicitly disabling every limit starts no worker.
//
// Parameters:
//   - ctx: lifecycle scope for both workers.
//   - retentionDays: age threshold in days; 0 disables the age limit.
//   - logDir: the directory holding log files.
//
// Return values: none.
func StartLogRetentionCleaner(ctx context.Context, retentionDays int, logDir string) {
	workerLogger := Logger.With(zap.String("component", "log-retention"))

	if config.AppLogSink == config.AppLogSinkStdout {
		workerLogger.Debug("log retention skipped: APP_LOG_SINK=stdout writes no files")
		return
	}

	maxTotalBytes := config.LogMaxTotalSizeBytes()
	minFreeBytes := config.LogMinFreeDiskBytes()
	// A profile-provided active-file ceiling is inert in single-file mode: the
	// writer has no rotation mechanism, and turning that compatibility case into
	// permanent emergency throttling would be a new user-visible failure. An
	// explicitly paired ceiling is rejected during configuration validation.
	maxActiveBytes := activeFileCeilingBytes()

	if retentionDays <= 0 && maxTotalBytes <= 0 && minFreeBytes <= 0 && maxActiveBytes <= 0 {
		// This is the default for the standalone profile, and it is deliberate:
		// an upgrade must not delete log files an operator chose to keep. Say
		// so once, loudly enough that the operator can opt in, and move on.
		workerLogger.Warn("log file retention is disabled: no age limit, no size ceiling, "+
			"and no free-disk guard, so the log directory grows without bound",
			zap.String("log_dir", logDir),
			zap.String("enable_with", "LOG_RETENTION_DAYS, LOG_MAX_TOTAL_SIZE_MB, LOG_MIN_FREE_DISK_MB, LOG_MAX_ACTIVE_FILE_SIZE_MB"),
			zap.String("or_set", "OBSERVABILITY_PROFILE=scaled"))
		return
	}

	if strings.TrimSpace(logDir) == "" {
		workerLogger.Warn("log retention enabled but log directory is empty", zap.Int("log_retention_days", retentionDays))
		return
	}

	startRetentionSweep(ctx, workerLogger, retentionDays, logDir, maxTotalBytes)
	startDiskPressureGuard(ctx, workerLogger, logDir, minFreeBytes, maxActiveBytes)
}

// startRetentionSweep runs the slow expiry loop.
//
// Parameters:
//   - ctx: lifecycle scope for the worker.
//   - workerLogger: the logger the worker keeps; the package-level Logger must
//     not be read from the worker goroutine, which would race
//     SetupEnhancedLogger.
//   - retentionDays: age threshold in days; 0 disables the age limit.
//   - logDir: the directory holding log files.
//   - maxTotalBytes: the directory budget; values <= 0 disable it.
//
// Return values: none.
func startRetentionSweep(ctx context.Context, workerLogger glog.Logger, retentionDays int, logDir string, maxTotalBytes int64) {
	if retentionDays <= 0 && maxTotalBytes <= 0 {
		return
	}

	runLogRetentionSweep(ctx, workerLogger, retentionDays, logDir, maxTotalBytes)

	ticker := time.NewTicker(config.RetentionSweepInterval())
	addRetentionWorker()

	go func(localLogger glog.Logger) {
		defer ticker.Stop()
		defer retentionWorkerDone()
		for {
			select {
			case <-ctx.Done():
				localLogger.Info("log retention cleaner stopped", zap.Error(ctx.Err()))
				return
			case <-ticker.C:
				runLogRetentionSweep(ctx, localLogger, retentionDays, logDir, maxTotalBytes)
			}
		}
	}(workerLogger)

	workerLogger.Info("log retention cleaner started",
		zap.Int("log_retention_days", retentionDays),
		zap.Int("log_max_total_size_mb", config.LogMaxTotalSizeMB),
		zap.Duration("sweep_interval", config.RetentionSweepInterval()),
		zap.String("log_dir", logDir))
}

// runLogRetentionSweep performs one expiry-and-budget pass over the log
// directory and reports its throughput.
//
// It is a named function rather than the closure it replaced so a test can drive
// exactly one sweep, and so the metric has a single completion point covering
// both halves of the work: age expiry and the directory budget are one sweep of
// one target, not two.
//
// The result label follows the same rule as the database sweeps
// (model.retentionSweepResult): a real failure is `failed`, a sweep the shutdown
// cut short at a file boundary is `canceled`, and only a sweep that examined
// everything it was asked to is `completed`. A genuine failure outranks a
// cancellation because a cancellation is routine at shutdown while a failure is
// not, and reporting the routine one would hide the actionable one.
//
// Parameters:
//   - ctx: lifecycle scope; cancellation stops the sweep at a file boundary.
//   - lg: the worker's logger; the package-level Logger must not be read from
//     the worker goroutine.
//   - retentionDays: age threshold in days; values <= 0 skip the expiry pass.
//   - logDir: the directory holding log files.
//   - maxTotalBytes: the directory budget; values <= 0 skip the budget pass.
//
// Return values: none; failures are logged because retention is best-effort.
func runLogRetentionSweep(ctx context.Context, lg glog.Logger, retentionDays int, logDir string, maxTotalBytes int64) {
	started := time.Now()
	var (
		removed  int
		failed   error
		canceled error
	)

	if retentionDays > 0 {
		deleted, err := deleteExpiredLogFiles(ctx, lg, retentionDays, logDir)
		removed += deleted
		switch {
		case err == nil:
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			canceled = err
		default:
			failed = err
			lg.Warn("log retention cleanup failed", zap.Error(err))
		}
	}

	// The budget pass is skipped once the context is done: it deletes files
	// oldest-first with no cancellation point of its own, and a shutdown asked
	// this worker to stop.
	if err := ctx.Err(); err != nil {
		canceled = err
	} else if deleted, err := enforceSizeCeiling(lg, logDir, maxTotalBytes); err != nil {
		failed = err
		lg.Warn("log size ceiling enforcement failed", zap.Error(err))
	} else {
		removed += deleted
		if deleted > 0 {
			lg.Info("deleted log files to satisfy the size ceiling",
				zap.Int("deleted_files", deleted),
				zap.Int("log_max_total_size_mb", config.LogMaxTotalSizeMB))
		}
	}

	result := metrics.RetentionResultCompleted
	switch {
	case failed != nil:
		result = metrics.RetentionResultFailed
	case canceled != nil:
		result = metrics.RetentionResultCanceled
	}
	metrics.RecordRetentionSweep(retentionTargetAppLogFiles, result,
		int64(removed), time.Since(started))
}

// deleteExpiredLogFiles removes log files older than the retention window from
// the configured log directory.
//
// Parameters:
//   - ctx: cancellation scope; a cancelled sweep stops at the next file and
//     reports what it already removed, mirroring the chunk-boundary contract of
//     the database sweeps.
//   - lg: the worker's logger.
//   - retentionDays: the age threshold in days.
//   - logDir: the directory to scan.
//
// Return values:
//   - int: how many files were removed.
//   - error: wrapped failure when the directory cannot be listed, or the wrapped
//     context error when the sweep was cancelled part-way.
func deleteExpiredLogFiles(ctx context.Context, lg glog.Logger, retentionDays int, logDir string) (int, error) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, errors.Wrap(err, "read log directory")
	}

	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)
	deleted := 0

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return deleted, errors.Wrapf(err,
				"log retention sweep stopped after %d files", deleted)
		}
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !isLogFileName(name) {
			continue
		}

		info, infoErr := entry.Info()
		if infoErr != nil {
			lg.Warn("skip log file without metadata", zap.String("log_path", filepath.Join(logDir, name)), zap.Error(infoErr))
			continue
		}

		modTime := info.ModTime().UTC()
		if !modTime.Before(cutoff) {
			continue
		}

		fullPath := filepath.Join(logDir, name)
		if removeErr := os.Remove(fullPath); removeErr != nil {
			lg.Warn("failed to delete expired log file", zap.String("log_path", fullPath), zap.Error(removeErr))
			continue
		}

		deleted++
		lg.Info("deleted expired log file", zap.String("log_path", fullPath), zap.Time("modified_at", modTime))
	}

	return deleted, nil
}
