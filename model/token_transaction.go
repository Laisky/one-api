package model

import (
	"context"

	"github.com/Laisky/errors/v2"
)

const (
	// TokenTransactionStatusPending indicates the pre-consume hold has been created
	// and is waiting for confirmation or cancellation.
	TokenTransactionStatusPending = 1
	// TokenTransactionStatusConfirmed marks a transaction that has been
	// reconciled by a post-consume record.
	TokenTransactionStatusConfirmed = 2
	// TokenTransactionStatusAutoConfirmed marks a transaction that reached its
	// timeout and was automatically confirmed with the pre-consume amount.
	TokenTransactionStatusAutoConfirmed = 3
	// TokenTransactionStatusCanceled marks a transaction that was explicitly
	// canceled before confirmation.
	TokenTransactionStatusCanceled = 4
)

// TokenTransaction records the lifecycle of an external billing transaction,
// tracking both the pre-consume reservation and the post-consume reconciliation.
//
// Fields:
//   - TransactionID: external identifier supplied by the upstream billing source.
//   - TokenId/UserId: link the transaction back to the token and user whose quota
//     was reserved.
//   - Status: current lifecycle state (see TokenTransactionStatus* constants).
//   - PreQuota: quota deducted during the pre-consume phase.
//   - FinalQuota: quota after reconciliation; nil until confirmed.
//   - Reason: free-form description provided by the upstream system.
//   - RequestId/TraceId: identifiers from the originating HTTP request for audit.
//   - ExpiresAt: timestamp (seconds) when the hold auto-confirms if still pending.
//   - ConfirmedAt/CanceledAt: timestamps for terminal states.
//   - AutoConfirmed: true when the timeout flow finalized the transaction.
//   - LogId: associated consumption log entry for updating audit records.
//   - ElapsedTimeMs: optional latency metric supplied by the upstream system.
//   - CreatedAt/UpdatedAt: standard audit timestamps.
//
// GORM automatically manages CreatedAt/UpdatedAt as millisecond timestamps.
type TokenTransaction struct {
	Id            int    `json:"id"`
	TransactionID string `json:"transaction_id" gorm:"size:128;uniqueIndex:uidx_token_txn"`
	TokenId       int    `json:"token_id" gorm:"index;uniqueIndex:uidx_token_txn"`
	UserId        int    `json:"user_id" gorm:"index"`
	Status        int    `json:"status" gorm:"index"`
	PreQuota      int64  `json:"pre_quota"`
	FinalQuota    *int64 `json:"final_quota"`
	Reason        string `json:"reason" gorm:"type:text"`
	RequestId     string `json:"request_id" gorm:"size:64"`
	TraceId       string `json:"trace_id" gorm:"size:64"`
	ExpiresAt     int64  `json:"expires_at" gorm:"index"`
	ConfirmedAt   *int64 `json:"confirmed_at"`
	CanceledAt    *int64 `json:"canceled_at"`
	AutoConfirmed bool   `json:"auto_confirmed" gorm:"default:false"`
	LogId         *int   `json:"log_id" gorm:"index"`
	ElapsedTimeMs *int64 `json:"elapsed_time_ms"`
	CreatedAt     int64  `json:"created_at" gorm:"autoCreateTime:milli"`
	UpdatedAt     int64  `json:"updated_at" gorm:"autoUpdateTime:milli"`
}

// TokenTransactionStatusString converts a status code into a human-readable label.
// The return value is "unknown" if the status is not recognized.
func TokenTransactionStatusString(status int) string {
	switch status {
	case TokenTransactionStatusPending:
		return "pending"
	case TokenTransactionStatusConfirmed:
		return "confirmed"
	case TokenTransactionStatusAutoConfirmed:
		return "auto_confirmed"
	case TokenTransactionStatusCanceled:
		return "canceled"
	default:
		return "unknown"
	}
}

