package model

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/helper"
)

func TestUpdateConsumeLogByIDFieldValidation(t *testing.T) {
	setupTestDatabase(t)

	logEntry := &Log{
		UserId:    999999,
		Type:      LogTypeConsume,
		Content:   "test consume log",
		CreatedAt: helper.GetTimestamp(),
		UpdatedAt: helper.GetTimestamp(),
		RequestId: fmt.Sprintf("test-log-%d", time.Now().UnixNano()),
	}
	require.NoError(t, LOG_DB.Create(logEntry).Error)

	// Allowed fields should update successfully
	err := UpdateConsumeLogByID(context.Background(), logEntry.Id, map[string]any{
		"quota":                42,
		"content":              "test consume log updated",
		"cached_prompt_tokens": 11,
	})
	require.NoError(t, err)

	var updated Log
	require.NoError(t, LOG_DB.First(&updated, logEntry.Id).Error)
	assert.Equal(t, 42, updated.Quota)
	assert.Equal(t, "test consume log updated", updated.Content)
	assert.Equal(t, 11, updated.CachedPromptTokens)

	// Unsupported fields should return an error
	err = UpdateConsumeLogByID(context.Background(), logEntry.Id, map[string]any{"unsupported": "value"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported consume log update field")
}

func TestUpdateTokenTransactionFieldValidation(t *testing.T) {
	setupTestDatabase(t)

	token := &Token{
		UserId:       999998,
		Key:          fmt.Sprintf("test-token-quota-%d", time.Now().UnixNano()),
		Status:       TokenStatusEnabled,
		Name:         "test token quota",
		CreatedTime:  helper.GetTimestamp(),
		AccessedTime: helper.GetTimestamp(),
		RemainQuota:  1000,
	}
	require.NoError(t, DB.Create(token).Error)

	txn := &TokenTransaction{
		TransactionID: fmt.Sprintf("test-txn-quota-%d", time.Now().UnixNano()),
		TokenId:       token.Id,
		UserId:        token.UserId,
		Status:        TokenTransactionStatusPending,
		PreQuota:      100,
		ExpiresAt:     helper.GetTimestamp() + 30,
	}
	require.NoError(t, DB.Create(txn).Error)

	err := UpdateTokenTransaction(context.Background(), txn.Id, map[string]any{"status": TokenTransactionStatusConfirmed})
	require.NoError(t, err)

	var fetched TokenTransaction
	require.NoError(t, DB.First(&fetched, txn.Id).Error)
	assert.Equal(t, TokenTransactionStatusConfirmed, fetched.Status)

	err = UpdateTokenTransaction(context.Background(), txn.Id, map[string]any{"bad_field": 1})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported token transaction update field")
}

func TestQuotaDecrementConcurrencyGuards(t *testing.T) {
	setupTestDatabase(t)

	user := &User{
		Username: fmt.Sprintf("test-user-quota-guard-%d", time.Now().UnixNano()),
		Password: "test-user-quota-guard",
		Status:   UserStatusEnabled,
		Role:     RoleCommonUser,
		Quota:    60,
	}
	require.NoError(t, DB.Create(user).Error)

	token := &Token{
		UserId:       user.Id,
		Key:          fmt.Sprintf("test-token-quota-guard-%d", time.Now().UnixNano()),
		Status:       TokenStatusEnabled,
		Name:         "test token quota guard",
		CreatedTime:  helper.GetTimestamp(),
		AccessedTime: helper.GetTimestamp(),
		RemainQuota:  50,
	}
	require.NoError(t, DB.Create(token).Error)

	ctx := context.Background()

	// Initial decrement succeeds
	require.NoError(t, decreaseTokenQuota(ctx, token.Id, 30))
	require.NoError(t, decreaseUserQuota(ctx, user.Id, 20))

	// Subsequent decrement that would go negative should fail for both token and user
	err := decreaseTokenQuota(ctx, token.Id, 25)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient token quota")

	err = decreaseUserQuota(ctx, user.Id, 50)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient user quota")

	// Ensure persisted values remain non-negative
	var refreshedToken Token
	require.NoError(t, DB.First(&refreshedToken, token.Id).Error)
	assert.GreaterOrEqual(t, refreshedToken.RemainQuota, int64(0))

	var refreshedUser User
	require.NoError(t, DB.First(&refreshedUser, user.Id).Error)
	assert.GreaterOrEqual(t, refreshedUser.Quota, int64(0))
}

func TestUpdatePendingTokenTransactionRequiresPendingStatus(t *testing.T) {
	setupTestDatabase(t)

	token := &Token{
		UserId:       999997,
		Key:          fmt.Sprintf("test-token-pending-claim-%d", time.Now().UnixNano()),
		Status:       TokenStatusEnabled,
		Name:         "test token pending claim",
		CreatedTime:  helper.GetTimestamp(),
		AccessedTime: helper.GetTimestamp(),
		RemainQuota:  1000,
	}
	require.NoError(t, DB.Create(token).Error)

	txn := &TokenTransaction{
		TransactionID: fmt.Sprintf("test-txn-pending-claim-%d", time.Now().UnixNano()),
		TokenId:       token.Id,
		UserId:        token.UserId,
		Status:        TokenTransactionStatusPending,
		PreQuota:      100,
		ExpiresAt:     helper.GetTimestamp() + 30,
	}
	require.NoError(t, DB.Create(txn).Error)

	confirmedAt := helper.GetTimestamp()
	err := UpdatePendingTokenTransaction(context.Background(), txn.Id, map[string]any{
		"status":       TokenTransactionStatusConfirmed,
		"final_quota":  int64(100),
		"confirmed_at": confirmedAt,
		"expires_at":   int64(0),
	})
	require.NoError(t, err)

	err = UpdatePendingTokenTransaction(context.Background(), txn.Id, map[string]any{"status": TokenTransactionStatusCanceled})
	require.Error(t, err)
	require.Contains(t, err.Error(), "is not pending")

	var fetched TokenTransaction
	require.NoError(t, DB.First(&fetched, txn.Id).Error)
	require.Equal(t, TokenTransactionStatusConfirmed, fetched.Status)
	require.NotNil(t, fetched.FinalQuota)
	require.Equal(t, int64(100), *fetched.FinalQuota)
}

func TestTryUpdatePendingTokenTransactionAllowsSingleConcurrentFinalizer(t *testing.T) {
	setupTestDatabase(t)

	token := &Token{
		UserId:       999996,
		Key:          fmt.Sprintf("test-token-pending-race-%d", time.Now().UnixNano()),
		Status:       TokenStatusEnabled,
		Name:         "test token pending race",
		CreatedTime:  helper.GetTimestamp(),
		AccessedTime: helper.GetTimestamp(),
		RemainQuota:  1000,
	}
	require.NoError(t, DB.Create(token).Error)

	txn := &TokenTransaction{
		TransactionID: fmt.Sprintf("test-txn-pending-race-%d", time.Now().UnixNano()),
		TokenId:       token.Id,
		UserId:        token.UserId,
		Status:        TokenTransactionStatusPending,
		PreQuota:      100,
		ExpiresAt:     helper.GetTimestamp() + 30,
	}
	require.NoError(t, DB.Create(txn).Error)

	const workers = 8
	var ready sync.WaitGroup
	ready.Add(workers)
	start := make(chan struct{})
	var done sync.WaitGroup
	done.Add(workers)
	var successes int64
	errCh := make(chan error, workers)

	for i := 0; i < workers; i++ {
		go func() {
			defer done.Done()
			ready.Done()
			<-start

			updated, err := TryUpdatePendingTokenTransaction(context.Background(), txn.Id, map[string]any{
				"status":      TokenTransactionStatusCanceled,
				"canceled_at": helper.GetTimestamp(),
				"final_quota": int64(0),
				"expires_at":  int64(0),
			})
			if err != nil {
				errCh <- err
				return
			}
			if updated {
				atomic.AddInt64(&successes, 1)
			}
		}()
	}

	ready.Wait()
	close(start)
	done.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}

	require.Equal(t, int64(1), successes)

	var fetched TokenTransaction
	require.NoError(t, DB.First(&fetched, txn.Id).Error)
	require.Equal(t, TokenTransactionStatusCanceled, fetched.Status)
}
