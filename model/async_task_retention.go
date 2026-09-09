package model

import (
	"context"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/logger"
)

// Async task retention needs its own plan (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 0 / W0.8:
// "async-task predicates need their own plan").
//
// The predicate this sweep replaces was:
//
//	CASE WHEN last_accessed_at > 0 THEN last_accessed_at ELSE created_at END < ?
//
// That expression is correct and unusable. A CASE over two columns is not
// sargable: no index can be probed with it, so every engine evaluates it row by
// row. Chunking made that worse rather than better -- each chunk full-scanned
// the table to find its next 5000 victims, so a sweep of N eligible rows in
// batches of B cost O(N/B) FULL SCANS. On the largest deployments that is the
// difference between a sweep that finishes and one that never does.
//
// The rewrite splits the CASE into the two disjoint, individually sargable row
// sets it actually describes, and sweeps each with its own cursor:
//
//	pass 1 (touched):   last_accessed_at > 0 AND last_accessed_at < cutoff
//	pass 2 (untouched): (last_accessed_at IS NULL OR last_accessed_at <= 0)
//	                    AND created_at < cutoff
//
// The union is exactly the old row set, including the NULL case: the CASE
// predicate takes its ELSE branch whenever `last_accessed_at > 0` is not TRUE,
// which is both `<= 0` and NULL. TestAsyncTaskRetentionPredicateMatchesLegacy
// pins that equivalence against the original expression.
//
// Execution plan, on the (last_accessed_at, created_at) index declared in
// async_task.go:
//
//   - Pass 1 is a pure range scan on the leading column, bounded on both sides
//     (`> 0` and `< cutoff`), and the keyset cursor walks that same column, so
//     each chunk is an index seek rather than a scan from the start.
//   - Pass 2 restricts the leading column to the never-accessed values and
//     ranges created_at, which is its cursor. Where the planner can union the
//     two leading-column ranges (MySQL's range optimizer, PostgreSQL's BitmapOr)
//     it uses the same composite index; SQLite instead seeks the created_at
//     index and rechecks last_accessed_at per row, which is still a bounded seek
//     on the cursor column -- verified with EXPLAIN QUERY PLAN in
//     TestAsyncTaskRetentionPredicatesAreSargable. Either way the whole pass
//     costs one traversal of the remaining created_at range per sweep, not one
//     per chunk, and it runs after pass 1 has already removed everything that
//     aged out by last access.
//
// Rows reach the pass 2 predicate only from before SaveAsyncTaskBinding started
// stamping last_accessed_at, or from the AutoMigrate that added the column and
// left existing rows NULL, so the pass is normally empty.
//
// Sweeping in two passes rather than one OR'd predicate is deliberate: it gives
// each row set the cursor its own index supports. One combined predicate would
// need a CASE cursor again, and would be back where it started.
const (
	// asyncTaskTouchedPredicate selects bindings that were accessed at least
	// once and whose last access has aged out.
	asyncTaskTouchedPredicate = "last_accessed_at > 0 AND last_accessed_at < ?"
	// asyncTaskUntouchedPredicate selects bindings that were never accessed
	// (including rows written before last_accessed_at was populated) and whose
	// creation has aged out.
	asyncTaskUntouchedPredicate = "(last_accessed_at IS NULL OR last_accessed_at <= 0) AND created_at < ?"
)

