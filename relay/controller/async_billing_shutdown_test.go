package controller

import (
	"context"
	"testing"
	"time"

	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/asyncvideo"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/stretchr/testify/require"
)

// shutdownReceiptProvider cancels the process operation after the upstream has
// accepted the job or returned completion, just before durable receipt storage.
type shutdownReceiptProvider struct {
	cancel context.CancelFunc
}

// SubmitVideo returns an accepted receipt despite concurrent gateway shutdown.
func (p shutdownReceiptProvider) SubmitVideo(context.Context, *meta.Meta, []byte) (asyncvideo.Submission, error) {
	p.cancel()
	return asyncvideo.Submission{ID: "paid-before-shutdown"}, nil
}

// PollVideo returns completion despite concurrent gateway shutdown.
func (p shutdownReceiptProvider) PollVideo(context.Context, *meta.Meta, string) (asyncvideo.Observation, error) {
	p.cancel()
	return asyncvideo.Observation{State: dbmodel.AsyncTaskCompleted, Result: &asyncvideo.Result{Videos: []asyncvideo.Video{{URL: "https://media.example/done.mp4"}}}}, nil
}

// TestAsyncBillingShutdownPreservesReceipt injects cancellation between provider
// success and the DB write. Joined workers must persist using a bounded detached
// context; cancelling service I/O is not permission to discard known paid work.
func TestAsyncBillingShutdownPreservesReceipt(t *testing.T) {
	for _, completion := range []bool{false, true} {
		t.Run(map[bool]string{false: "submission", true: "completion"}[completion], func(t *testing.T) {
			muapiTaskSetup(t, 10000000)
			server := newMuapiTaskServer(t)
			_, task := muapiSubmitTask(t, server, "shutdown-receipt")
			if completion {
				muapiRunWorkerStep(t)
				require.NoError(t, dbmodel.DB.Model(&dbmodel.AsyncTask{}).Where("id = ?", task.ID).Update("next_poll_at", 0).Error)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			provider := shutdownReceiptProvider{cancel: cancel}
			worked, err := asyncvideo.ProcessOne(ctx, func(context.Context, *dbmodel.AsyncTask) (asyncvideo.Provider, *meta.Meta, error) {
				return provider, &meta.Meta{}, nil
			}, time.Now())
			require.True(t, worked)
			require.NoError(t, err)
			saved, err := dbmodel.GetOwnedAsyncTask(context.Background(), task.ID, task.UserID, task.UserUUID)
			require.NoError(t, err)
			require.EqualValues(t, 9800000, reloadUserQuota(t))
			if completion {
				require.Equal(t, dbmodel.AsyncTaskCompleted, saved.State)
				require.Equal(t, dbmodel.AsyncBillingSettled, saved.BillingState)
			} else {
				require.Equal(t, dbmodel.AsyncTaskQueued, saved.State)
				require.Equal(t, "paid-before-shutdown", saved.UpstreamID)
			}
		})
	}
}

// TestAsyncBillingActualProviderChargeCannotExceedDebit reproduces a provider's
// final cost being above its earlier estimate. Dynamic pricing must collect the
// difference before publishing success, even when the initial hold spent all funds.
func TestAsyncBillingActualProviderChargeCannotExceedDebit(t *testing.T) {
	muapiTaskSetup(t, 200000)
	server := newMuapiTaskServer(t)
	server.poll.Store(`{"id":"muapi-job-1","status":"completed","outputs":["https://media.example/video.mp4"],"cost":{"amount_usd":0.8,"refunded":false}}`)
	_, task := muapiSubmitTask(t, server, "actual-cost")
	muapiRunWorkerStep(t)
	muapiRunWorkerStep(t)
	require.EqualValues(t, -200000, reloadUserQuota(t), "an exhausted wallet must record debt instead of forgiving the additional provider cost")
	stored, err := dbmodel.GetOwnedAsyncTask(context.Background(), task.ID, task.UserID, task.UserUUID)
	require.NoError(t, err)
	require.EqualValues(t, 400000, stored.Quota)
}
