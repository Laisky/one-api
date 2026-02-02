package model

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/Laisky/zap"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/graceful"
	"github.com/songquanpeng/one-api/common/logger"
)

const (
	BatchUpdateTypeUserQuota = iota
	BatchUpdateTypeTokenQuota
	BatchUpdateTypeUsedQuota
	BatchUpdateTypeChannelUsedQuota
	BatchUpdateTypeRequestCount
	BatchUpdateTypeCount // if you add a new type, you need to add a new map and a new lock
)

var batchUpdateStores []map[int]int64
var batchUpdateLocks []sync.Mutex

// batchUpdaterStop is used to signal the batch updater goroutine to stop.
// When closed, the updater will perform a final flush and exit.
var batchUpdaterStop chan struct{}

// batchUpdaterDone is closed when the batch updater has completed its final flush.
var batchUpdaterDone chan struct{}

func init() {
	for range BatchUpdateTypeCount {
		batchUpdateStores = append(batchUpdateStores, make(map[int]int64))
		batchUpdateLocks = append(batchUpdateLocks, sync.Mutex{})
	}
}

// InitBatchUpdater starts a background goroutine that periodically flushes accumulated
// quota and statistics changes to the database.
//
// Purpose:
// In high-throughput scenarios, updating the database for every single API request
// creates significant overhead—each request may trigger multiple UPDATE statements
// (user quota, token quota, used quota, request count, channel usage). This can lead to:
//   - Database contention and lock conflicts (especially with SQLite)
//   - Increased latency for API responses
//   - Higher database load and resource consumption
//
// Solution:
// Instead of immediate writes, quota changes are accumulated in memory (batchUpdateStores)
// and flushed to the database at regular intervals (config.BatchUpdateInterval seconds).
// This batching reduces database operations from O(requests) to O(interval), dramatically
// improving throughput.
//
// Trade-offs:
//   - Consistency: There's a small window where in-memory values differ from database.
//     If the server crashes unexpectedly (kill -9, power loss), uncommitted changes are lost.
//   - Latency: Dashboard/UI may show slightly stale quota values until next flush.
//
// Graceful Shutdown:
// The batch updater participates in graceful shutdown via graceful.GoCritical.
// When the server receives SIGTERM/SIGINT, StopBatchUpdater() should be called to:
//  1. Signal the updater loop to stop accepting new cycles
//  2. Perform a final flush of all pending changes
//  3. Complete within the shutdown timeout to avoid data loss
//
// Configuration:
//   - config.BatchUpdateEnabled: Set to true to enable batching
//   - config.BatchUpdateInterval: Seconds between flushes (default typically 5-10s)
//   - config.BatchUpdateTimeoutSec: Maximum time for each flush cycle
func InitBatchUpdater() {
	batchUpdaterStop = make(chan struct{})
	batchUpdaterDone = make(chan struct{})

	go func() {
		defer close(batchUpdaterDone)

		ticker := time.NewTicker(time.Duration(config.BatchUpdateInterval) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-batchUpdaterStop:
				logger.Logger.Info("batch updater received stop signal, performing final flush")
				// Final flush with timeout - this is critical to persist pending changes
				ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.BatchUpdateTimeoutSec)*time.Second)
				batchUpdate(ctx)
				cancel()
				logger.Logger.Info("batch updater final flush completed, exiting")
				return
			case <-ticker.C:
				// Regular periodic flush
				ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.BatchUpdateTimeoutSec)*time.Second)
				batchUpdate(ctx)
				cancel()
			}
		}
	}()
}

