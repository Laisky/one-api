package model

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// refundDeferralContextKey identifies a trace value used to verify that bounded
// deferral keeps context values without inheriting the expired sweep deadline.
type refundDeferralContextKey struct{}

// TestQuotaRefundDeferralSurvivesSweepDeadline reproduces a limited-token write
// blocked until the sweep deadline. It verifies durable deferral, preserved
// balances, and progress for a later healthy refund while the first stays blocked.
func TestQuotaRefundDeferralSurvivesSweepDeadline(t *testing.T) {
	_, poison := quotaRefundFixture(t)
	ctx := context.WithValue(context.Background(), refundDeferralContextKey{}, "refund-trace")
	require.NoError(t, EnsureQuotaRefund(ctx, poison))
	// The healthy unlimited token still debits its user, but requires no token
	// balance UPDATE. It can progress while the limited-token write is blocked.
	good := poison
	good.ID, good.TokenID, good.Amount = uuid.NewString(), 103, 100
	require.NoError(t, DB.Create(&Token{Id: good.TokenID, UserId: good.UserID,
		Key: "refund-healthy-unlimited", UnlimitedQuota: true}).Error)
	require.NoError(t, PostConsumeTokenQuota(ctx, good.TokenID, good.Amount))
	require.NoError(t, EnsureQuotaRefund(ctx, good))
	require.NoError(t, DB.Model(&QuotaRefund{}).Where("id = ?", poison.ID).Update("retry_at", -2).Error)
	require.NoError(t, DB.Model(&QuotaRefund{}).Where("id = ?", good.ID).Update("retry_at", -1).Error)

	var outage atomic.Bool
	outage.Store(true)
	var blocked atomic.Int32
	var deferralContext context.Context
	var deferralEntryErr error
	var deferralDeadline time.Duration
	const hook = "refund:expire_sweep_before_deferral"
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(hook, func(tx *gorm.DB) {
		if tx.Statement.Table == "tokens" && outage.Load() {
			blocked.Add(1)
			select {
			case <-tx.Statement.Context.Done():
				tx.AddError(errors.Wrap(tx.Statement.Context.Err(), "injected token lock deadline"))
			case <-time.After(2 * time.Second):
				tx.AddError(errors.New("refund fixture missed its sweep deadline"))
			}
		}
		updates, ok := tx.Statement.Dest.(map[string]any)
		if tx.Statement.Table == "quota_refunds" && ok && updates["retry_at"] != nil {
			deferralContext = tx.Statement.Context
			deferralEntryErr = deferralContext.Err()
			if deadline, ok := deferralContext.Deadline(); ok {
				deferralDeadline = time.Until(deadline)
			}
		}
	}))
	t.Cleanup(func() { require.NoError(t, DB.Callback().Update().Remove(hook)) })

	passCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, RecoverQuotaRefunds(passCtx), context.DeadlineExceeded)
	require.Equal(t, int32(1), blocked.Load())
	var pending QuotaRefund
	require.NoError(t, DB.Where("id = ?", poison.ID).Take(&pending).Error)
	require.Equal(t, QuotaRefundPending, pending.Status)
	require.Equal(t, int64(1), pending.Attempts)
	require.Greater(t, pending.RetryAt, time.Now().Unix())
	require.NotNil(t, deferralContext)
	require.NoError(t, deferralEntryErr)
	require.Greater(t, deferralDeadline, time.Duration(0))
	require.LessOrEqual(t, deferralDeadline, time.Second)
	require.Equal(t, "refund-trace", deferralContext.Value(refundDeferralContextKey{}))
	require.ErrorIs(t, deferralContext.Err(), context.Canceled, "release the cleanup context immediately after its write")

	var user User
	require.NoError(t, DB.First(&user, poison.UserID).Error)
	require.Equal(t, int64(9600), user.Quota, "failed token credit must roll back the user credit")
	nextCtx, stopNext := context.WithTimeout(ctx, time.Second)
	defer stopNext()
	require.NoError(t, RecoverQuotaRefunds(nextCtx))
	require.Equal(t, int32(1), blocked.Load(), "the timed-out head must not consume the next sweep")
	pending = QuotaRefund{}
	require.NoError(t, DB.Where("id = ?", good.ID).Take(&pending).Error)
	require.Equal(t, QuotaRefundCompleted, pending.Status)
	pending = QuotaRefund{}
	require.NoError(t, DB.Where("id = ?", poison.ID).Take(&pending).Error)
	require.Equal(t, QuotaRefundPending, pending.Status)
	assertQuotaRefundBalance(t, 9700)

	outage.Store(false)
	require.NoError(t, DB.Model(&QuotaRefund{}).Where("id = ?", poison.ID).Update("retry_at", 0).Error)
	require.NoError(t, RecoverQuotaRefunds(ctx))
	require.NoError(t, RecoverQuotaRefunds(ctx))
	assertQuotaRefundBalance(t, 10000)
}

// TestQuotaRefundDeferralDeadlineBoundsCleanup simulates a second blocked write
// during deferral. Its fresh context must be usable yet bounded, and neither an
// expired sweep nor failed deferral may complete or credit the pending refund.
func TestQuotaRefundDeferralDeadlineBoundsCleanup(t *testing.T) {
	_, intent := quotaRefundFixture(t)
	ctx := context.Background()
	require.NoError(t, EnsureQuotaRefund(ctx, intent))
	var cleanupEntered bool
	var entryErr error
	var cleanupBudget time.Duration
	var backupExpired bool
	const hook = "refund:block_deferral_write"
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(hook, func(tx *gorm.DB) {
		updates, ok := tx.Statement.Dest.(map[string]any)
		if tx.Statement.Table == "tokens" {
			select {
			case <-tx.Statement.Context.Done():
				tx.AddError(errors.Wrap(tx.Statement.Context.Err(), "injected token deadline"))
			case <-time.After(2 * time.Second):
				tx.AddError(errors.New("refund fixture missing token deadline"))
			}
		}
		if tx.Statement.Table != "quota_refunds" || !ok || updates["retry_at"] == nil {
			return
		}
		cleanupEntered, entryErr = true, tx.Statement.Context.Err()
		if deadline, ok := tx.Statement.Context.Deadline(); ok {
			cleanupBudget = time.Until(deadline)
		}
		select {
		case <-tx.Statement.Context.Done():
			tx.AddError(errors.Wrap(tx.Statement.Context.Err(), "injected deferral deadline"))
		case <-time.After(2 * time.Second):
			backupExpired = true
			tx.AddError(errors.New("refund deferral exceeded its cleanup bound"))
		}
	}))
	t.Cleanup(func() { require.NoError(t, DB.Callback().Update().Remove(hook)) })
	passCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, RecoverQuotaRefunds(passCtx), context.DeadlineExceeded)
	require.True(t, cleanupEntered)
	require.NoError(t, entryErr)
	require.Greater(t, cleanupBudget, time.Duration(0))
	require.LessOrEqual(t, cleanupBudget, time.Second)
	require.False(t, backupExpired)
	var pending QuotaRefund
	require.NoError(t, DB.Where("id = ?", intent.ID).Take(&pending).Error)
	require.Equal(t, QuotaRefundPending, pending.Status)
	require.Zero(t, pending.CompletedAt)
	assertQuotaRefundBalance(t, 9700)
}
