package model

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
)

// quotaRefundFixture opens an isolated persistent SQLite ledger and reserves 300
// quota from a limited token. It returns the database path and immutable intent.
func quotaRefundFixture(t *testing.T) (string, QuotaRefund) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "refund.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &QuotaRefund{}))
	oldDB, oldSQLite, oldRedis := DB, common.UsingSQLite.Load(), common.IsRedisEnabled()
	DB = db
	common.UsingSQLite.Store(true)
	common.SetRedisEnabled(false)
	t.Cleanup(func() {
		pool, err := DB.DB()
		require.NoError(t, err)
		require.NoError(t, pool.Close())
		DB = oldDB
		common.UsingSQLite.Store(oldSQLite)
		common.SetRedisEnabled(oldRedis)
	})
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(1)
	user := &User{Id: 101, Username: "refund-fixture", Quota: 10000}
	token := &Token{Id: 102, UserId: 101, Key: "refund-fixture-key", RemainQuota: 10000}
	require.NoError(t, db.Create(user).Error)
	require.NoError(t, db.Create(token).Error)
	require.NoError(t, PostConsumeTokenQuota(context.Background(), token.Id, 300))
	return path, QuotaRefund{ID: uuid.NewString(), TokenID: token.Id, UserID: user.Id, Amount: 300, RequestID: "request", Reason: "rejected_media"}
}

// assertQuotaRefundBalance checks both durable balances and the token's consumed
// counter, so partial transactions and duplicate credits cannot pass unnoticed.
func assertQuotaRefundBalance(t *testing.T, want int64) {
	t.Helper()
	var user User
	var token Token
	require.NoError(t, DB.First(&user, 101).Error)
	require.NoError(t, DB.First(&token, 102).Error)
	require.Equal(t, want, user.Quota)
	require.Equal(t, want, token.RemainQuota)
	require.Equal(t, int64(10000)-want, token.UsedQuota)
}

// TestQuotaRefundRecoveryDurable verifies a failed token credit rolls back the
// user credit and completion marker, then closes every pool and recovers through
// the startup worker from the on-disk intent alone. No request state is retained.
func TestQuotaRefundRecoveryDurable(t *testing.T) {
	path, intent := quotaRefundFixture(t)
	ctx := context.Background()
	require.NoError(t, EnsureQuotaRefund(ctx, intent))
	const hook = "refund:fail_token_credit"
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(hook, func(tx *gorm.DB) {
		if tx.Statement.Table == "tokens" {
			tx.AddError(errors.New("injected refund token write failure"))
		}
	}))
	persisted, err := RefundQuotaWithRecovery(ctx, intent)
	require.True(t, persisted)
	require.Error(t, err)
	assertQuotaRefundBalance(t, 9700)
	var pending QuotaRefund
	require.NoError(t, DB.Where("id = ?", intent.ID).Take(&pending).Error)
	require.Equal(t, QuotaRefundPending, pending.Status)
	require.Zero(t, pending.CompletedAt)
	pool, err := DB.DB()
	require.NoError(t, err)
	require.NoError(t, pool.Close())
	DB, err = gorm.Open(sqlite.Open(path), &gorm.Config{})
	require.NoError(t, err)
	// Startup, not a client retry, supplies the recovery trigger.
	workerCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(func() {
		cancel()
		waitCtx, stop := context.WithTimeout(ctx, time.Second)
		defer stop()
		require.NoError(t, WaitForBackgroundWorkers(waitCtx))
	})
	StartBackgroundWorker(workerCtx, RunQuotaRefundRecovery)
	require.Eventually(t, func() bool {
		var row QuotaRefund
		return DB.Where("id = ?", intent.ID).Take(&row).Error == nil && row.Status == QuotaRefundCompleted
	}, 3*time.Second, 10*time.Millisecond)
	cancel()
	waitCtx, stop := context.WithTimeout(ctx, time.Second)
	defer stop()
	require.NoError(t, WaitForBackgroundWorkers(waitCtx))
	assertQuotaRefundBalance(t, 10000)
	// Retrying a result whose COMMIT acknowledgement was lost is a no-op.
	require.NoError(t, ApplyQuotaRefund(ctx, intent.ID))
	assertQuotaRefundBalance(t, 10000)
}