// CreateTokenTransaction inserts a new token transaction record.
// Parameters:
//   - ctx: request context for logging and cancellation.
//   - txn: populated TokenTransaction instance to persist.
//
// Returns an error if the insert fails.
func CreateTokenTransaction(ctx context.Context, txn *TokenTransaction) error {
	if txn == nil {
		return errors.Errorf("token transaction payload cannot be nil")
	}

	if err := DB.WithContext(ctx).Create(txn).Error; err != nil {
		return errors.Wrap(err, "failed to create token transaction")
	}
	return nil
}

// GetTokenTransactionByTokenAndID retrieves a token transaction by its token ID and external transaction identifier.
// Parameters:
//   - ctx: request context for cancellation propagation.
//   - tokenID: token identifier associated with the transaction.
//   - transactionID: external transaction identifier provided during pre-consume.
//
// Returns:
//   - *TokenTransaction: populated transaction if found.
//   - error: wrapped error, including gorm.ErrRecordNotFound if no record exists.
func GetTokenTransactionByTokenAndID(ctx context.Context, tokenID int, transactionID string) (*TokenTransaction, error) {
	txn := &TokenTransaction{}
	err := DB.WithContext(ctx).
		Where("token_id = ? AND transaction_id = ?", tokenID, transactionID).
		First(txn).Error
	if err != nil {
		return nil, errors.Wrapf(err, "failed to fetch token transaction: token_id=%d, transaction_id=%s", tokenID, transactionID)
	}
	return txn, nil
}

// UpdateTokenTransaction applies a partial update to an existing transaction.
// Parameters:
//   - ctx: request context.
//   - transactionID: primary key of the transaction to update.
//   - updates: columns to update.
//
// Returns an error if the update fails.
func UpdateTokenTransaction(ctx context.Context, transactionID int, updates map[string]any) error {
	if transactionID == 0 {
		return errors.Errorf("transaction id cannot be zero")
	}
	if len(updates) == 0 {
		return nil
	}
	if err := validateTokenTransactionUpdates(updates); err != nil {
		return errors.Wrap(err, "validate token transaction updates")
	}

	if err := DB.WithContext(ctx).Model(&TokenTransaction{}).
		Where("id = ?", transactionID).
		Updates(updates).Error; err != nil {
		return errors.Wrapf(err, "failed to update token transaction: id=%d", transactionID)
	}
	return nil
}

// UpdatePendingTokenTransaction applies a partial update only when the transaction is still pending.
// Parameters:
//   - ctx: request context.
//   - transactionID: primary key of the transaction to update.
//   - updates: columns to update while atomically claiming the pending state.
//
// Returns an error if the update fails or if another worker already finalized the transaction.
func UpdatePendingTokenTransaction(ctx context.Context, transactionID int, updates map[string]any) error {
	updated, err := TryUpdatePendingTokenTransaction(ctx, transactionID, updates)
	if err != nil {
		return errors.Wrap(err, "try update pending token transaction")
	}
	if !updated {
		return errors.Errorf("token transaction %d is not pending", transactionID)
	}
	return nil
}

// TryUpdatePendingTokenTransaction applies a partial update only when the transaction is still pending.
// Parameters:
//   - ctx: request context.
//   - transactionID: primary key of the transaction to update.
//   - updates: columns to update while atomically claiming the pending state.
//
// Returns true when the pending transaction was updated, false when it was already finalized, and an error for validation or database failures.
func TryUpdatePendingTokenTransaction(ctx context.Context, transactionID int, updates map[string]any) (bool, error) {
	if transactionID == 0 {
		return false, errors.Errorf("transaction id cannot be zero")
	}
	if len(updates) == 0 {
		return true, nil
	}
	if err := validateTokenTransactionUpdates(updates); err != nil {
		return false, errors.Wrap(err, "validate token transaction updates")
	}

	result := DB.WithContext(ctx).Model(&TokenTransaction{}).
		Where("id = ? AND status = ?", transactionID, TokenTransactionStatusPending).
		Updates(updates)
	if result.Error != nil {
		return false, errors.Wrapf(result.Error, "failed to update pending token transaction: id=%d", transactionID)
	}
	return result.RowsAffected == 1, nil
}

