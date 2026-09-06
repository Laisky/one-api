package model

import (
	"context"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
)

const (
	sqliteBusyRetryAttempts  = 5
	sqliteBusyRetryBaseDelay = 20 * time.Millisecond
)

// runWithSQLiteBusyRetry executes operation and retries when SQLite reports a busy/locked database.
// The retry loop only triggers when SQLite is the active backend and the error message indicates a lock.
// ctx may be nil; in that case context.Background() is used.
func runWithSQLiteBusyRetry(ctx context.Context, operation func() error) error {
	return runWithSQLiteBusyRetryEnabled(ctx, common.UsingSQLite.Load(), operation)
}

// runWithSQLiteBusyRetryForDB executes operation with SQLite busy retries when
// db itself uses the SQLite dialect. It avoids process-global dialect flags for
// code paths that already own a concrete database handle.
//
// Parameters:
//   - ctx: cancellation scope for retry backoff; nil uses context.Background().
//   - db: database handle whose dialect controls retry behavior.
//   - operation: database operation to execute.
//
// Return values:
//   - error: nil on success, otherwise the operation or retry-exhaustion error.
func runWithSQLiteBusyRetryForDB(ctx context.Context, db *gorm.DB, operation func() error) error {
	isSQLite := db != nil && db.Dialector != nil && db.Dialector.Name() == "sqlite"
	return runWithSQLiteBusyRetryEnabled(ctx, isSQLite, operation)
}

// runWithSQLiteBusyRetryEnabled contains the shared bounded retry loop.
//
// Parameters:
//   - ctx: cancellation scope for retry backoff; nil uses context.Background().
//   - enabled: whether SQLite-specific busy errors should be retried.
//   - operation: database operation to execute.
//
// Return values:
//   - error: nil on success, otherwise the operation or retry-exhaustion error.
func runWithSQLiteBusyRetryEnabled(ctx context.Context, enabled bool, operation func() error) error {
	if ctx == nil {
		ctx = context.Background()
	}

	if !enabled {
		return operation()
	}

	var lastErr error
	for attempt := 0; attempt <= sqliteBusyRetryAttempts; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt) * sqliteBusyRetryBaseDelay
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return errors.Wrap(lastErr, "context canceled while waiting for SQLite lock")
			case <-timer.C:
			}
		}

		lastErr = operation()
		if lastErr == nil || !shouldRetrySQLiteBusy(lastErr) {
			return lastErr
		}
	}

	return errors.Wrap(lastErr, "SQLite remained busy after retries")
}

// shouldRetrySQLiteBusy inspects error messages returned by the SQLite driver to decide whether a retry is warranted.
func shouldRetrySQLiteBusy(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "database is locked") ||
		strings.Contains(msg, "database table is locked") ||
		strings.Contains(msg, "database schema is locked") ||
		strings.Contains(msg, "database is busy")
}
