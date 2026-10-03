package model

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/helper"
)

// TestPostConsumeTokenQuotaRollsBackWhenUserQuotaIsInsufficient verifies that
// a failed user debit does not debit the associated token and returns an error.
func TestPostConsumeTokenQuotaRollsBackWhenUserQuotaIsInsufficient(t *testing.T) {
	setupTestDatabase(t)

	previousBatchUpdateEnabled := config.BatchUpdateEnabled
	config.BatchUpdateEnabled = false
	t.Cleanup(func() {
		config.BatchUpdateEnabled = previousBatchUpdateEnabled
	})

	user := &User{
		Username: fmt.Sprintf("test-post-consume-user-insufficient-%d", time.Now().UnixNano()),
		Password: "testpassword12345",
		Status:   UserStatusEnabled,
		Role:     RoleCommonUser,
		Quota:    5,
	}
	require.NoError(t, DB.Create(user).Error)

	token := &Token{
		UserId:       user.Id,
		Key:          fmt.Sprintf("test-post-consume-user-insufficient-%d", time.Now().UnixNano()),
		Status:       TokenStatusEnabled,
		Name:         fmt.Sprintf("test post consume user insufficient %d", time.Now().UnixNano()),
		CreatedTime:  helper.GetTimestamp(),
		AccessedTime: helper.GetTimestamp(),
		RemainQuota:  100,
		UsedQuota:    0,
	}
	require.NoError(t, DB.Create(token).Error)

	err := PostConsumeTokenQuota(context.Background(), token.Id, 10)
	require.Error(t, err)
	require.Contains(t, err.Error(), "insufficient user quota")

	var persistedUser User
	require.NoError(t, DB.First(&persistedUser, user.Id).Error)
	require.Equal(t, int64(5), persistedUser.Quota)

	var persistedToken Token
	require.NoError(t, DB.First(&persistedToken, token.Id).Error)
	require.Equal(t, int64(100), persistedToken.RemainQuota)
	require.Equal(t, int64(0), persistedToken.UsedQuota)
}

// TestPostConsumeTokenQuotaRollsBackWhenTokenQuotaIsInsufficient verifies that
// a failed token debit rolls back the associated user debit.
func TestPostConsumeTokenQuotaRollsBackWhenTokenQuotaIsInsufficient(t *testing.T) {
	setupTestDatabase(t)

	previousBatchUpdateEnabled := config.BatchUpdateEnabled
	config.BatchUpdateEnabled = false
	t.Cleanup(func() {
		config.BatchUpdateEnabled = previousBatchUpdateEnabled
	})

	user := &User{
		Username: fmt.Sprintf("test-post-consume-token-insufficient-%d", time.Now().UnixNano()),
		Password: "testpassword12345",
		Status:   UserStatusEnabled,
		Role:     RoleCommonUser,
		Quota:    100,
	}
	require.NoError(t, DB.Create(user).Error)

	token := &Token{
		UserId:       user.Id,
		Key:          fmt.Sprintf("test-post-consume-token-insufficient-%d", time.Now().UnixNano()),
		Status:       TokenStatusEnabled,
		Name:         fmt.Sprintf("test post consume token insufficient %d", time.Now().UnixNano()),
		CreatedTime:  helper.GetTimestamp(),
		AccessedTime: helper.GetTimestamp(),
		RemainQuota:  5,
		UsedQuota:    0,
	}
	require.NoError(t, DB.Create(token).Error)

	err := PostConsumeTokenQuota(context.Background(), token.Id, 10)
	require.Error(t, err)
	require.Contains(t, err.Error(), "insufficient token quota")

	var persistedUser User
	require.NoError(t, DB.First(&persistedUser, user.Id).Error)
	require.Equal(t, int64(100), persistedUser.Quota)

	var persistedToken Token
	require.NoError(t, DB.First(&persistedToken, token.Id).Error)
	require.Equal(t, int64(5), persistedToken.RemainQuota)
	require.Equal(t, int64(0), persistedToken.UsedQuota)
}

// TestPostConsumeTokenQuotaRefundRequiresTokenOwner verifies that a refund
// fails without changing the token when its owning user row no longer exists.
func TestPostConsumeTokenQuotaRefundRequiresTokenOwner(t *testing.T) {
	setupTestDatabase(t)

	previousBatchUpdateEnabled := config.BatchUpdateEnabled
	config.BatchUpdateEnabled = false
	t.Cleanup(func() {
		config.BatchUpdateEnabled = previousBatchUpdateEnabled
	})

	user := &User{
		Username: fmt.Sprintf("test-post-consume-refund-owner-%d", time.Now().UnixNano()),
		Password: "testpassword12345",
		Status:   UserStatusEnabled,
		Role:     RoleCommonUser,
		Quota:    90,
	}
	require.NoError(t, DB.Create(user).Error)

	token := &Token{
		UserId:       user.Id,
		Key:          fmt.Sprintf("test-post-consume-refund-owner-%d", time.Now().UnixNano()),
		Status:       TokenStatusEnabled,
		Name:         fmt.Sprintf("test post consume refund owner %d", time.Now().UnixNano()),
		CreatedTime:  helper.GetTimestamp(),
		AccessedTime: helper.GetTimestamp(),
		RemainQuota:  90,
		UsedQuota:    10,
	}
	require.NoError(t, DB.Create(token).Error)
	require.NoError(t, DB.Delete(&User{}, user.Id).Error)

	err := PostConsumeTokenQuota(context.Background(), token.Id, -10)
	require.Error(t, err)
	require.Contains(t, err.Error(), "user")

	var persistedToken Token
	require.NoError(t, DB.First(&persistedToken, token.Id).Error)
	require.Equal(t, int64(90), persistedToken.RemainQuota)
	require.Equal(t, int64(10), persistedToken.UsedQuota)
}
