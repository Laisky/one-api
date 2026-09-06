package logger

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	errors "github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/config"
)

var retentionWorkerGroup sync.WaitGroup

// StartLogRetentionCleaner launches a background worker that bounds the log
// directory.
//
// The worker enforces three independent limits, any one of which is enough to
// start it: an age limit (LOG_RETENTION_DAYS), a total size ceiling
// (LOG_MAX_TOTAL_SIZE_MB), and a free-space floor on the log volume
// (LOG_MIN_FREE_DISK_MB). It runs immediately and then every
// RETENTION_SWEEP_INTERVAL_MINUTES until ctx is cancelled.
//
// Parameters:
//   - ctx: lifecycle scope for the worker.
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

	maxTotalBytes := int64(config.LogMaxTotalSizeMB) * bytesPerMiB
	minFreeBytes := int64(config.LogMinFreeDiskMB) * bytesPerMiB

	if retentionDays <= 0 && maxTotalBytes <= 0 && minFreeBytes <= 0 {
		// This is the default for the standalone profile, and it is deliberate:
		// an upgrade must not delete log files an operator chose to keep. Say
		// so once, loudly enough that the operator can opt in, and move on.
		workerLogger.Warn("log file retention is disabled: no age limit, no size ceiling, "+
			"and no free-disk guard, so the log directory grows without bound",
			zap.String("log_dir", logDir),
			zap.String("enable_with", "LOG_RETENTION_DAYS, LOG_MAX_TOTAL_SIZE_MB, LOG_MIN_FREE_DISK_MB"),
			zap.String("or_set", "OBSERVABILITY_PROFILE=scaled"))
		return
	}

	if strings.TrimSpace(logDir) == "" {
		workerLogger.Warn("log retention enabled but log directory is empty", zap.Int("log_retention_days", retentionDays))
		return
	}

	cleanup := func(localLogger glog.Logger) {
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
		if deleted, err := enforceFreeDiskFloor(localLogger, logDir, minFreeBytes); err != nil {
			localLogger.Warn("free disk guard failed", zap.Error(err))
		} else if deleted > 0 {
			localLogger.Info("deleted log files to restore free disk space",
				zap.Int("deleted_files", deleted),
				zap.Int("log_min_free_disk_mb", config.LogMinFreeDiskMB))
		}
	}

	cleanup(workerLogger)

	ticker := time.NewTicker(config.RetentionSweepInterval())

	retentionWorkerGroup.Add(1)

	go func(localLogger glog.Logger) {
		defer ticker.Stop()
		defer retentionWorkerGroup.Done()
		for {
			select {
			case <-ctx.Done():
				localLogger.Info("log retention cleaner stopped", zap.Error(ctx.Err()))
				return
			case <-ticker.C:
				cleanup(localLogger)
			}
		}
	}(workerLogger)

	workerLogger.Info("log retention cleaner started",
		zap.Int("log_retention_days", retentionDays),
		zap.Int("log_max_total_size_mb", config.LogMaxTotalSizeMB),
		zap.Int("log_min_free_disk_mb", config.LogMinFreeDiskMB),
		zap.Duration("sweep_interval", config.RetentionSweepInterval()),
		zap.String("log_dir", logDir))
}

// bytesPerMiB converts the megabyte-denominated configuration knobs into bytes.
const bytesPerMiB = 1 << 20

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
