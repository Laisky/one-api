package model

import (
	"context"
	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"
)

// ErrTokenTransactionNotPending identifies a lost finalization race or a stale
// transaction reference. It never authorizes a second quota movement.
var ErrTokenTransactionNotPending = errors.New("token transaction is not pending")

// TokenTransactionFinalization describes a terminal transition, not arbitrary
// database updates. FinalQuota applies to explicit confirmation. At is UTC Unix
// time in seconds; optional reason and positive elapsed time update audit data.
type TokenTransactionFinalization struct {
	Status        int
	FinalQuota    int64
	At            int64
	Reason        *string
	ElapsedTimeMs *int64
}

// FinalizePendingTokenTransaction atomically claims a pending transaction and
// adjusts both balances using the reservation stored in that same database.
// ctx controls cancellation, tokenID scopes ownership, transactionID is the
// internal ledger key, and final selects one terminal transition. It returns
// the committed record or an error with every financial mutation rolled back.
// Cache invalidation happens only after commit, never inside a retry attempt.
func FinalizePendingTokenTransaction(ctx context.Context, tokenID, transactionID int, final TokenTransactionFinalization) (*TokenTransaction, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if tokenID <= 0 || transactionID <= 0 || final.At <= 0 || final.FinalQuota < 0 {
		return nil, errors.New("invalid token transaction finalization")
	}
	updates := map[string]any{"status": final.Status, "expires_at": int64(0), "auto_confirmed": false}
	switch final.Status {
	case TokenTransactionStatusConfirmed:
		updates["final_quota"] = final.FinalQuota
		updates["confirmed_at"] = final.At
	case TokenTransactionStatusCanceled:
		if final.FinalQuota != 0 {
			return nil, errors.New("canceled transaction must have zero final quota")
		}
		updates["final_quota"] = int64(0)
		updates["canceled_at"] = final.At
	case TokenTransactionStatusAutoConfirmed:
		if final.FinalQuota != 0 {
			return nil, errors.New("automatic confirmation must use the stored reservation")
		}
		updates["final_quota"] = gorm.Expr("pre_quota")
		updates["confirmed_at"] = final.At
		updates["auto_confirmed"] = true
	default:
		return nil, errors.New("unsupported terminal token transaction status")
	}
	if final.Reason != nil {
		updates["reason"] = *final.Reason
	}
	if final.ElapsedTimeMs != nil && *final.ElapsedTimeMs > 0 {
		updates["elapsed_time_ms"] = *final.ElapsedTimeMs
	}
	var committed *TokenTransaction
	var tokenKey string
	db := DB
	err := runWithSQLiteBusyRetryForDB(ctx, db, func() error {
		committed = nil
		tokenKey = ""
		return errors.WithStack(db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			claim := tx.Model(&TokenTransaction{}).Where("id = ? AND token_id = ? AND status = ?", transactionID, tokenID, TokenTransactionStatusPending)
			if final.Status == TokenTransactionStatusAutoConfirmed {
				claim = claim.Where("expires_at > 0 AND expires_at <= ?", final.At)
			}
			result := claim.Updates(updates)
			if result.Error != nil {
				return errors.Wrap(result.Error, "claim pending token transaction")
			}
			if result.RowsAffected != 1 {
				return errors.WithStack(ErrTokenTransactionNotPending)
			}
			var txn TokenTransaction
			if err := tx.First(&txn, transactionID).Error; err != nil {
				return errors.Wrap(err, "read claimed token transaction")
			}
			if txn.PreQuota < 0 || txn.FinalQuota == nil || *txn.FinalQuota < 0 {
				return errors.New("invalid stored quota in token transaction")
			}
			delta := *txn.FinalQuota - txn.PreQuota
			if delta != 0 {
				var token Token
				if err := tx.Where("id = ? AND user_id = ?", tokenID, txn.UserId).First(&token).Error; err != nil {
					return errors.Wrap(err, "read transaction owner token")
				}
				if err := adjustPostConsumeQuota(tx, &token, delta); err != nil {
					return errors.Wrap(err, "apply token transaction quota delta")
				}
				tokenKey = token.Key
			}
			committed = &txn
			return nil
		}))
	})
	if err != nil {
		return nil, errors.Wrapf(err, "finalize pending token transaction: id=%d", transactionID)
	}
	if tokenKey != "" {
		clearTokenCache(ctx, tokenKey)
	}
	return committed, nil
}
