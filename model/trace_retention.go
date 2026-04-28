package model

import (
	"context"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/logger"
)

const traceRetentionSweepInterval = 24 * time.Hour

// StartTraceRetentionCleaner launches a background worker that removes expired trace records according to the configured retention period.
func StartTraceRetentionCleaner(ctx context.Context, retentionDays int) {
	if retentionDays <= 0 {
		logger.Logger.Debug("trace retention disabled", zap.Int("trace_retention_days", retentionDays))
		return
	}

	cleanup := func() {
		deleted, err := CleanExpiredTraces(retentionDays)
		if err != nil {
			logger.Logger.Warn("trace retention cleanup failed", zap.Error(err))
			return
		}

		if deleted > 0 {
			logger.Logger.Info("deleted expired trace records", zap.Int64("deleted_rows", deleted), zap.Int("trace_retention_days", retentionDays))
		} else {
			logger.Logger.Debug("trace retention cleanup completed", zap.Int("trace_retention_days", retentionDays))
		}
	}

	cleanup()

	ticker := time.NewTicker(traceRetentionSweepInterval)

	go func() {
		defer ticker.Stop()
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

// CleanExpiredTraces deletes trace records whose creation time is older than the configured retentionDays window.
func CleanExpiredTraces(retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		return 0, nil
	}

	cutoff := time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour).UnixMilli()

	tx := DB.Where("created_at < ?", cutoff).Delete(&Trace{})
	if tx.Error != nil {
		return 0, errors.Wrap(tx.Error, "delete expired trace records")
	}

	return tx.RowsAffected, nil
}
