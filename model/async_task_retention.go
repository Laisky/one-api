package model

import (
	"context"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/logger"
)

// StartAsyncTaskRetentionCleaner launches a background worker that removes async task bindings older than the configured retention window.
func StartAsyncTaskRetentionCleaner(ctx context.Context, retentionDays int) {
	if retentionDays <= 0 {
		logger.Logger.Debug("async task retention disabled", zap.Int("async_task_retention_days", retentionDays))
		return
	}

	cleanup := func() {
		deleted, err := CleanExpiredAsyncTaskBindingsContext(ctx, retentionDays)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				logger.Logger.Info("async task retention sweep stopped early", zap.Error(err))
			} else {
				logger.Logger.Warn("async task retention cleanup failed", zap.Error(err))
			}
			return
		}
		if deleted > 0 {
			logger.Logger.Info("deleted expired async task bindings", zap.Int64("deleted_rows", deleted), zap.Int("async_task_retention_days", retentionDays))
		} else {
			logger.Logger.Debug("async task retention sweep completed", zap.Int("async_task_retention_days", retentionDays))
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
					logger.Logger.Info("async task retention cleaner stopped", zap.Error(err))
				} else {
					logger.Logger.Info("async task retention cleaner stopped")
				}
				return
			case <-ticker.C:
				cleanup()
			}
		}
	}()

	logger.Logger.Info("async task retention cleaner started", zap.Int("async_task_retention_days", retentionDays))
}

// CleanExpiredAsyncTaskBindings deletes task bindings whose last access (or
// creation when never accessed) exceeds the retention window.
//
// Parameters:
//   - retentionDays: the retention window; values <= 0 disable the sweep.
//
// Return values:
//   - int64: rows removed.
//   - error: wrapped failure from the chunk that could not complete.
func CleanExpiredAsyncTaskBindings(retentionDays int) (int64, error) {
	return CleanExpiredAsyncTaskBindingsContext(context.Background(), retentionDays)
}

// CleanExpiredAsyncTaskBindingsContext deletes expired bindings in bounded chunks.
//
// Parameters:
//   - ctx: cancellation scope; a cancelled sweep returns what it already removed.
//   - retentionDays: the retention window; values <= 0 disable the sweep.
//
// Return values:
//   - int64: rows removed.
//   - error: wrapped failure from the chunk that could not complete.
func CleanExpiredAsyncTaskBindingsContext(ctx context.Context, retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		return 0, nil
	}

	cutoff := time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour).UnixMilli()

	return ChunkedDelete(ctx, DB, ChunkedDeleteOptions{
		Table: "async_task_bindings",
		Where: "CASE WHEN last_accessed_at > 0 THEN last_accessed_at ELSE created_at END < ?",
		Args:  []any{cutoff},
		Pause: config.RetentionDeletePause(),
	})
}
