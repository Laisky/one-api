package controller

import (
	"context"
	"testing"
	"time"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/asyncvideo"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/stretchr/testify/require"
)

// auditPartialProvider models a second provider that returns an early preview
// before final cost is available. All bookkeeping remains in the shared worker.
type auditPartialProvider struct{ calls int }

// SubmitVideo returns one accepted job without any provider-specific payload.
func (p *auditPartialProvider) SubmitVideo(context.Context, *meta.Meta, []byte) (asyncvideo.Submission, error) {
	return asyncvideo.Submission{ID: "partial-job"}, nil
}

// PollVideo exposes a preview once, then final output and its larger charge.
func (p *auditPartialProvider) PollVideo(context.Context, *meta.Meta, string) (asyncvideo.Observation, error) {
	p.calls++
	state, cost := model.AsyncTaskRunning, ""
	if p.calls > 1 {
		state, cost = model.AsyncTaskCompleted, "0.8"
	}
	return asyncvideo.Observation{State: state, CostUSD: cost,
		Result: &asyncvideo.Result{Videos: []asyncvideo.Video{{URL: "https://media.example/final.mp4"}}}}, nil
}

// TestAsyncVideoWorkerWithholdsPartialResults exercises real reservation,
// worker polling and accounting: a running preview is not a successful result,
// and the final result is published only with the supplemental charge committed.
func TestAsyncVideoWorkerWithholdsPartialResults(t *testing.T) {
	muapiTaskSetup(t, 10000000)
	server := newMuapiTaskServer(t)
	_, task := muapiSubmitTask(t, server, "partial-result")
	provider := &auditPartialProvider{}
	resolver := func(context.Context, *model.AsyncTask) (asyncvideo.Provider, *meta.Meta, error) {
		return provider, &meta.Meta{}, nil
	}
	for step := range 3 {
		require.NoError(t, model.DB.Model(&model.AsyncTask{}).Where("id = ?", task.ID).Update("next_poll_at", 0).Error)
		worked, err := asyncvideo.ProcessOne(context.Background(), resolver, time.Now())
		require.NoError(t, err)
		require.True(t, worked)
		saved, err := model.GetOwnedAsyncTask(context.Background(), task.ID, task.UserID, task.UserUUID)
		require.NoError(t, err)
		dto, err := publicAsyncVideoTask(saved)
		require.NoError(t, err)
		if step < 2 {
			require.Empty(t, saved.ResultJSON, "previews must not become durable public results")
			require.Nil(t, dto.Result)
			require.Equal(t, model.AsyncBillingHeld, saved.BillingState)
			require.EqualValues(t, 9800000, reloadUserQuota(t))
		} else {
			require.NotNil(t, dto.Result)
			require.Equal(t, model.AsyncBillingSettled, saved.BillingState)
			require.EqualValues(t, 9600000, reloadUserQuota(t))
		}
	}
}
