package model

import (
	"context"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/logger"
)

// StartTraceRetentionCleaner launches a background worker that removes expired trace records according to the configured retention period.
//
// The worker registers itself with the process retention group, so a shutdown
// can cancel ctx and then join it through WaitForRetentionCleaners before the
// database handles close. Cancellation stops an in-flight sweep at its next
// chunk boundary rather than at the end of an unbounded delete.
//
// Parameters:
//   - ctx: lifecycle scope for the worker and for every sweep it runs;
//     cancellation ends both.
//   - retentionDays: the retention window; values <= 0 start no worker.
//
// Return values: none.
func StartTraceRetentionCleaner(ctx context.Context, retentionDays int) {
	if retentionDays <= 0 {
		logger.Logger.Debug("trace retention disabled", zap.Int("trace_retention_days", retentionDays))
		return
	}

	cleanup := func() {
		stats, err := CleanExpiredTracesStats(ctx, retentionDays)
		if err != nil {
			if stats.Backlog > 0 {
				fields := append(retentionSweepLogFields(stats, retentionDays), zap.Error(err))
				logger.Logger.Warn("trace retention is behind", fields...)
				return
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				logger.Logger.Info("trace retention sweep stopped early", zap.Error(err))
			} else {
				logger.Logger.Warn("trace retention cleanup failed", zap.Error(err))
			}
			return
		}

		fields := retentionSweepLogFields(stats, retentionDays)
		switch {
		case stats.Backlog > 0:
			// Eligible rows survived every pass, so trace arrivals are outrunning
			// the sweeper. W0.8 requires this to be visible rather than silent.
			logger.Logger.Warn("trace retention is behind", fields...)
		case stats.Deleted > 0:
			logger.Logger.Info("deleted expired trace records", fields...)
		default:
			logger.Logger.Debug("trace retention cleanup completed", fields...)
		}
	}

	// Registered before the goroutine starts so a shutdown that begins in the
	// same instant still joins this worker (see retention_workers.go).
	addRetentionWorker()
	go func() {
		defer retentionWorkerDone()
		ticker := time.NewTicker(config.RetentionSweepInterval())
		defer ticker.Stop()
		cleanup()
		for {
			select {
			case <-ctx.Done():
				if err := ctx.Err(); err != nil {
					logger.Logger.Info("trace retention cleaner stopped", zap.Error(err))
				} else {
					logger.Logger.Info("trace retention cleaner stopped")
				}
				return
			case <-ticker.C:
				cleanup()
			}
		}
	}()

	logger.Logger.Info("trace retention cleaner started", zap.Int("trace_retention_days", retentionDays))
}

// CleanExpiredTraces deletes trace records older than the retention window.
//
// It is the context-free wrapper kept for existing callers; new code should use
// CleanExpiredTracesContext so a shutdown can interrupt a long sweep.
//
// Parameters:
//   - retentionDays: the retention window; values <= 0 disable the sweep.
//
// Return values:
//   - int64: rows removed.
//   - error: wrapped failure from the chunk that could not complete.
func CleanExpiredTraces(retentionDays int) (int64, error) {
	return CleanExpiredTracesContext(context.Background(), retentionDays)
}

// CleanExpiredTracesContext deletes expired trace records in bounded chunks.
//
// Parameters:
//   - ctx: cancellation scope; a cancelled sweep returns what it already removed.
//   - retentionDays: the retention window; values <= 0 disable the sweep.
//
// Return values:
//   - int64: rows removed.
//   - error: wrapped failure from the chunk that could not complete.
func CleanExpiredTracesContext(ctx context.Context, retentionDays int) (int64, error) {
	stats, err := CleanExpiredTracesStats(ctx, retentionDays)
	return stats.Deleted, err
}

// CleanExpiredTracesStats deletes expired trace records and reports whether the
// sweep kept up with new arrivals.
//
// The sweep walks traces.created_at with a keyset cursor, so each chunk seeks to
// where the previous one stopped instead of re-scanning the expired prefix; see
// retention_chunk.go. created_at carries its own index, which is what makes both
// the cursor and the backlog probe index-only work.
//
// Parameters:
//   - ctx: cancellation scope; a cancelled sweep returns what it already removed.
//   - retentionDays: the retention window; values <= 0 disable the sweep.
//
// Return values:
//   - ChunkedDeleteStats: rows removed plus the backlog left behind.
//   - error: wrapped failure from the chunk that could not complete.
func CleanExpiredTracesStats(ctx context.Context, retentionDays int) (ChunkedDeleteStats, error) {
	if retentionDays <= 0 {
		return ChunkedDeleteStats{}, nil
	}

	cutoff := time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour).UnixMilli()

	// Measured around the sweep itself rather than around the whole cleanup
	// closure, so the reported throughput is time spent deleting and not time
	// spent formatting log fields.
	started := time.Now()
	stats, err := ChunkedDeleteWithStats(ctx, DB, ChunkedDeleteOptions{
		Table:       "traces",
		Where:       "created_at < ?",
		Args:        []any{cutoff},
		OrderColumn: "created_at",
		Pause:       config.RetentionDeletePause(),
	})
	// Recorded on every path, including the cancelled one: a shutdown that keeps
	// cutting sweeps short is the condition section 8.3 asks operators to see.
	recordRetentionSweep("traces", stats, err, time.Since(started))
	if err != nil {
		return stats, errors.Wrap(err, "sweep expired traces")
	}
	return stats, nil
}
