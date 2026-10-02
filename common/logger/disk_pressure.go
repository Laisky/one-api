package logger

// Fast disk-pressure guard (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 0 / W0.3).
//
// The shipped guard shared one ticker with the retention sweep, so on the
// standalone profile the free-disk floor was sampled once every 24 hours. That
// cadence cannot protect a reserve measured in seconds: at 16 MB/s of
// application output a 1 GB floor is consumed in about 62 seconds, so the
// guard's first observation of the problem would come long after the volume
// filled.
//
// This loop is the survival half of that split. It does only what has to be
// fast -- read free space, stat the active file, engage or release the bounded
// emergency policy -- and never deletes by age or rebuilds a directory budget,
// which stay on the slow sweep.

import (
	"context"
	"sync/atomic"
	"time"

	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/metrics"
)

// diskPressureChecks counts completed pressure observations.
//
// The guard's whole purpose is a cadence, and a cadence is only provable by
// counting: "did this loop start at all" and "did it sample faster than the
// retention sweep" are otherwise indistinguishable from the outside. One
// relaxed atomic increment every LOG_DISK_CHECK_INTERVAL_SEC is free.
var diskPressureChecks atomic.Int64

// startDiskPressureGuard runs the fast pressure loop.
//
// It does NOT start when neither the free-space floor nor the active-file
// ceiling is configured. This is an explicit opt-out configuration; the
// standalone profile enables the active-file ceiling by default, so it starts
// the guard.
//
// Parameters:
//   - ctx: lifecycle scope for the worker; cancellation stops the loop.
//   - workerLogger: the logger the worker keeps for its whole life. The
//     package-level Logger must not be read from the worker goroutine, which
//     would race SetupEnhancedLogger.
//   - logDir: the directory whose filesystem is inspected.
//   - minFreeBytes: the free-space floor; values <= 0 disable that check.
//   - maxActiveBytes: the active-file ceiling; values <= 0 disable that check.
//
// Return values: none.
func startDiskPressureGuard(ctx context.Context, workerLogger glog.Logger, logDir string, minFreeBytes, maxActiveBytes int64) {
	if minFreeBytes <= 0 && maxActiveBytes <= 0 {
		return
	}

	guardLogger := workerLogger.With(zap.String("component", "log-disk-pressure"))
	interval := config.LogDiskCheckInterval()

	check := func(localLogger glog.Logger) {
		checkDiskPressure(localLogger, logDir, minFreeBytes, maxActiveBytes)
	}

	check(guardLogger)

	ticker := time.NewTicker(interval)
	addRetentionWorker()

	go func(localLogger glog.Logger) {
		defer ticker.Stop()
		defer retentionWorkerDone()
		for {
			select {
			case <-ctx.Done():
				// Leaving the emergency on shutdown keeps the process-wide log
				// level and the disk-pressure gauge from outliving the guard
				// that owns them.
				leaveDiskPressure(localLogger)
				releaseActiveFileCap(localLogger)
				localLogger.Info("log disk pressure guard stopped", zap.Error(ctx.Err()))
				return
			case <-ticker.C:
				check(localLogger)
			}
		}
	}(guardLogger)

	guardLogger.Info("log disk pressure guard started",
		zap.Int("log_min_free_disk_mb", config.LogMinFreeDiskMB),
		zap.Int("log_max_active_file_size_mb", config.LogMaxActiveFileSizeMB),
		zap.Int("log_emergency_max_bytes_per_sec", config.LogEmergencyMaxBytesPerSec),
		zap.Int("log_disk_recovery_margin_pct", config.LogDiskRecoveryMarginPct),
		zap.Duration("check_interval", interval),
		zap.String("log_dir", logDir))
}

// checkDiskPressure performs one pressure observation.
//
// Failures are reported at WARN, not ERROR: a directory that briefly cannot be
// stat'ed is an expected operational condition (an unmounted volume, a racing
// rotation), not a server fault, and the loop retries on the next tick.
//
// Parameters:
//   - lg: the worker's logger.
//   - logDir: the directory whose filesystem is inspected.
//   - minFreeBytes: the free-space floor; values <= 0 disable that check.
//   - maxActiveBytes: the active-file ceiling; values <= 0 disable that check.
//
// Return values: none.
func checkDiskPressure(lg glog.Logger, logDir string, minFreeBytes, maxActiveBytes int64) {
	defer diskPressureChecks.Add(1)
	// The guard can start before the real metrics recorder is installed. Publish
	// every observed state so an already-engaged emergency becomes visible once
	// monitoring is ready, instead of waiting for a state transition.
	defer metrics.UpdateLogDiskPressure(diskEmergency.engaged.Load())

	if err := enforceActiveFileCeiling(lg, logDir, maxActiveBytes); err != nil {
		lg.Warn("active log file ceiling check failed", zap.Error(err))
	}

	if deleted, err := enforceFreeDiskFloor(lg, logDir, minFreeBytes); err != nil {
		lg.Warn("free disk guard failed", zap.Error(err))
	} else if deleted > 0 {
		lg.Info("deleted log files to restore free disk space",
			zap.Int("deleted_files", deleted),
			zap.Int("log_min_free_disk_mb", config.LogMinFreeDiskMB))
	}
}
