package model

import (
	"context"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"
)

// reserveTokenQuota synchronously reserves quota for token and its owner in one
// transaction. It returns a wrapped error without changing either balance if a
// conditional debit fails. In-memory batch updates are deliberately bypassed.
func reserveTokenQuota(ctx context.Context, token *Token, quota int64) error {
	if token == nil || token.Id <= 0 || token.UserId <= 0 || quota < 0 {
		return errors.New("invalid quota reservation")
	}
	if quota == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := runWithSQLiteBusyRetry(ctx, func() error {
		return errors.WithStack(DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return adjustPostConsumeQuota(tx, token, quota)
		}))
	}); err != nil {
		return errors.Wrap(err, "atomically reserve user and token quota")
	}
	clearTokenCache(ctx, token.Key)
	return nil
}
