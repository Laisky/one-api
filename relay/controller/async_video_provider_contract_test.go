package controller

import (
	"context"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/asyncvideo"
	"github.com/Laisky/one-api/relay/meta"
)

// otherVideoProvider exercises the public interface without MuAPI HTTP, fields
// or status names. Counters are accessed serially through ProcessOne.
type otherVideoProvider struct {
	submits, polls int
	malformed      bool
}

// SubmitVideo supplies a receipt in the gateway's provider-independent contract.
func (p *otherVideoProvider) SubmitVideo(context.Context, *meta.Meta, []byte) (asyncvideo.Submission, error) {
	p.submits++
	return asyncvideo.Submission{ID: "another-provider-id"}, nil
}

// PollVideo returns a normalized result; the common layer rejects malformed data.
func (p *otherVideoProvider) PollVideo(context.Context, *meta.Meta, string) (asyncvideo.Observation, error) {
	p.polls++
	result := &asyncvideo.Result{Videos: []asyncvideo.Video{{URL: "https://other.example/video.mp4"}}}
	if p.malformed {
		result = nil
	}
	return asyncvideo.Observation{State: dbmodel.AsyncTaskCompleted, Result: result}, nil
}

// TestAsyncVideoSecondProviderUsesSameLifecycle validates the extensibility seam:
// another provider implementation uses the same persistence, accounting and DTO,
// and even its accidental empty completion cannot charge a successful result.
func TestAsyncVideoSecondProviderUsesSameLifecycle(t *testing.T) {
	muapiTaskSetup(t, 10000000)
	server := newMuapiTaskServer(t)
	_, task := muapiSubmitTask(t, server, "second-provider")
	provider := &otherVideoProvider{malformed: true}
	resolver := func(context.Context, *dbmodel.AsyncTask) (asyncvideo.Provider, *meta.Meta, error) {
		return provider, &meta.Meta{}, nil
	}
	for range 2 {
		require.NoError(t, dbmodel.DB.Model(&dbmodel.AsyncTask{}).Where("id = ?", task.ID).Update("next_poll_at", 0).Error)
		worked, err := asyncvideo.ProcessOne(context.Background(), resolver, time.Now())
		require.NoError(t, err)
		require.True(t, worked)
	}
	held, err := dbmodel.GetOwnedAsyncTask(context.Background(), task.ID, task.UserID, task.UserUUID)
	require.NoError(t, err)
	require.Equal(t, dbmodel.AsyncBillingHeld, held.BillingState)
	provider.malformed = false
	require.NoError(t, dbmodel.DB.Model(&dbmodel.AsyncTask{}).Where("id = ?", task.ID).Update("next_poll_at", 0).Error)
	_, err = asyncvideo.ProcessOne(context.Background(), resolver, time.Now())
	require.NoError(t, err)
	finished, err := dbmodel.GetOwnedAsyncTask(context.Background(), task.ID, task.UserID, task.UserUUID)
	require.NoError(t, err)
	dto, err := publicAsyncVideoTask(finished)
	require.NoError(t, err)
	require.Equal(t, "https://other.example/video.mp4", dto.Result.Videos[0].URL)
	require.Equal(t, 1, provider.submits)
	require.Equal(t, 2, provider.polls)
	require.Zero(t, server.creates.Load())
}

// TestAsyncVideoPersistenceFailureAfterAcceptance retains the original paid
// operation when the primary store fails AFTER the provider returns a task ID.
// The lost receipt becomes explicitly uncertain rather than a refundable failure.
func TestAsyncVideoPersistenceFailureAfterAcceptance(t *testing.T) {
	muapiTaskSetup(t, 10000000)
	server := newMuapiTaskServer(t)
	_, task := muapiSubmitTask(t, server, "lost-database")
	require.NoError(t, dbmodel.DB.Callback().Update().Before("gorm:update").Register("async_test_receipt_failure", func(tx *gorm.DB) {
		if tx.Statement.Table != "async_tasks" {
			return
		}
		if fields, ok := tx.Statement.Dest.(map[string]any); ok && fields["upstream_id"] != "" && fields["upstream_id"] != nil {
			tx.AddError(errors.New("injected whole-store write failure"))
		}
	}))
	worked, err := asyncvideo.ProcessOne(context.Background(), ResolveAsyncVideoProvider, time.Now())
	require.True(t, worked)
	require.Error(t, err)
	require.NoError(t, dbmodel.DB.Callback().Update().Remove("async_test_receipt_failure"))
	require.NoError(t, dbmodel.DB.Model(&dbmodel.AsyncTask{}).Where("id = ?", task.ID).Update("lease_until", 0).Error)
	worked, err = asyncvideo.ProcessOne(context.Background(), ResolveAsyncVideoProvider, time.Now())
	require.NoError(t, err)
	require.False(t, worked)
	stored, err := dbmodel.GetOwnedAsyncTask(context.Background(), task.ID, task.UserID, task.UserUUID)
	require.NoError(t, err)
	require.Equal(t, dbmodel.AsyncTaskUnknown, stored.State)
	require.Equal(t, dbmodel.AsyncBillingHeld, stored.BillingState)
	require.EqualValues(t, 1, server.creates.Load())
	require.EqualValues(t, 9800000, reloadUserQuota(t))
}
