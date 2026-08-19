package model

import (
	"context"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"

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
	if ctx == nil {
		ctx = context.Background()
	}

	if !common.UsingSQLite.Load() {
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
	return strings.Contains(msg, "database is locked") || strings.Contains(msg, "database table is locked") || strings.Contains(msg, "database is busy")
}
