package controller

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/model"
)

// TestExternalSettlementWriteFailureRollsBack injects a database error after
// each financial row update. The terminal claim and every balance must roll
// back together, and an ordinary retry must settle the original hold once.
func TestExternalSettlementWriteFailureRollsBack(t *testing.T) {
	for _, table := range []string{"users", "tokens"} {
		t.Run(table, func(t *testing.T) {
			txn, token := newSettlementHold(t)
			var reached atomic.Bool
			const callback = "test:fail-financial-write"
			require.NoError(t, model.DB.Callback().Update().After("gorm:update").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Table == table && tx.RowsAffected == 1 && reached.CompareAndSwap(false, true) {
					tx.AddError(errors.New("injected settlement write failure"))
				}
			}))
			t.Cleanup(func() { require.NoError(t, model.DB.Callback().Update().Remove(callback)) })
			c, _ := newConsumeTokenContext(t, http.MethodPost, "{}", token.UserId, token.Id, "fault-settlement")
			quota := uint64(170)
			req := &consumeTokenRequest{TransactionID: &txn.TransactionID, FinalUsedQuota: &quota, AddReason: "test"}
			_, _, err := processPostConsume(context.Background(), c, token, token.UserId, req, nil)
			require.True(t, reached.Load(), "fault must execute after the real financial UPDATE")
			require.Error(t, err)
			var stored model.TokenTransaction
			require.NoError(t, model.DB.First(&stored, txn.Id).Error)
			require.Equal(t, model.TokenTransactionStatusPending, stored.Status)
			require.Nil(t, stored.FinalQuota)
			assertSettlementBalances(t, token, 100)
			require.NoError(t, model.DB.Callback().Update().Remove(callback))
			_, _, err = processPostConsume(context.Background(), c, token, token.UserId, req, nil)
			require.NoError(t, err)
			assertSettlementBalances(t, token, 170)
		})
	}
}

// TestExternalSettlementFinalQuotaBoundaries verifies zero, unchanged and
// increased final charges and ensures a repeated finalization cannot move money.
func TestExternalSettlementFinalQuotaBoundaries(t *testing.T) {
	for _, quota := range []int64{0, 100, 170} {
		txn, token := newSettlementHold(t)
		result := model.TokenTransactionFinalization{Status: model.TokenTransactionStatusConfirmed, FinalQuota: quota, At: helper.GetTimestamp()}
		stored, err := model.FinalizePendingTokenTransaction(context.Background(), token.Id, txn.Id, result)
		require.NoError(t, err)
		require.Equal(t, quota, *stored.FinalQuota)
		assertSettlementBalances(t, token, quota)
		_, err = model.FinalizePendingTokenTransaction(context.Background(), token.Id, txn.Id, result)
		require.Error(t, err)
		assertSettlementBalances(t, token, quota)
	}
}
