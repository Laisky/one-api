package model

import (
	"context"
	"time"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"
)

// channelWriteTimeout caps a channel write that is detached from request cancellation,
// including the time it waits for another transaction's row lock.
var channelWriteTimeout = 30 * time.Second

// detachedChannelWriteContext derives the context for a channel write that must run to
// completion even if the client disconnects. It keeps ctx's values (logging, tracing),
// drops its cancellation, and bounds it by ctx's deadline or channelWriteTimeout,
// whichever is sooner.
//
// Parameters:
//   - ctx: the caller's context.
//
// Return values:
//   - context.Context: the bounded, detached write context.
//   - context.CancelFunc: releases the context's timer; callers must call it.
func detachedChannelWriteContext(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := channelWriteTimeout
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining < timeout {
			timeout = remaining
		}
	}
	return context.WithTimeout(context.WithoutCancel(ctx), timeout)
}

// lockChannelRow takes the channel row's write lock inside tx before the transaction
// reads it. The no-op UPDATE works on SQLite, MySQL, and PostgreSQL without dialect-
// specific SELECT ... FOR UPDATE syntax, and UpdateColumn skips timestamp hooks. A
// concurrent writer of the same row blocks until tx ends, so a read-then-write inside
// tx cannot interleave with it. Do not interpret MySQL's no-op RowsAffected as absence.
//
// Parameters:
//   - tx: the transaction that will read and write the row.
//   - id: the channel to lock.
//
// Return values:
//   - error: a wrapped database failure.
func lockChannelRow(tx *gorm.DB, id int) error {
	if err := tx.Model(&Channel{}).Where("id = ?", id).UpdateColumn("models", gorm.Expr("models")).Error; err != nil {
		return errors.Wrapf(err, "lock channel row %d", id)
	}
	return nil
}
