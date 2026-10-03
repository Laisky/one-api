package model

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// testAsyncBillingEvidenceRecovery checks the two-commit evidence boundary using
// actual database rows. The lease expires with evidence saved but accounting not
// committed; reclaiming cannot repeat generation or erase the known supplement.
func testAsyncBillingEvidenceRecovery(t *testing.T, input *AsyncTask, knownID bool) {
	t.Helper()
	ctx := context.Background()
	input.CostQuotaPerUSD = "500000"
	saved := reserveDurableJob(t, input)
	lease, err := ClaimAsyncTask(ctx, time.Now())
	require.NoError(t, err)
	require.NotNil(t, lease)
	evidence := AsyncTaskUpdate{CostUSD: "3.0"}
	if knownID {
		evidence.UpstreamID = "evidence-job"
	}
	require.NoError(t, RecordAsyncTaskEvidence(ctx, lease, evidence))
	// Replayed/lower evidence cannot erase the accepted identity or higher cost.
	evidence.CostUSD = "0.4"
	require.NoError(t, RecordAsyncTaskEvidence(ctx, lease, evidence))
	var pending AsyncTask
	require.NoError(t, DB.Where("id = ?", saved.ID).Take(&pending).Error)
	require.True(t, pending.EvidencePending)
	require.EqualValues(t, 2, pending.EvidenceVersion)
	require.Equal(t, "3.0", pending.ObservedCostUSD)
	user, _, _ := asyncBillingBalances(t, saved)
	require.EqualValues(t, 800000, user.Quota, "recording evidence alone cannot pretend a debit committed")
	require.Equal(t, AsyncBillingHeld, pending.BillingState)
	require.Empty(t, pending.ResultJSON)
	if knownID {
		require.Error(t, RecordAsyncTaskEvidence(ctx, lease, AsyncTaskUpdate{UpstreamID: "wrong-job", CostUSD: "9.0"}))
	}
	require.NoError(t, DB.Model(&AsyncTask{}).Where("id = ?", saved.ID).Update("lease_until", 0).Error)
	recovered, err := ClaimAsyncTask(ctx, time.Now())
	require.NoError(t, err)
	require.NotNil(t, recovered)
	expected := AsyncTaskUnknown
	if knownID {
		expected = AsyncTaskQueued
	}
	require.Equal(t, expected, recovered.State)
	require.ErrorIs(t, RecordAsyncTaskEvidence(ctx, lease, evidence), ErrAsyncLeaseLost)
	require.ErrorIs(t, ApplyAsyncTaskUpdate(ctx, lease, AsyncTaskUpdate{State: AsyncTaskFailed, Refund: true}), ErrAsyncLeaseLost)
	update := AsyncTaskUpdate{State: AsyncTaskUnknown}
	if knownID {
		update.State = AsyncTaskCompleted
		update.ResultJSON = `{"videos":[{"url":"https://media.example/evidence.mp4"}]}`
	}
	// No cost in the current observation: the durable evidence must drive collection.
	require.NoError(t, ApplyAsyncTaskUpdate(ctx, recovered, update))
	require.ErrorIs(t, ApplyAsyncTaskUpdate(ctx, recovered, update), ErrAsyncLeaseLost)
	require.NoError(t, DB.Where("id = ?", saved.ID).Take(&pending).Error)
	require.False(t, pending.EvidencePending)
	require.EqualValues(t, 1500000, pending.Quota)
	user, token, channel := asyncBillingBalances(t, saved)
	require.EqualValues(t, -500000, user.Quota)
	require.EqualValues(t, -500000, token.RemainQuota)
	require.EqualValues(t, 1500000, token.UsedQuota)
	if knownID {
		require.EqualValues(t, 1500000, user.UsedQuota)
		require.EqualValues(t, 1500000, channel.UsedQuota)
		require.EqualValues(t, 1, user.RequestCount)
	}
	require.NoError(t, FlushAsyncTaskLogs(ctx))
	require.NoError(t, FlushAsyncTaskLogs(ctx))
	var costs []UserRequestCost
	var logs []Log
	require.NoError(t, DB.Find(&costs).Error)
	require.NoError(t, LOG_DB.Find(&logs).Error)
	require.Len(t, costs, 1)
	require.Len(t, logs, 1)
	require.EqualValues(t, 1500000, costs[0].Quota)
	require.EqualValues(t, 1500000, logs[0].Quota)
	idle, err := ClaimAsyncTask(ctx, time.Now().Add(time.Hour))
	require.NoError(t, err)
	require.Nil(t, idle, "anonymous evidence must finish collection without spinning")
}

