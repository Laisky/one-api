package controller

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/model"
)

// newSettlementHold reserves 100 quota through the production pre-consume
// implementation and returns its durable transaction and the original token.
func newSettlementHold(t *testing.T) (*model.TokenTransaction, *model.Token) {
	t.Helper()
	cleanup, user, token := setupConsumeTokenTest(t)
	t.Cleanup(cleanup)
	// A canceled SQL transaction can discard the last pooled connection. A
	// file database preserves durable rows across that event; shared in-memory
	// SQLite would lose the entire test schema and produce a false failure.
	db := openSettlementTestDatabase(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.TokenTransaction{}, &model.Log{}))
	require.NoError(t, db.Create(user).Error)
	require.NoError(t, db.Create(token).Error)
	model.DB, model.LOG_DB = db, db
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(4)
	pool.SetMaxIdleConns(4)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })

	c, _ := newConsumeTokenContext(t, http.MethodPost, "{}", user.Id, token.Id, "settlement-pre")
	txn, _, err := processPreConsume(c.Request.Context(), c, token, user.Id,
		&consumeTokenRequest{AddUsedQuota: 100, AddReason: "atomic-test"}, "settlement-pre", "")
	require.NoError(t, err)
	return txn, token
}

// assertSettlementBalances reads physical database rows and checks the exact
// owner balance, token balance and token usage for the expected final charge.
func assertSettlementBalances(t *testing.T, token *model.Token, charged int64) {
	t.Helper()
	var actualToken model.Token
	var actualUser model.User
	require.NoError(t, model.DB.First(&actualToken, token.Id).Error)
	require.NoError(t, model.DB.First(&actualUser, token.UserId).Error)
	require.Equal(t, int64(1000)-charged, actualUser.Quota)
	require.Equal(t, int64(1000)-charged, actualToken.RemainQuota)
	require.Equal(t, charged, actualToken.UsedQuota)
}

// TestExternalSettlementCancellationIsAtomic cancels the real request context
// immediately after the transaction-row update callback. A committed terminal
// claim must not survive without its corresponding persistent balance movement.
func TestExternalSettlementCancellationIsAtomic(t *testing.T) {
	for _, phase := range []string{"post", "cancel"} {
		t.Run(phase, func(t *testing.T) {
			txn, token := newSettlementHold(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var triggered atomic.Bool
			const callback = "test:cancel-after-terminal-update"
			require.NoError(t, model.DB.Callback().Update().After("gorm:commit_or_rollback_transaction").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Table == "token_transactions" && tx.RowsAffected == 1 {
					updates, ok := tx.Statement.Dest.(map[string]any)
					if ok && updates["status"] != model.TokenTransactionStatusPending && triggered.CompareAndSwap(false, true) {
						cancel()
					}
				}
			}))
			t.Cleanup(func() { require.NoError(t, model.DB.Callback().Update().Remove(callback)) })
			c, _ := newConsumeTokenContext(t, http.MethodPost, "{}", token.UserId, token.Id, "settlement-interrupt")
			c.Request = c.Request.WithContext(ctx)
			final := uint64(70)
			req := &consumeTokenRequest{TransactionID: &txn.TransactionID, FinalUsedQuota: &final, AddReason: "atomic-test"}
			var err error
			if phase == "post" {
				_, _, err = processPostConsume(ctx, c, token, token.UserId, req, nil)
			} else {
				_, _, err = processCancelConsume(ctx, c, token, token.UserId, req)
			}
			require.True(t, triggered.Load(), "the test must reach the terminal-row update")
			require.Error(t, err)
			// Prevent retry from being interrupted again.
			require.NoError(t, model.DB.Callback().Update().Remove(callback))
			var stored model.TokenTransaction
			require.NoError(t, model.DB.First(&stored, txn.Id).Error)
			require.Equal(t, model.TokenTransactionStatusPending, stored.Status, "an interrupted settlement must leave the reservation retryable")
			require.Nil(t, stored.FinalQuota)
			assertSettlementBalances(t, token, 100)
			c, _ = newConsumeTokenContext(t, http.MethodPost, "{}", token.UserId, token.Id, "settlement-retry")
			if phase == "post" {
				_, _, err = processPostConsume(c.Request.Context(), c, token, token.UserId, req, nil)
				require.NoError(t, err)
				assertSettlementBalances(t, token, 70)
			} else {
				_, _, err = processCancelConsume(c.Request.Context(), c, token, token.UserId, req)
				require.NoError(t, err)
				assertSettlementBalances(t, token, 0)
			}
		})
	}
}

// TestExternalSettlementConcurrentFinalizers races real post/cancel/expiry
// paths and verifies the committed winner alone determines all three balances.
func TestExternalSettlementConcurrentFinalizers(t *testing.T) {
	for _, scenario := range []string{"post", "cancel", "mixed", "with_expiry"} {
		t.Run(scenario, func(t *testing.T) {
			txn, token := newSettlementHold(t)
			require.NoError(t, model.DB.Model(txn).Update("expires_at", helper.GetTimestamp()-1).Error)
			const n = 12
			start := make(chan struct{})
			var wg sync.WaitGroup
			var wins atomic.Int64
			for i := 0; i < n; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					<-start
					if scenario == "with_expiry" && i%3 == 0 {
						confirmed, err := model.AutoConfirmExpiredTokenTransactions(context.Background(), token.Id, helper.GetTimestamp())
						if err == nil {
							wins.Add(int64(len(confirmed)))
						}
						return
					}
					c, _ := newConsumeTokenContext(t, http.MethodPost, "{}", token.UserId, token.Id, "settlement-race")
					final := uint64(70)
					req := &consumeTokenRequest{TransactionID: &txn.TransactionID, FinalUsedQuota: &final, AddReason: "atomic-test"}
					var err error
					if scenario == "cancel" || (scenario != "post" && i%2 == 0) {
						_, _, err = processCancelConsume(c.Request.Context(), c, token, token.UserId, req)
					} else {
						_, _, err = processPostConsume(c.Request.Context(), c, token, token.UserId, req, nil)
					}
					if err == nil {
						wins.Add(1)
					}
				}(i)
			}
			close(start)
			wg.Wait()
			require.Equal(t, int64(1), wins.Load())
			var stored model.TokenTransaction
			require.NoError(t, model.DB.First(&stored, txn.Id).Error)
			require.NotNil(t, stored.FinalQuota)
			assertSettlementBalances(t, token, *stored.FinalQuota)
			require.Contains(t, []int{model.TokenTransactionStatusConfirmed, model.TokenTransactionStatusCanceled, model.TokenTransactionStatusAutoConfirmed}, stored.Status)
		})
	}
}
