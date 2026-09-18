package model

import (
	"context"
	"math"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/helper"
)

// SettleConsumedTokenQuota durably applies the remaining charge for work already
// performed. Unlike admission, settlement can record debt when concurrent work
// exhausted a balance. Future admission still rejects insufficient funds. Both
// user and finite-token balances change in one transaction, never an in-memory
// batch; missing rows or arithmetic overflow return a wrapped error and roll back.
func SettleConsumedTokenQuota(ctx context.Context, tokenID int, userID int, quota int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if tokenID <= 0 || userID <= 0 || quota == math.MinInt64 {
		return errors.New("invalid consumed quota settlement")
	}
	token, err := GetTokenById(tokenID)
	if err != nil {
		return errors.Wrap(err, "load token for consumed quota settlement")
	}
	if token.UserId != userID {
		return errors.New("settlement token does not belong to the billed user")
	}
	if quota == 0 {
		return nil
	}
	if err := runWithSQLiteBusyRetry(ctx, func() error {
		return errors.WithStack(DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return adjustSettledQuota(tx, token, quota)
		}))
	}); err != nil {
		return errors.Wrap(err, "commit consumed quota settlement")
	}
	clearTokenCache(ctx, token.Key)
	return nil
}

// adjustSettledQuota applies a signed debit inside tx with representability
// guards. It permits debt, but never a partial write or integer wraparound.
func adjustSettledQuota(tx *gorm.DB, token *Token, quota int64) error {
	user := tx.Model(&User{}).Where("id = ?", token.UserId)
	if quota > 0 {
		user = user.Where("quota >= ?", int64(math.MinInt64)+quota)
	} else {
		user = user.Where("quota <= ?", int64(math.MaxInt64)+quota)
	}
	result := user.Update("quota", gorm.Expr("quota - ?", quota))
	if result.Error != nil {
		return errors.Wrap(result.Error, "settle user balance")
	}
	if result.RowsAffected != 1 {
		return errors.New("missing user or user balance overflow in settlement")
	}
	if token.UnlimitedQuota {
		return nil
	}
	query := tx.Model(&Token{}).Where("id = ? AND user_id = ?", token.Id, token.UserId)
	if quota > 0 {
		query = query.Where("remain_quota >= ? AND used_quota <= ?", int64(math.MinInt64)+quota, int64(math.MaxInt64)-quota)
	} else {
		query = query.Where("remain_quota <= ? AND used_quota >= ?", int64(math.MaxInt64)+quota, int64(math.MinInt64)-quota)
	}
	result = query.Updates(map[string]any{"remain_quota": gorm.Expr("remain_quota - ?", quota),
		"used_quota": gorm.Expr("used_quota + ?", quota), "accessed_time": helper.GetTimestamp()})
	if result.Error != nil {
		return errors.Wrap(result.Error, "settle token balance")
	}
	if result.RowsAffected != 1 {
		return errors.New("missing token or token balance overflow in settlement")
	}
	return nil
}
