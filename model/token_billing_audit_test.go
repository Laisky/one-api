package model

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

var billingAuditSequence atomic.Uint64

// billingAuditRows creates an isolated user/token pair with equal initial quota.
// It returns their persisted identities for transaction and concurrency tests.
func billingAuditRows(t *testing.T, quota int64, unlimited bool) (*User, *Token) {
	t.Helper()
	name := fmt.Sprintf("test-billing-%d", billingAuditSequence.Add(1))
	user := &User{Username: name, AccessToken: name, AffCode: name, Quota: quota, Status: UserStatusEnabled, Role: RoleCommonUser}
	require.NoError(t, DB.Create(user).Error)
	token := &Token{UserId: user.Id, Key: name, Name: name, Status: TokenStatusEnabled, RemainQuota: quota, UnlimitedQuota: unlimited}
	require.NoError(t, DB.Create(token).Error)
	return user, token
}

// requireBillingAuditBalances asserts both physical balances and token usage.
// It does not consult Redis or in-memory batch queues.
func requireBillingAuditBalances(t *testing.T, user *User, token *Token, userQuota, tokenQuota, used int64) {
	t.Helper()
	var actualUser User
	var actualToken Token
	require.NoError(t, DB.First(&actualUser, user.Id).Error)
	require.NoError(t, DB.First(&actualToken, token.Id).Error)
	require.Equal(t, userQuota, actualUser.Quota)
	require.Equal(t, tokenQuota, actualToken.RemainQuota)
	require.Equal(t, used, actualToken.UsedQuota)
}

// TestBillingAuditDurableBalances verifies admission, incremental debit, refund
// and final settlement remain synchronous even with aggregate batching enabled.
func TestBillingAuditDurableBalances(t *testing.T) {
	setupTestDatabase(t)
	before := config.BatchUpdateEnabled
	config.BatchUpdateEnabled = true
	t.Cleanup(func() { config.BatchUpdateEnabled = before })
	user, token := billingAuditRows(t, 1000, false)
	require.NoError(t, PreConsumeTokenQuota(context.Background(), token.Id, 200))
	requireBillingAuditBalances(t, user, token, 800, 800, 200)
	require.NoError(t, PostConsumeTokenQuota(context.Background(), token.Id, 50))
	requireBillingAuditBalances(t, user, token, 750, 750, 250)
	require.NoError(t, SettleConsumedTokenQuota(context.Background(), token.Id, user.Id, -125))
	requireBillingAuditBalances(t, user, token, 875, 875, 125)
}

// TestBillingAuditReservationRollback verifies a token-side failure rolls back
// the preceding user debit and that insufficient user funds never debit a token.
func TestBillingAuditReservationRollback(t *testing.T) {
	setupTestDatabase(t)
	user, token := billingAuditRows(t, 100, false)
	stale := *token
	stale.Id = -1
	require.Error(t, reserveTokenQuota(context.Background(), &stale, 20))
	stale.Id = token.Id + 100000000
	require.Error(t, reserveTokenQuota(context.Background(), &stale, 20))
	requireBillingAuditBalances(t, user, token, 100, 100, 0)
	require.Error(t, PreConsumeTokenQuota(context.Background(), token.Id, 101))
	requireBillingAuditBalances(t, user, token, 100, 100, 0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.Error(t, reserveTokenQuota(ctx, token, 20))
	requireBillingAuditBalances(t, user, token, 100, 100, 0)
}

// TestBillingAuditConcurrentAdmission proves only funded reservations succeed
// under contention and every success debits both balances exactly once.
func TestBillingAuditConcurrentAdmission(t *testing.T) {
	setupTestDatabase(t)
	user, token := billingAuditRows(t, 1000, false)
	var successes atomic.Int64
	var workers sync.WaitGroup
	for range 40 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if err := PreConsumeTokenQuota(context.Background(), token.Id, 60); err == nil {
				successes.Add(1)
			}
		}()
	}
	workers.Wait()
	require.EqualValues(t, 16, successes.Load())
	requireBillingAuditBalances(t, user, token, 40, 40, 960)
}

// TestBillingAuditSettlementRecordsDebt verifies already-consumed work is not
// forgiven when balances run out, while subsequent admission remains blocked.
func TestBillingAuditSettlementRecordsDebt(t *testing.T) {
	setupTestDatabase(t)
	for _, unlimited := range []bool{false, true} {
		t.Run(fmt.Sprint(unlimited), func(t *testing.T) {
			user, token := billingAuditRows(t, 20, unlimited)
			require.NoError(t, SettleConsumedTokenQuota(context.Background(), token.Id, user.Id, 50))
			if unlimited {
				requireBillingAuditBalances(t, user, token, -30, 20, 0)
			} else {
				requireBillingAuditBalances(t, user, token, -30, -30, 50)
			}
			require.Error(t, PreConsumeTokenQuota(context.Background(), token.Id, 1))
		})
	}
}

// TestBillingAuditSettlementRollback protects both balances when token writes
// cannot be represented, or the owning user disappeared before settlement.
func TestBillingAuditSettlementRollback(t *testing.T) {
	setupTestDatabase(t)
	user, token := billingAuditRows(t, 100, false)
	require.Error(t, SettleConsumedTokenQuota(context.Background(), token.Id, user.Id+1, 10))
	requireBillingAuditBalances(t, user, token, 100, 100, 0)
	require.NoError(t, DB.Model(&Token{}).Where("id = ?", token.Id).Update("used_quota", int64(math.MaxInt64)).Error)
	require.Error(t, SettleConsumedTokenQuota(context.Background(), token.Id, user.Id, 1))
	requireBillingAuditBalances(t, user, token, 100, 100, math.MaxInt64)
	require.NoError(t, DB.Delete(&User{}, user.Id).Error)
	require.Error(t, SettleConsumedTokenQuota(context.Background(), token.Id, user.Id, -10))
	var actual Token
	require.NoError(t, DB.First(&actual, token.Id).Error)
	require.EqualValues(t, 100, actual.RemainQuota)
	require.Error(t, SettleConsumedTokenQuota(context.Background(), token.Id, user.Id, math.MinInt64))
}
