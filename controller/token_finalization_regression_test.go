package controller

import (
	"context"
	"fmt"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
)

// seedFinalizationReservation reserves 100 units through production admission.
// t, user, and token identify the fixture; it returns the pending ledger record.
func seedFinalizationReservation(t *testing.T, user *model.User, token *model.Token) *model.TokenTransaction {
	t.Helper()
	id := "finalization-regression"
	c, _ := newConsumeTokenContext(t, http.MethodPost, "", user.Id, token.Id, "reserve")
	txn, _, err := processPreConsume(context.Background(), c, token, user.Id,
		&consumeTokenRequest{AddUsedQuota: 100, AddReason: "regression", TransactionID: &id}, "reserve", "")
	require.NoError(t, err)
	require.Equal(t, int64(100), txn.PreQuota)
	return txn
}

// assertFinalizationBalances checks both persistent accounts against charge.
// t supplies assertions and the IDs identify fixture rows; it returns nothing.
func assertFinalizationBalances(t *testing.T, userID, tokenID int, charge int64) {
	t.Helper()
	var user model.User
	var token model.Token
	require.NoError(t, model.DB.First(&user, userID).Error)
	require.NoError(t, model.DB.First(&token, tokenID).Error)
	require.Equal(t, int64(1000)-charge, user.Quota)
	require.Equal(t, int64(1000)-charge, token.RemainQuota)
	require.Equal(t, charge, token.UsedQuota)
}

// TestExternalFinalizationCrashRollback injects a panic during quota mutation,
// then checks atomic rollback and a retry that applies the charge only once.
func TestExternalFinalizationCrashRollback(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(map[bool]string{false: "post", true: "cancel"}[cancel], func(t *testing.T) {
			cleanup, user, token := setupConsumeTokenTest(t)
			defer cleanup()
			txn := seedFinalizationReservation(t, user, token)
			final := uint64(40)
			req := &consumeTokenRequest{TransactionID: &txn.TransactionID, FinalUsedQuota: &final, AddReason: "regression"}
			c, _ := newConsumeTokenContext(t, http.MethodPost, "", user.Id, token.Id, "finalize")
			const callback = "test:interrupt-financial-finalization"
			require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Table == "tokens" {
					panic("injected interruption before token balance mutation")
				}
			}))
			removed := false
			defer func() {
				if !removed {
					require.NoError(t, model.DB.Callback().Update().Remove(callback))
				}
			}()
			require.Panics(t, func() {
				if cancel {
					_, _, _ = processCancelConsume(context.Background(), c, token, user.Id, req)
				} else {
					_, _, _ = processPostConsume(context.Background(), c, token, user.Id, req, txn)
				}
			})
			require.NoError(t, model.DB.Callback().Update().Remove(callback))
			removed = true
			persisted, err := model.GetTokenTransactionByTokenAndID(context.Background(), token.Id, txn.TransactionID)
			require.NoError(t, err)
			require.Equal(t, model.TokenTransactionStatusPending, persisted.Status, "interruption cannot commit a terminal state without its balance change")
			require.Nil(t, persisted.FinalQuota)
			assertFinalizationBalances(t, user.Id, token.Id, 100)
			var updated *model.TokenTransaction
			if cancel {
				updated, _, err = processCancelConsume(context.Background(), c, token, user.Id, req)
			} else {
				updated, _, err = processPostConsume(context.Background(), c, token, user.Id, req, persisted)
			}
			require.NoError(t, err)
			require.NotNil(t, updated.FinalQuota)
			expected := int64(40)
			if cancel {
				expected = 0
			}
			require.Equal(t, expected, *updated.FinalQuota)
			assertFinalizationBalances(t, user.Id, token.Id, expected)
			if cancel {
				_, _, err = processCancelConsume(context.Background(), c, token, user.Id, req)
			} else {
				_, _, err = processPostConsume(context.Background(), c, token, user.Id, req, nil)
			}
			require.Error(t, err, "replaying a terminal transaction cannot move quota")
			assertFinalizationBalances(t, user.Id, token.Id, expected)
		})
	}
}

// TestExternalFinalizationConcurrentPosts uses stale pending snapshots to verify
// that concurrent post workers cannot independently refund one reservation.
func TestExternalFinalizationConcurrentPosts(t *testing.T) {
	cleanup, user, token := setupConsumeTokenTest(t)
	defer cleanup()
	txn := seedFinalizationReservation(t, user, token)
	var success atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 12; i++ {
		snapshot := *txn
		c, _ := newConsumeTokenContext(t, http.MethodPost, "", user.Id, token.Id, fmt.Sprintf("post-%d", i))
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			final := uint64(40)
			_, _, err := processPostConsume(context.Background(), c, token, user.Id,
				&consumeTokenRequest{TransactionID: &snapshot.TransactionID, FinalUsedQuota: &final, AddReason: "concurrent"}, &snapshot)
			if err == nil {
				success.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	require.Equal(t, int64(1), success.Load())
	assertFinalizationBalances(t, user.Id, token.Id, 40)
}

// TestExternalFinalizationPostCancelTimeoutRace races all terminal paths and
// verifies one winner whose ledger and two account balances remain consistent.
func TestExternalFinalizationPostCancelTimeoutRace(t *testing.T) {
	cleanup, user, token := setupConsumeTokenTest(t)
	defer cleanup()
	txn := seedFinalizationReservation(t, user, token)
	now := helper.GetTimestamp()
	require.NoError(t, model.DB.Model(txn).Update("expires_at", now-1).Error)
	var success atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 15; i++ {
		snapshot := *txn
		c, _ := newConsumeTokenContext(t, http.MethodPost, "", user.Id, token.Id, fmt.Sprintf("mixed-%d", i))
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			final := uint64(40)
			req := &consumeTokenRequest{TransactionID: &snapshot.TransactionID, FinalUsedQuota: &final, AddReason: "mixed"}
			switch i % 3 {
			case 0:
				if _, _, err := processPostConsume(context.Background(), c, token, user.Id, req, &snapshot); err == nil {
					success.Add(1)
				}
			case 1:
				if _, _, err := processCancelConsume(context.Background(), c, token, user.Id, req); err == nil {
					success.Add(1)
				}
			case 2:
				if rows, err := model.AutoConfirmExpiredTokenTransactions(context.Background(), token.Id, now); err == nil {
					success.Add(int64(len(rows)))
				}
			}
		}()
	}
	close(start)
	wg.Wait()
	require.Equal(t, int64(1), success.Load())
	persisted, err := model.GetTokenTransactionByTokenAndID(context.Background(), token.Id, txn.TransactionID)
	require.NoError(t, err)
	require.NotNil(t, persisted.FinalQuota)
	charge, ok := map[int]int64{model.TokenTransactionStatusConfirmed: 40, model.TokenTransactionStatusCanceled: 0, model.TokenTransactionStatusAutoConfirmed: 100}[persisted.Status]
	require.True(t, ok)
	require.Equal(t, charge, *persisted.FinalQuota)
	assertFinalizationBalances(t, user.Id, token.Id, charge)
}
