package controller

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
)

// TestExternalFinalizationResponseExpiry preserves the historical HTTP response
// expiry without restoring the terminal database deadline. t provides isolated
// request/database fixtures; the test also verifies balances and timeout safety.
func TestExternalFinalizationResponseExpiry(t *testing.T) {
	for _, phase := range []string{"post", "cancel", "single", "single_zero"} {
		t.Run(phase, func(t *testing.T) {
			cleanup, user, token := setupConsumeTokenTest(t)
			defer cleanup()
			config.ExternalBillingDefaultTimeoutSec = 300
			config.ExternalBillingMaxTimeoutSec = 300
			// Observe actual inserted hold instead of guessing wall-clock expiry.
			var reservedExpiry atomic.Int64
			const callback = "test:observe-reservation-expiry"
			require.NoError(t, model.DB.Callback().Create().After("gorm:create").Register(callback, func(tx *gorm.DB) {
				if txn, ok := tx.Statement.Dest.(*model.TokenTransaction); ok && txn.Status == model.TokenTransactionStatusPending {
					reservedExpiry.Store(txn.ExpiresAt)
				}
			}))
			defer func() { require.NoError(t, model.DB.Callback().Create().Remove(callback)) }()
			body := `{"phase":"single","add_used_quota":100,"add_reason":"expiry-contract"}`
			charge := int64(100)
			status := model.TokenTransactionStatusConfirmed
			if phase == "single_zero" {
				body = `{"phase":"single","add_used_quota":0,"add_reason":"expiry-contract"}`
				charge = 0
			} else if phase == "post" || phase == "cancel" {
				pre, _ := consumeTokenContractDo(t, user.Id, token.Id, "expiry-pre",
					`{"phase":"pre","add_used_quota":100,"add_reason":"expiry-contract"}`)
				hold := pre["transaction"].(map[string]any)
				require.EqualValues(t, reservedExpiry.Load(), hold["expires_at"])
				body = fmt.Sprintf(`{"phase":%q,"transaction_id":%q,"final_used_quota":40,"add_reason":"expiry-contract"}`,
					phase, hold["transaction_id"].(string))
				charge = 40
				if phase == "cancel" {
					charge = 0
					status = model.TokenTransactionStatusCanceled
				}
			}
			response, _ := consumeTokenContractDo(t, user.Id, token.Id, "expiry-final", body)
			wire := response["transaction"].(map[string]any)
			persisted, err := model.GetTokenTransactionByTokenAndID(context.Background(), token.Id, wire["transaction_id"].(string))
			require.NoError(t, err)
			require.Equal(t, status, persisted.Status)
			require.Zero(t, persisted.ExpiresAt, "terminal DB rows must not retain an active timeout")
			require.NotNil(t, persisted.FinalQuota)
			require.Equal(t, charge, *persisted.FinalQuota)
			assertFinalizationBalances(t, user.Id, token.Id, charge)
			autoConfirmed, err := model.AutoConfirmExpiredTokenTransactions(context.Background(), token.Id, reservedExpiry.Load()+1)
			require.NoError(t, err)
			require.Empty(t, autoConfirmed, "response compatibility must not reopen timeout finalization")
			assertFinalizationBalances(t, user.Id, token.Id, charge)
			if phase != "single_zero" {
				require.Positive(t, reservedExpiry.Load(), "the fixture must have observed a real hold")
			}
			require.EqualValues(t, reservedExpiry.Load(), wire["expires_at"],
				"preserve the original reservation expiry in the terminal HTTP response only")
		})
	}
}