// StartAsyncTaskRetentionCleaner launches a background worker that removes async task bindings older than the configured retention window.
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
func StartAsyncTaskRetentionCleaner(ctx context.Context, retentionDays int) {
	if retentionDays <= 0 {
		logger.Logger.Debug("async task retention disabled", zap.Int("async_task_retention_days", retentionDays))
		return
	}

	cleanup := func() {
		stats, err := CleanExpiredAsyncTaskBindingsStats(ctx, retentionDays)
		if err != nil {
			if stats.Backlog > 0 {
				fields := append(retentionSweepLogFields(stats, retentionDays), zap.Error(err))
				logger.Logger.Warn("async task retention is behind", fields...)
				return
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				logger.Logger.Info("async task retention sweep stopped early", zap.Error(err))
			} else {
				logger.Logger.Warn("async task retention cleanup failed", zap.Error(err))
			}
			return
		}

		fields := retentionSweepLogFields(stats, retentionDays)
		switch {
		case stats.Backlog > 0:
			// The sweep exhausted its passes with eligible rows still present:
			// arrivals are outrunning deletion, which is the condition W0.8
			// asks operators to be able to see.
			logger.Logger.Warn("async task retention is behind", fields...)
		case stats.Deleted > 0:
			logger.Logger.Info("deleted expired async task bindings", fields...)
		default:
			logger.Logger.Debug("async task retention sweep completed", fields...)
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

// retentionSweepLogFields renders sweep accounting for the workers' logs.
//
// Parameters:
//   - stats: the sweep result.
//   - retentionDays: the configured window, echoed for context.
//
// Return values:
//   - []zap.Field: deleted rows, work performed, and the keeping-up measures.
func retentionSweepLogFields(stats ChunkedDeleteStats, retentionDays int) []zap.Field {
	fields := []zap.Field{
		zap.Int64("deleted_rows", stats.Deleted),
		zap.Int("chunks", stats.Chunks),
		zap.Int("passes", stats.Passes),
		zap.Int64("backlog_rows", stats.Backlog),
		zap.Bool("backlog_truncated", stats.BacklogTruncated),
		zap.Int("retention_days", retentionDays),
	}
	if age := stats.OldestEligibleAge(time.Now().UTC()); age > 0 {
		fields = append(fields, zap.Duration("oldest_eligible_age", age))
	}
	return fields
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
	stats, err := CleanExpiredAsyncTaskBindingsStats(ctx, retentionDays)
	return stats.Deleted, err
}

// CleanExpiredAsyncTaskBindingsStats deletes expired bindings and reports
// whether the sweep kept up.
//
// The two passes are described at the top of this file: they partition the old
// CASE predicate into sargable halves so each can use the
// (last_accessed_at, created_at) index and its own keyset cursor.
//
// Parameters:
//   - ctx: cancellation scope; a cancelled sweep returns what it already removed.
//   - retentionDays: the retention window; values <= 0 disable the sweep.
//
// Return values:
//   - ChunkedDeleteStats: combined statistics of both passes, including the
//     backlog left behind.
//   - error: wrapped failure from the pass that could not complete.
func CleanExpiredAsyncTaskBindingsStats(ctx context.Context, retentionDays int) (ChunkedDeleteStats, error) {
	var stats ChunkedDeleteStats
	if retentionDays <= 0 {
		return stats, nil
	}

	cutoff := time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour).UnixMilli()
	pause := config.RetentionDeletePause()
	// The two passes are one sweep of one table, so they produce ONE metric
	// sample carrying their merged rows and their combined elapsed time. Counting
	// them separately would double the sweep rate of this target against every
	// other one.
	started := time.Now()

	touched, err := ChunkedDeleteWithStats(ctx, DB, ChunkedDeleteOptions{
		Table:       "async_task_bindings",
		Where:       asyncTaskTouchedPredicate,
		Args:        []any{cutoff},
		OrderColumn: "last_accessed_at",
		Pause:       pause,
	})
	stats = stats.Merge(touched)
	if err != nil {
		recordRetentionSweep("async_task_bindings", stats, err, time.Since(started))
		return stats, errors.Wrap(err, "sweep accessed async task bindings")
	}

	untouched, err := ChunkedDeleteWithStats(ctx, DB, ChunkedDeleteOptions{
		Table:       "async_task_bindings",
		Where:       asyncTaskUntouchedPredicate,
		Args:        []any{cutoff},
		OrderColumn: "created_at",
		Pause:       pause,
	})
	stats = stats.Merge(untouched)
	recordRetentionSweep("async_task_bindings", stats, err, time.Since(started))
	if err != nil {
		return stats, errors.Wrap(err, "sweep never-accessed async task bindings")
	}

	return stats, nil
}
