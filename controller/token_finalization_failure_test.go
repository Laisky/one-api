package controller

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/model"
)

// TestExternalFinalizationFailedWritesRetainPending verifies ordinary balance
// failures and cancellation between the state claim and account mutation. t
// supplies isolated fixtures and assertions; no failed attempt may commit.
func TestExternalFinalizationFailedWritesRetainPending(t *testing.T) {
	for _, failure := range []string{"user quota", "token quota", "canceled context", "cancel during token write"} {
		t.Run(failure, func(t *testing.T) {
			cleanup, user, token := setupConsumeTokenTest(t)
			defer cleanup()
			// Cancellation may discard the driver's connection. Keep the shared
			// in-memory database alive independently of that disposable connection.
			sqlDB, err := model.DB.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(2)
			keeper, err := sqlDB.Conn(context.Background())
			require.NoError(t, err)
			defer func() { require.NoError(t, keeper.Close()) }()
			txn := seedFinalizationReservation(t, user, token)
			switch failure {
			case "user quota":
				require.NoError(t, model.DB.Model(user).Update("quota", int64(5)).Error)
			case "token quota":
				require.NoError(t, model.DB.Model(token).Update("remain_quota", int64(5)).Error)
			}
			var beforeUser model.User
			var beforeToken model.Token
			require.NoError(t, model.DB.First(&beforeUser, user.Id).Error)
			require.NoError(t, model.DB.First(&beforeToken, token.Id).Error)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if failure == "canceled context" {
				cancel()
			}
			if failure == "cancel during token write" {
				const callback = "test:cancel-finalization-context"
				require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
					if tx.Statement.Table == "tokens" {
						cancel()
					}
				}))
				defer func() { require.NoError(t, model.DB.Callback().Update().Remove(callback)) }()
			}
			final := uint64(120)
			c, _ := newConsumeTokenContext(t, http.MethodPost, "", user.Id, token.Id, "failed-finalize")
			_, _, err = processPostConsume(ctx, c, token, user.Id, &consumeTokenRequest{TransactionID: &txn.TransactionID, FinalUsedQuota: &final, AddReason: "failure"}, txn)
			require.Error(t, err)
			current, err := model.GetTokenTransactionByTokenAndID(context.Background(), token.Id, txn.TransactionID)
			require.NoError(t, err)
			require.Equal(t, model.TokenTransactionStatusPending, current.Status)
			require.Nil(t, current.FinalQuota)
			var afterUser model.User
			var afterToken model.Token
			require.NoError(t, model.DB.First(&afterUser, user.Id).Error)
			require.NoError(t, model.DB.First(&afterToken, token.Id).Error)
			require.Equal(t, beforeUser.Quota, afterUser.Quota)
			require.Equal(t, beforeToken.RemainQuota, afterToken.RemainQuota)
			require.Equal(t, beforeToken.UsedQuota, afterToken.UsedQuota)
		})
	}
}

// TestExternalFinalizationUsesPersistedReservation verifies that a stale caller
// cannot alter the settlement delta by passing a changed reservation snapshot.
func TestExternalFinalizationUsesPersistedReservation(t *testing.T) {
	cleanup, user, token := setupConsumeTokenTest(t)
	defer cleanup()
	txn := seedFinalizationReservation(t, user, token)
	txn.PreQuota = 900
	final := uint64(40)
	c, _ := newConsumeTokenContext(t, http.MethodPost, "", user.Id, token.Id, "stale-reservation")
	result, _, err := processPostConsume(context.Background(), c, token, user.Id,
		&consumeTokenRequest{TransactionID: &txn.TransactionID, FinalUsedQuota: &final, AddReason: "stale"}, txn)
	require.NoError(t, err)
	require.Equal(t, int64(100), result.PreQuota)
	assertFinalizationBalances(t, user.Id, token.Id, 40)
}
