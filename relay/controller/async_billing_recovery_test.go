package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/asyncvideo"
	"github.com/Laisky/one-api/relay/meta"
)

// TestAsyncBillingTemporaryResolverFailureResumesPolling checks that a temporary
// DB/credential lookup failure neither strands known work nor releases its debit.
func TestAsyncBillingTemporaryResolverFailureResumesPolling(t *testing.T) {
	muapiTaskSetup(t, 10000000)
	server := newMuapiTaskServer(t)
	_, task := muapiSubmitTask(t, server, "resolver-recovery")
	muapiRunWorkerStep(t)
	require.NoError(t, dbmodel.DB.Model(&dbmodel.AsyncTask{}).Where("id = ?", task.ID).Update("next_poll_at", 0).Error)
	worked, err := asyncvideo.ProcessOne(context.Background(), func(context.Context, *dbmodel.AsyncTask) (asyncvideo.Provider, *meta.Meta, error) {
		return nil, nil, errors.New("temporary database outage")
	}, time.Now())
	require.NoError(t, err)
	require.True(t, worked)
	saved, err := dbmodel.GetOwnedAsyncTask(context.Background(), task.ID, task.UserID, task.UserUUID)
	require.NoError(t, err)
	require.Equal(t, dbmodel.AsyncTaskQueued, saved.State)
	require.Equal(t, dbmodel.AsyncBillingHeld, saved.BillingState)
	require.EqualValues(t, 9800000, reloadUserQuota(t))
	require.Greater(t, saved.NextPollAt, time.Now().UnixMilli())
	muapiRunWorkerStep(t)
	saved, err = dbmodel.GetOwnedAsyncTask(context.Background(), task.ID, task.UserID, task.UserUUID)
	require.NoError(t, err)
	require.Equal(t, dbmodel.AsyncTaskCompleted, saved.State)
	require.EqualValues(t, 1, server.creates.Load())
	require.EqualValues(t, 1, server.polls.Load())
}

// TestAsyncBillingMalformedOutputRetainsObservedCost exercises real provider
// parsing through the worker. Bad output/status cannot hide a known higher bill
// or publish a result before the financial transaction commits successfully.
func TestAsyncBillingMalformedOutputRetainsObservedCost(t *testing.T) {
	for _, body := range []string{
		`{"id":"muapi-job-1","status":"completed","outputs":[{}],"cost":{"amount_usd":0.8}}`,
		`{"id":"muapi-job-1","status":{},"cost":{"amount_usd":0.8}}`,
		`{"id":"muapi-job-1","status":"completed","outputs":[],"cost":{"amount_usd":0.8}}`,
		`{"id":"muapi-job-1","status":"completed","outputs":["file:///private"],"cost":{"amount_usd":0.8}}`,
		`{"id":"muapi-job-1","status":"unexpected","cost":{"amount_usd":0.8}}`,
		`{"id":"muapi-job-1","status":"running","cost":{"amount_usd":0.8,"refunded":true}}`,
	} {
		t.Run(body, func(t *testing.T) {
			muapiTaskSetup(t, 200000)
			server := newMuapiTaskServer(t)
			server.poll.Store(body)
			_, task := muapiSubmitTask(t, server, "malformed-cost")
			muapiRunWorkerStep(t)
			muapiRunWorkerStep(t)
			saved, err := dbmodel.GetOwnedAsyncTask(context.Background(), task.ID, task.UserID, task.UserUUID)
			require.NoError(t, err)
			require.EqualValues(t, -200000, reloadUserQuota(t))
			require.EqualValues(t, 400000, saved.Quota)
			require.Empty(t, saved.ResultJSON)
			require.Equal(t, dbmodel.AsyncBillingHeld, saved.BillingState)
		})
	}
}

// TestAsyncBillingUnknownSubmissionStillCollectsKnownCost retains an actual
// submit charge even when no usable provider task ID was returned.
func TestAsyncBillingUnknownSubmissionStillCollectsKnownCost(t *testing.T) {
	muapiTaskSetup(t, 200000)
	server := newMuapiTaskServer(t)
	server.submit.Store(`{"status":"processing","cost":{"amount_usd":0.8}}`)
	_, task := muapiSubmitTask(t, server, "unknown-charge")
	muapiRunWorkerStep(t)
	saved, err := dbmodel.GetOwnedAsyncTask(context.Background(), task.ID, task.UserID, task.UserUUID)
	require.NoError(t, err)
	require.Equal(t, dbmodel.AsyncTaskUnknown, saved.State)
	require.EqualValues(t, 400000, saved.Quota)
	require.EqualValues(t, -200000, reloadUserQuota(t))
	worked, err := asyncvideo.ProcessOne(context.Background(), ResolveAsyncVideoProvider, time.Now().Add(time.Hour))
	require.NoError(t, err)
	require.False(t, worked)
	require.EqualValues(t, 1, server.creates.Load())
}

// TestAsyncBillingSupplementFailureWithholdsResult injects a SQL failure after
// upstream completion. The result is not visible until recovery collects the
// supplemental charge atomically; repeated polling cannot multiply that charge.
func TestAsyncBillingSupplementFailureWithholdsResult(t *testing.T) {
	muapiTaskSetup(t, 200000)
	server := newMuapiTaskServer(t)
	server.poll.Store(`{"id":"muapi-job-1","status":"completed","outputs":["https://media.example/done.mp4"],"cost":{"amount_usd":0.8}}`)
	_, task := muapiSubmitTask(t, server, "supplement-failure")
	muapiRunWorkerStep(t)
	require.NoError(t, dbmodel.DB.Model(&dbmodel.AsyncTask{}).Where("id = ?", task.ID).Update("next_poll_at", 0).Error)
	require.NoError(t, dbmodel.DB.Callback().Update().Before("gorm:update").Register("test:debit_outage", func(tx *gorm.DB) {
		if tx.Statement.Table == "tokens" {
			tx.AddError(errors.New("debit outage"))
		}
	}))
	_, err := asyncvideo.ProcessOne(context.Background(), ResolveAsyncVideoProvider, time.Now())
	require.Error(t, err)
	require.NoError(t, dbmodel.DB.Callback().Update().Remove("test:debit_outage"))
	saved, err := dbmodel.GetOwnedAsyncTask(context.Background(), task.ID, task.UserID, task.UserUUID)
	require.NoError(t, err)
	require.Empty(t, saved.ResultJSON)
	require.Equal(t, dbmodel.AsyncTaskQueued, saved.State)
	require.EqualValues(t, 0, reloadUserQuota(t))
	require.EqualValues(t, 200000, saved.Quota)
	require.NoError(t, dbmodel.DB.Model(&dbmodel.AsyncTask{}).Where("id = ?", task.ID).Update("lease_until", 0).Error)
	muapiRunWorkerStep(t)
	saved, err = dbmodel.GetOwnedAsyncTask(context.Background(), task.ID, task.UserID, task.UserUUID)
	require.NoError(t, err)
	require.Equal(t, dbmodel.AsyncTaskCompleted, saved.State)
	require.NotEmpty(t, saved.ResultJSON)
	require.EqualValues(t, -200000, reloadUserQuota(t))
	require.EqualValues(t, 1, server.creates.Load())
}