// TestAsyncBillingEvidenceRecovery covers replay, fencing, identity conflict and
// recovery on SQLite; the separate live suite runs these same assertions on SQL
// engines with different locking and RowsAffected behavior.
func TestAsyncBillingEvidenceRecovery(t *testing.T) {
	for _, knownID := range []bool{false, true} {
		name := "anonymous"
		if knownID {
			name = "accepted"
		}
		t.Run(name, func(t *testing.T) {
			_, input := durableJobTestStore(t)
			testAsyncBillingEvidenceRecovery(t, input, knownID)
		})
	}
}

// TestAsyncBillingEvidenceLiveDatabases verifies the new evidence/reconciliation
// transaction boundary on physical MySQL and PostgreSQL databases, not dialect
// mocks. Missing DSNs are failures when the required-backend flag is set.
func TestAsyncBillingEvidenceLiveDatabases(t *testing.T) {
	for _, backend := range []string{"mysql", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			for _, knownID := range []bool{false, true} {
				name := "anonymous"
				if knownID {
					name = "accepted"
				}
				t.Run(name, func(t *testing.T) {
					input := asyncBillingLiveStore(t, backend)
					testAsyncBillingEvidenceRecovery(t, input, knownID)
				})
			}
		})
	}
}

// TestAsyncBillingEvidenceLostCommitAcknowledgement proves evidence survives a
// committed write whose acknowledgement is lost, without a speculative refund.
func TestAsyncBillingEvidenceLostCommitAcknowledgement(t *testing.T) {
	_, input := durableJobTestStore(t)
	input.CostQuotaPerUSD = "500000"
	saved := reserveDurableJob(t, input)
	ctx := context.Background()
	lease, err := ClaimAsyncTask(ctx, time.Now())
	require.NoError(t, err)
	require.NotNil(t, lease)
	pool := injectAsyncLostCommit(t, DB)
	pool.lose.Store(true)
	require.Error(t, RecordAsyncTaskEvidence(ctx, lease, AsyncTaskUpdate{UpstreamID: "ack-job", CostUSD: "3.0"}))
	require.NoError(t, DB.Model(&AsyncTask{}).Where("id = ?", saved.ID).Update("lease_until", 0).Error)
	recovered, err := ClaimAsyncTask(ctx, time.Now())
	require.NoError(t, err)
	require.NotNil(t, recovered)
	require.Equal(t, "ack-job", recovered.UpstreamID)
	require.Equal(t, "3.0", recovered.ObservedCostUSD)
	require.NoError(t, ApplyAsyncTaskUpdate(ctx, recovered, AsyncTaskUpdate{State: AsyncTaskCompleted, ResultJSON: `{"videos":[{"url":"https://media.example/ack.mp4"}]}`}))
	user, token, channel := asyncBillingBalances(t, saved)
	require.EqualValues(t, -500000, user.Quota)
	require.EqualValues(t, -500000, token.RemainQuota)
	require.EqualValues(t, 1500000, user.UsedQuota)
	require.EqualValues(t, 1500000, token.UsedQuota)
	require.EqualValues(t, 1500000, channel.UsedQuota)
	require.EqualValues(t, 1, user.RequestCount)
}