// validateTokenTransactionUpdates ensures partial token transaction updates only target approved columns.
// Parameters:
//   - updates: columns and values the caller wants to persist.
//
// Returns an error when an unsupported column is present.
func validateTokenTransactionUpdates(updates map[string]any) error {
	for field := range updates {
		switch field {
		case "status", "final_quota", "confirmed_at", "auto_confirmed", "expires_at", "reason", "elapsed_time_ms", "canceled_at":
			// allowed field
		default:
			return errors.Errorf("unsupported token transaction update field: %s", field)
		}
	}

	return nil
}

// AutoConfirmExpiredTokenTransactions marks pending transactions as auto-confirmed when their timeout is reached.
// Parameters:
//   - ctx: request context for cancellation.
//   - tokenID: token owner of the transactions to scan.
//   - now: current timestamp in seconds; transactions with expires_at <= now are auto-confirmed.
//
// Returns the list of transactions that were auto-confirmed during this invocation.
func AutoConfirmExpiredTokenTransactions(ctx context.Context, tokenID int, now int64) ([]*TokenTransaction, error) {
	var pending []*TokenTransaction
	err := DB.WithContext(ctx).
		Where("token_id = ? AND status = ? AND expires_at > 0 AND expires_at <= ?", tokenID, TokenTransactionStatusPending, now).
		Find(&pending).Error
	if err != nil {
		return nil, errors.Wrapf(err, "failed to find expired token transactions: token_id=%d", tokenID)
	}

	if len(pending) == 0 {
		return nil, nil
	}

	confirmed := make([]*TokenTransaction, 0, len(pending))
	for _, txn := range pending {
		final := txn.PreQuota
		confirmedAt := now
		updates := map[string]any{
			"status":         TokenTransactionStatusAutoConfirmed,
			"final_quota":    final,
			"confirmed_at":   confirmedAt,
			"auto_confirmed": true,
			"expires_at":     int64(0),
		}

		updated, updateErr := TryUpdatePendingTokenTransaction(ctx, txn.Id, updates)
		if updateErr != nil {
			return nil, errors.Wrapf(updateErr, "failed to auto-confirm token transaction: id=%d", txn.Id)
		}
		if !updated {
			continue
		}

		txn.Status = TokenTransactionStatusAutoConfirmed
		txn.AutoConfirmed = true
		txn.FinalQuota = &final
		txn.ConfirmedAt = &confirmedAt
		confirmed = append(confirmed, txn)
	}

	return confirmed, nil
}

// GetTokenTransactionsByTokenID retrieves a paginated list of transactions for a specific token.
// Parameters:
//   - ctx: request context for cancellation.
//   - tokenID: token ID whose transactions to retrieve.
//   - startIdx: offset for pagination.
//   - num: number of records to return.
//
// Returns:
//   - []*TokenTransaction: list of transaction records.
//   - error: wrapped error if the query fails.
func GetTokenTransactionsByTokenID(ctx context.Context, tokenID int, startIdx int, num int) ([]*TokenTransaction, error) {
	var txns []*TokenTransaction
	err := DB.WithContext(ctx).
		Where("token_id = ?", tokenID).
		Order("id desc").
		Limit(num).Offset(startIdx).
		Find(&txns).Error
	if err != nil {
		return nil, errors.Wrapf(err, "failed to fetch token transactions: token_id=%d", tokenID)
	}
	return txns, nil
}

// GetTokenTransactionCountByTokenID returns the total number of transactions recorded for a token.
// Parameters:
//   - ctx: request context for cancellation.
//   - tokenID: token ID to query.
//
// Returns:
//   - int64: total count.
//   - error: wrapped error if the query fails.
func GetTokenTransactionCountByTokenID(ctx context.Context, tokenID int) (int64, error) {
	var count int64
	err := DB.WithContext(ctx).Model(&TokenTransaction{}).
		Where("token_id = ?", tokenID).
		Count(&count).Error
	if err != nil {
		return 0, errors.Wrapf(err, "failed to count token transactions: token_id=%d", tokenID)
	}
	return count, nil
}
