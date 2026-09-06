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
func StartTraceRetentionCleaner(ctx context.Context, retentionDays int) {
	if retentionDays <= 0 {
		logger.Logger.Debug("trace retention disabled", zap.Int("trace_retention_days", retentionDays))
		return
	}

	cleanup := func() {
		deleted, err := CleanExpiredTracesContext(ctx, retentionDays)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				logger.Logger.Info("trace retention sweep stopped early", zap.Error(err))
			} else {
				logger.Logger.Warn("trace retention cleanup failed", zap.Error(err))
			}
			return
		}

		if deleted > 0 {
			logger.Logger.Info("deleted expired trace records", zap.Int64("deleted_rows", deleted), zap.Int("trace_retention_days", retentionDays))
		} else {
			logger.Logger.Debug("trace retention cleanup completed", zap.Int("trace_retention_days", retentionDays))
		}
	}

	go func() {
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
	if retentionDays <= 0 {
		return 0, nil
	}

	cutoff := time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour).UnixMilli()

	return ChunkedDelete(ctx, DB, ChunkedDeleteOptions{
		Table: "traces",
		Where: "created_at < ?",
		Args:  []any{cutoff},
		Pause: config.RetentionDeletePause(),
	})
}
