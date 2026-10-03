package model

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestAsyncTaskSettlementCannotReplaceAcceptedIdentity verifies the financial
// transaction itself enforces identity, independently of the evidence helper.
func TestAsyncTaskSettlementCannotReplaceAcceptedIdentity(t *testing.T) {
	for _, outcome := range []string{AsyncTaskQueued, AsyncTaskCompleted, AsyncTaskFailed} {
		t.Run(outcome, func(t *testing.T) {
			_, input := durableJobTestStore(t)
			verifyAsyncTaskSettlementIdentity(t, input, outcome)
		})
	}
}

// TestAsyncBillingIdentityLiveDatabases checks the byte-exact identity fence
// on actual SQL engines, including case-insensitive or space-padding collations.
func TestAsyncBillingIdentityLiveDatabases(t *testing.T) {
	for _, backend := range []string{"mysql", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			for _, outcome := range []string{AsyncTaskQueued, AsyncTaskCompleted, AsyncTaskFailed} {
				t.Run(outcome, func(t *testing.T) {
					verifyAsyncTaskSettlementIdentity(t, asyncBillingLiveStore(t, backend), outcome)
				})
			}
		})
	}
}

// verifyAsyncTaskSettlementIdentity checks rejected updates leave the receipt,
// wallet and lease unchanged, then closes a matching observation exactly once.
func verifyAsyncTaskSettlementIdentity(t *testing.T, input *AsyncTask, outcome string) {
	t.Helper()
	saved := reserveDurableJob(t, input)
	ctx := context.Background()
	lease, err := ClaimAsyncTask(ctx, time.Now())
	require.NoError(t, err)
	require.NotNil(t, lease)
	require.NoError(t, RecordAsyncTaskEvidence(ctx, lease, AsyncTaskUpdate{UpstreamID: "accepted-original"}))
	update := AsyncTaskUpdate{State: outcome, Refund: outcome == AsyncTaskFailed}
	if outcome == AsyncTaskCompleted {
		update.ResultJSON = `{"videos":[{"url":"https://media.example/result.mp4"}]}`
	}
	for _, wrongID := range []string{"other-job", "ACCEPTED-ORIGINAL", "accepted-original "} {
		update.UpstreamID = wrongID
		require.Error(t, ApplyAsyncTaskUpdate(ctx, lease, update), "a valid lease cannot change accepted identity to %q", wrongID)
		task, err := GetOwnedAsyncTask(ctx, saved.ID, saved.UserID, saved.UserUUID)
		require.NoError(t, err)
		require.Equal(t, "accepted-original", task.UpstreamID)
		require.Equal(t, AsyncBillingHeld, task.BillingState)
		require.Empty(t, task.ResultJSON)
		user, token, channel := asyncBillingBalances(t, saved)
		require.EqualValues(t, 800000, user.Quota)
		require.EqualValues(t, 800000, token.RemainQuota)
		require.Zero(t, user.UsedQuota)
		require.Zero(t, channel.UsedQuota)
	}
	// Matching identity remains a legal observation on the unchanged lease.
	update.UpstreamID = "accepted-original"
	require.NoError(t, ApplyAsyncTaskUpdate(ctx, lease, update))
	require.ErrorIs(t, ApplyAsyncTaskUpdate(ctx, lease, update), ErrAsyncLeaseLost)
	user, token, channel := asyncBillingBalances(t, saved)
	wantRemaining := int64(800000)
	if update.Refund {
		wantRemaining = 1000000
	}
	require.EqualValues(t, wantRemaining, user.Quota)
	require.EqualValues(t, wantRemaining, token.RemainQuota)
	if outcome == AsyncTaskCompleted {
		require.EqualValues(t, 200000, user.UsedQuota)
		require.EqualValues(t, 200000, channel.UsedQuota)
	}
	require.NoError(t, FlushAsyncTaskLogs(ctx))
	require.NoError(t, FlushAsyncTaskLogs(ctx))
	var logs int64
	require.NoError(t, LOG_DB.Model(&Log{}).Where("request_id = ?", input.RequestID).Count(&logs).Error)
	require.EqualValues(t, 1, logs)
}
