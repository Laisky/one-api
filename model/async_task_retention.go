package model

import (
	"context"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/logger"
)

const asyncTaskSweepInterval = 24 * time.Hour

// StartAsyncTaskRetentionCleaner launches a background worker that removes async task bindings older than the configured retention window.
func StartAsyncTaskRetentionCleaner(ctx context.Context, retentionDays int) {
	if retentionDays <= 0 {
		logger.Logger.Debug("async task retention disabled", zap.Int("async_task_retention_days", retentionDays))
		return
	}

	cleanup := func() {
		deleted, err := CleanExpiredAsyncTaskBindings(retentionDays)
		if err != nil {
			logger.Logger.Warn("async task retention cleanup failed", zap.Error(err))
			return
		}
		if deleted > 0 {
			logger.Logger.Info("deleted expired async task bindings", zap.Int64("deleted_rows", deleted), zap.Int("async_task_retention_days", retentionDays))
		} else {
			logger.Logger.Debug("async task retention sweep completed", zap.Int("async_task_retention_days", retentionDays))
		}
	}

	cleanup()

	ticker := time.NewTicker(asyncTaskSweepInterval)
	go func() {
		defer ticker.Stop()
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

// CleanExpiredAsyncTaskBindings deletes task bindings whose last access (or creation when never accessed) exceeds the retention window.
func CleanExpiredAsyncTaskBindings(retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		return 0, nil
	}

	cutoff := time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour).UnixMilli()
	condition := "CASE WHEN last_accessed_at > 0 THEN last_accessed_at ELSE created_at END < ?"

	tx := DB.Where(condition, cutoff).Delete(&AsyncTaskBinding{})
	if tx.Error != nil {
		return 0, errors.Wrap(tx.Error, "delete expired async task bindings")
	}
	return tx.RowsAffected, nil
}