// StopBatchUpdater signals the batch updater to stop and performs a final flush.
// This should be called during graceful shutdown to ensure all pending quota changes
// are persisted to the database before the process exits.
//
// The function uses graceful.GoCritical to ensure the shutdown process waits for
// the final flush to complete. This is critical because:
//   - Users may have consumed quota that hasn't been persisted yet
//   - Token quotas may be out of sync with the database
//   - Usage statistics would be inaccurate
//
// Call this function after graceful.SetDraining() but before closing the database.
func StopBatchUpdater(ctx context.Context) {
	lg := logger.FromContext(ctx)
	if batchUpdaterStop == nil {
		// Batch updater was never started
		return
	}

	graceful.GoCritical(ctx, "batchUpdaterFinalFlush", func(_ context.Context) {
		// Signal the updater loop to stop
		close(batchUpdaterStop)

		// Wait for the updater to complete its final flush
		select {
		case <-ctx.Done():
			lg.Warn("batch updater shutdown context expired before final flush completed")
		case <-batchUpdaterDone:
			lg.Info("batch updater shutdown completed successfully")
		}
	})
}

// addNewRecord accumulates a quota/count change for later batch processing.
// It is thread-safe and can be called concurrently from multiple request handlers.
//
// Parameters:
//   - type_: One of BatchUpdateType* constants indicating the update category
//   - id: The user/token/channel ID to update
//   - value: The delta to add (can be negative for decrements)
func addNewRecord(type_ int, id int, value int64) {
	batchUpdateLocks[type_].Lock()
	defer batchUpdateLocks[type_].Unlock()
	if _, ok := batchUpdateStores[type_][id]; !ok {
		batchUpdateStores[type_][id] = value
	} else {
		batchUpdateStores[type_][id] += value
	}
}

// ValidateOrderClause ensures that the sort field and order are valid to prevent SQL injection.
// It returns a sanitized ORDER BY clause string.
func ValidateOrderClause(sortBy, sortOrder string, allowed map[string]string, defaultClause string) string {
	sortBy = strings.ToLower(strings.TrimSpace(sortBy))
	sortOrder = strings.ToLower(strings.TrimSpace(sortOrder))

	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}

	if col, ok := allowed[sortBy]; ok {
		return col + " " + sortOrder
	}

	return defaultClause
}

// batchUpdate flushes all accumulated changes to the database.
// It swaps out the in-memory stores atomically (per type) to allow concurrent
// accumulation while writing to the database.
//
// ctx is used to propagate cancellation/timeout to database operations.
// If the context is canceled or times out, ongoing database operations will be
// interrupted (if the database driver supports context cancellation).
func batchUpdate(ctx context.Context) {
	lg := logger.FromContext(ctx)
	lg.Info("batch update started")
	for i := range BatchUpdateTypeCount {
		// Check context before processing each type
		if ctx.Err() != nil {
			lg.Warn("batch update interrupted by context cancellation",
				zap.Error(ctx.Err()),
				zap.Int("remaining_types", BatchUpdateTypeCount-i))
			return
		}

		batchUpdateLocks[i].Lock()
		store := batchUpdateStores[i]
		batchUpdateStores[i] = make(map[int]int64)
		batchUpdateLocks[i].Unlock()
		// TODO: maybe we can combine updates with same key?
		for key, value := range store {
			// Check context before each database operation
			if ctx.Err() != nil {
				lg.Warn("batch update interrupted during store processing",
					zap.Error(ctx.Err()),
					zap.Int("type", i),
					zap.Int("remaining_items", len(store)))
				return
			}

			switch i {
			case BatchUpdateTypeUserQuota:
				err := increaseUserQuota(ctx, key, value)
				if err != nil {
					lg.Error("failed to batch update user quota",
						zap.Int("user_id", key),
						zap.Int64("value", value),
						zap.Error(err))
				}
			case BatchUpdateTypeTokenQuota:
				err := increaseTokenQuota(ctx, key, value)
				if err != nil {
					lg.Error("failed to batch update token quota",
						zap.Int("token_id", key),
						zap.Int64("value", value),
						zap.Error(err))
				}
			case BatchUpdateTypeUsedQuota:
				updateUserUsedQuota(key, value)
			case BatchUpdateTypeRequestCount:
				updateUserRequestCount(key, int(value))
			case BatchUpdateTypeChannelUsedQuota:
				updateChannelUsedQuota(key, value)
			}
		}
	}
	lg.Info("batch update finished")
}
