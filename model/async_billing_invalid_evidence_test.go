package model

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestAsyncBillingInvalidCostKeepsAcceptedIdentity proves malformed cost evidence
// cannot erase an otherwise valid provider receipt. Recovery polls the same job
// and collects a later validated charge, never converting acceptance to unknown.
func TestAsyncBillingInvalidCostKeepsAcceptedIdentity(t *testing.T) {
	_, input := durableJobTestStore(t)
	verifyAsyncBillingInvalidCostIdentity(t, input)
}

// TestAsyncBillingInvalidEvidenceLiveDatabases verifies the same identity and
// wallet invariants on actual MySQL and PostgreSQL transaction implementations.
func TestAsyncBillingInvalidEvidenceLiveDatabases(t *testing.T) {
	for _, backend := range []string{"mysql", "postgres"} {
		t.Run(backend, func(t *testing.T) { verifyAsyncBillingInvalidCostIdentity(t, asyncBillingLiveStore(t, backend)) })
	}
}

// verifyAsyncBillingInvalidCostIdentity checks a malformed provider observation,
// persisted receipt recovery and a single subsequent supplemental settlement.
func verifyAsyncBillingInvalidCostIdentity(t *testing.T, input *AsyncTask) {
	t.Helper()
	input.CostQuotaPerUSD = "500000"
	task := reserveDurableJob(t, input)
	ctx := context.Background()
	lease, err := ClaimAsyncTask(ctx, time.Now())
	require.NoError(t, err)
	require.NotNil(t, lease)
	require.Error(t, RecordAsyncTaskEvidence(ctx, lease, AsyncTaskUpdate{UpstreamID: "accepted-valid-id", CostUSD: "not-a-decimal"}))
	saved, err := GetOwnedAsyncTask(ctx, task.ID, task.UserID, task.UserUUID)
	require.NoError(t, err)
	require.Equal(t, "accepted-valid-id", saved.UpstreamID)
	require.Empty(t, saved.ObservedCostUSD)
	require.Equal(t, AsyncBillingHeld, saved.BillingState)
	user, token, _ := asyncBillingBalances(t, task)
	require.EqualValues(t, 800000, user.Quota)
	require.EqualValues(t, 800000, token.RemainQuota)
	require.NoError(t, DB.Model(&AsyncTask{}).Where("id = ?", task.ID).Update("lease_until", 0).Error)
	recovered, err := ClaimAsyncTask(ctx, time.Now())
	require.NoError(t, err)
	require.NotNil(t, recovered)
	require.Equal(t, AsyncTaskQueued, recovered.State)
	require.Equal(t, "accepted-valid-id", recovered.UpstreamID)
	update := AsyncTaskUpdate{State: AsyncTaskCompleted, CostUSD: "0.8", ResultJSON: `{"videos":[{"url":"https://media.example/final.mp4"}]}`}
	require.NoError(t, ApplyAsyncTaskUpdate(ctx, recovered, update))
	require.ErrorIs(t, ApplyAsyncTaskUpdate(ctx, recovered, update), ErrAsyncLeaseLost)
	user, token, _ = asyncBillingBalances(t, task)
	require.EqualValues(t, 600000, user.Quota)
	require.EqualValues(t, 600000, token.RemainQuota)
	require.EqualValues(t, 400000, user.UsedQuota)
}