// TestQuotaRefundConcurrentReplay verifies overlapping refund/recovery callers
// credit one ID once and cannot repurpose a completed ID for another amount.
func TestQuotaRefundConcurrentReplay(t *testing.T) {
	_, intent := quotaRefundFixture(t)
	ctx := context.Background()
	require.NoError(t, EnsureQuotaRefund(ctx, intent))
	var wg sync.WaitGroup
	results := make(chan error, 20)
	for i := 0; i < cap(results); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- ApplyQuotaRefund(ctx, intent.ID)
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	assertQuotaRefundBalance(t, 10000)
	require.NoError(t, EnsureQuotaRefund(ctx, intent))
	altered := intent
	altered.Amount++
	require.Error(t, EnsureQuotaRefund(ctx, altered))
	require.NoError(t, RecoverQuotaRefunds(ctx))
	assertQuotaRefundBalance(t, 10000)
}

// TestQuotaRefundEnqueueRetry verifies a transient intent-store outage reuses
// the same ID, creates exactly one durable row, and never loses the refund.
func TestQuotaRefundEnqueueRetry(t *testing.T) {
	_, intent := quotaRefundFixture(t)
	var failed atomic.Bool
	const hook = "refund:enqueue_once"
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(hook, func(tx *gorm.DB) {
		if tx.Statement.Table == "quota_refunds" && failed.CompareAndSwap(false, true) {
			tx.AddError(errors.New("injected refund enqueue failure"))
		}
	}))
	persisted, err := RefundQuotaWithRecovery(context.Background(), intent)
	require.NoError(t, err)
	require.True(t, persisted)
	require.True(t, failed.Load())
	var count int64
	require.NoError(t, DB.Model(&QuotaRefund{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
	assertQuotaRefundBalance(t, 10000)
}

// TestQuotaRefundDeadlineAndOwnership checks cancellation, invalid intent fields
// and a changed token owner leave balances untouched and an existing refund pending.
func TestQuotaRefundDeadlineAndOwnership(t *testing.T) {
	_, intent := quotaRefundFixture(t)
	ctx := context.Background()
	for _, mutate := range []func(*QuotaRefund){
		func(q *QuotaRefund) { q.ID = "invalid" },
		func(q *QuotaRefund) { q.Amount = 0 },
		func(q *QuotaRefund) { q.Amount = -1 },
		func(q *QuotaRefund) { q.UserID++ },
	} {
		bad := intent
		mutate(&bad)
		require.Error(t, EnsureQuotaRefund(ctx, bad))
	}
	require.NoError(t, EnsureQuotaRefund(ctx, intent))
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err := RefundQuotaWithRecovery(cancelled, intent)
	require.Error(t, err)
	assertQuotaRefundBalance(t, 9700)
	require.NoError(t, DB.Model(&Token{}).Where("id = ?", intent.TokenID).Update("user_id", 999).Error)
	require.Error(t, ApplyQuotaRefund(ctx, intent.ID))
	assertQuotaRefundBalance(t, 9700)
	require.NoError(t, DB.Model(&Token{}).Where("id = ?", intent.TokenID).Update("user_id", intent.UserID).Error)
	require.NoError(t, RecoverQuotaRefunds(ctx))
	assertQuotaRefundBalance(t, 10000)
}

// TestQuotaRefundRecoveryDefersPoison verifies a persistent error stays pending
// with a retry deadline while a later healthy intent is settled in the same sweep.
func TestQuotaRefundRecoveryDefersPoison(t *testing.T) {
	_, poison := quotaRefundFixture(t)
	ctx := context.Background()
	require.NoError(t, EnsureQuotaRefund(ctx, poison))
	good := poison
	good.ID = uuid.NewString()
	good.Amount = 100
	require.NoError(t, PostConsumeTokenQuota(ctx, good.TokenID, good.Amount))
	require.NoError(t, EnsureQuotaRefund(ctx, good))
	// A removed original owner cannot be silently reassigned during recovery.
	require.NoError(t, DB.Model(&QuotaRefund{}).Where("id = ?", poison.ID).Update("user_id", 999).Error)
	require.Error(t, RecoverQuotaRefunds(ctx))
	assertQuotaRefundBalance(t, 9700)
	var row QuotaRefund
	require.NoError(t, DB.Where("id = ?", poison.ID).Take(&row).Error)
	require.Equal(t, QuotaRefundPending, row.Status)
	require.Equal(t, int64(1), row.Attempts)
	require.Greater(t, row.RetryAt, time.Now().Unix())
	require.NoError(t, DB.Model(&QuotaRefund{}).Where("id = ?", poison.ID).
		Updates(map[string]any{"user_id": poison.UserID, "retry_at": 0}).Error)
	require.NoError(t, RecoverQuotaRefunds(ctx))
	assertQuotaRefundBalance(t, 10000)
}
