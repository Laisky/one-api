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
)

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

	sweep := func(localLogger glog.Logger) {
		if retentionDays > 0 {
			if err := deleteExpiredLogFiles(localLogger, retentionDays, logDir); err != nil {
				localLogger.Warn("log retention cleanup failed", zap.Error(err))
			}
		}
		if deleted, err := enforceSizeCeiling(localLogger, logDir, maxTotalBytes); err != nil {
			localLogger.Warn("log size ceiling enforcement failed", zap.Error(err))
		} else if deleted > 0 {
			localLogger.Info("deleted log files to satisfy the size ceiling",
				zap.Int("deleted_files", deleted),
				zap.Int("log_max_total_size_mb", config.LogMaxTotalSizeMB))
		}
	}

	sweep(workerLogger)

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
				sweep(localLogger)
			}
		}
	}(workerLogger)

	workerLogger.Info("log retention cleaner started",
		zap.Int("log_retention_days", retentionDays),
		zap.Int("log_max_total_size_mb", config.LogMaxTotalSizeMB),
		zap.Duration("sweep_interval", config.RetentionSweepInterval()),
		zap.String("log_dir", logDir))
}

// deleteExpiredLogFiles removes log files older than the retention window from the configured log directory.
// The retentionDays parameter defines the age threshold in days, logDir is the directory to scan,
// and the returned error reports failures when listing entries.
func deleteExpiredLogFiles(lg glog.Logger, retentionDays int, logDir string) error {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.Wrap(err, "read log directory")
	}

	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)

	for _, entry := range entries {
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

		lg.Info("deleted expired log file", zap.String("log_path", fullPath), zap.Time("modified_at", modTime))
	}

	return nil
}
