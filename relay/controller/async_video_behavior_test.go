package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/asyncvideo"
)

// TestAsyncVideoLostAcknowledgementNeverReplays reproduces an upstream accepting
// a paid request but returning a truncated JSON receipt. The full worker path
// retains funds and never calls creation again, even after repeated scans.
func TestAsyncVideoLostAcknowledgementNeverReplays(t *testing.T) {
	muapiTaskSetup(t, 10000000)
	server := newMuapiTaskServer(t)
	server.submit.Store(`{"request_id":`)
	c, task := muapiSubmitTask(t, server, "lost-ack")
	muapiRunWorkerStep(t)
	for range 3 {
		worked, err := asyncvideo.ProcessOne(context.Background(), ResolveAsyncVideoProvider, time.Now().UTC().Add(time.Hour))
		require.NoError(t, err)
		require.False(t, worked)
	}
	task, err := dbmodel.GetOwnedAsyncTask(context.Background(), task.ID, task.UserID, task.UserUUID)
	require.NoError(t, err)
	require.Equal(t, dbmodel.AsyncTaskUnknown, task.State)
	require.Equal(t, dbmodel.AsyncBillingHeld, task.BillingState)
	require.EqualValues(t, 9800000, reloadUserQuota(t))
	require.EqualValues(t, 1, server.creates.Load())
	replay, recorder := muapiVideoContext(t, http.MethodPost, "/v1/async/videos", `{"model":"alias","duration":5,"prompt":"a lighthouse"}`, server.URL, 9800000, fallbackUserID)
	replay.Request.Header.Set("Idempotency-Key", "lost-ack")
	ReplayAsyncVideoTask(replay)
	require.Equal(t, 202, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), task.ID)
	require.EqualValues(t, 1, server.estimates.Load(), "reattachment is before even a price quote")
	require.Equal(t, task.ID, c.GetString(asyncvideo.DurableTaskKey))
}

// TestAsyncVideoCrashFencesSubmitAndRecoversPolling simulates process loss by
// throwing away a lease owner and expiring its persisted lease. It distinguishes
// unsafe POST replay from safe GET polling recovery.
func TestAsyncVideoCrashFencesSubmitAndRecoversPolling(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		t.Run(map[bool]string{false: "during submission", true: "during polling"}[accepted], func(t *testing.T) {
			muapiTaskSetup(t, 10000000)
			server := newMuapiTaskServer(t)
			_, task := muapiSubmitTask(t, server, "crash")
			if accepted {
				muapiRunWorkerStep(t)
				require.NoError(t, dbmodel.DB.Model(&dbmodel.AsyncTask{}).Where("id = ?", task.ID).Update("next_poll_at", 0).Error)
			}
			lease, err := dbmodel.ClaimAsyncTask(context.Background(), time.Now().UTC())
			require.NoError(t, err)
			require.NotNil(t, lease)
			require.NoError(t, dbmodel.DB.Model(&dbmodel.AsyncTask{}).Where("id = ?", task.ID).Update("lease_until", time.Now().Add(-time.Second).UnixMilli()).Error)
			worked, err := asyncvideo.ProcessOne(context.Background(), ResolveAsyncVideoProvider, time.Now().UTC())
			require.NoError(t, err)
			require.Equal(t, accepted, worked)
			task, err = dbmodel.GetOwnedAsyncTask(context.Background(), task.ID, task.UserID, task.UserUUID)
			require.NoError(t, err)
			if accepted {
				require.Equal(t, dbmodel.AsyncTaskCompleted, task.State)
				require.EqualValues(t, 1, server.creates.Load())
				require.EqualValues(t, 1, server.polls.Load())
			} else {
				require.Equal(t, dbmodel.AsyncTaskUnknown, task.State)
				require.Zero(t, server.creates.Load())
			}
			require.EqualValues(t, 9800000, reloadUserQuota(t))
			require.ErrorIs(t, dbmodel.ApplyAsyncTaskUpdate(context.Background(), lease, dbmodel.AsyncTaskUpdate{State: dbmodel.AsyncTaskFailed, Refund: true}), dbmodel.ErrAsyncLeaseLost)
		})
	}
}

// TestAsyncVideoRefundRequiresProviderConfirmation proves terminal failure alone
// is not refund evidence. A later confirmed refund restores the captured finite
// token even if its current unlimited flag changed; repeated polling is harmless.
func TestAsyncVideoRefundRequiresProviderConfirmation(t *testing.T) {
	muapiTaskSetup(t, 200000)
	server := newMuapiTaskServer(t)
	server.poll.Store(`{"id":"muapi-job-1","status":"failed","cost":{"refunded":false}}`)
	_, task := muapiSubmitTask(t, server, "refund")
	muapiRunWorkerStep(t)
	muapiRunWorkerStep(t)
	require.Zero(t, reloadUserQuota(t))
	persisted, err := dbmodel.GetOwnedAsyncTask(context.Background(), task.ID, task.UserID, task.UserUUID)
	require.NoError(t, err)
	require.Equal(t, dbmodel.AsyncTaskFailed, persisted.State)
	require.Equal(t, dbmodel.AsyncBillingHeld, persisted.BillingState)
	require.NoError(t, dbmodel.DB.Model(&dbmodel.Token{}).Where("id = ?", fallbackTokenID).Updates(map[string]any{"unlimited_quota": true, "status": dbmodel.TokenStatusExhausted}).Error)
	server.poll.Store(`{"id":"muapi-job-1","status":"failed","cost":{"refunded":true}}`)
	muapiRunWorkerStep(t)
	require.EqualValues(t, 200000, reloadUserQuota(t))
	var token dbmodel.Token
	require.NoError(t, dbmodel.DB.Where("id = ?", fallbackTokenID).Take(&token).Error)
	require.EqualValues(t, 200000, token.RemainQuota)
	require.Zero(t, token.UsedQuota)
	require.Equal(t, dbmodel.TokenStatusEnabled, token.Status)
	worked, err := asyncvideo.ProcessOne(context.Background(), ResolveAsyncVideoProvider, time.Now().Add(time.Hour))
	require.NoError(t, err)
	require.False(t, worked)
	require.EqualValues(t, 1, server.creates.Load())
	require.EqualValues(t, 2, server.polls.Load())
}

// TestAsyncVideoWaitTimeoutDisconnectAndSyncResult exercises the SAME admitted
// job through synchronous delivery. A wait timeout or cancelled HTTP connection
// does not cancel, refund, resubmit, or lose the task.
func TestAsyncVideoWaitTimeoutDisconnectAndSyncResult(t *testing.T) {
	muapiTaskSetup(t, 10000000)
	server := newMuapiTaskServer(t)
	_, task := muapiSubmitTask(t, server, "wait")
	c, timed := muapiVideoContext(t, http.MethodPost, "/v1/videos/generations", "{}", server.URL, 10000000, fallbackUserID)
	deliverAsyncVideoTask(c, task, true, 5*time.Millisecond)
	require.Equal(t, 504, timed.Code)
	require.Contains(t, timed.Body.String(), task.ID)
	require.Equal(t, "/v1/async/videos/"+task.ID, timed.Header().Get("Location"))
	cancelled, writer := muapiVideoContext(t, http.MethodPost, "/v1/videos/generations", "{}", server.URL, 10000000, fallbackUserID)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cancelled.Request = cancelled.Request.WithContext(ctx)
	deliverAsyncVideoTask(cancelled, task, true, time.Second)
	require.Zero(t, writer.Body.Len())
	require.EqualValues(t, 9800000, reloadUserQuota(t))
	muapiRunWorkerStep(t)
	completed := make(chan error, 1)
	go func() {
		// A separate worker completes after synchronous waiting has begun. It only
		// uses immutable fixture configuration, and the goroutine is joined below.
		time.Sleep(20 * time.Millisecond)
		err := dbmodel.DB.Model(&dbmodel.AsyncTask{}).Where("id = ?", task.ID).Update("next_poll_at", 0).Error
		if err == nil {
			_, err = asyncvideo.ProcessOne(context.Background(), ResolveAsyncVideoProvider, time.Now().UTC())
		}
		completed <- err
	}()
	syncRequest, result := muapiVideoContext(t, http.MethodPost, "/v1/videos/generations", "{}", server.URL, 10000000, fallbackUserID)
	deliverAsyncVideoTask(syncRequest, task, true, 2*time.Second)
	require.NoError(t, <-completed)
	require.Equal(t, 200, result.Code, result.Body.String())
	var payload struct {
		Data []asyncvideo.Video `json:"data"`
	}
	require.NoError(t, json.Unmarshal(result.Body.Bytes(), &payload))
	require.Len(t, payload.Data, 1)
	require.Equal(t, "https://media.example/video.mp4", payload.Data[0].URL)
	require.EqualValues(t, 1, server.creates.Load())
	require.EqualValues(t, 9800000, reloadUserQuota(t))
}

// TestAsyncVideoIdempotencyOwnershipAndModelPermissions checks stable delivery,
// conflict detection and current-token restrictions before channel/price work.
func TestAsyncVideoIdempotencyOwnershipAndModelPermissions(t *testing.T) {
	muapiTaskSetup(t, 10000000)
	server := newMuapiTaskServer(t)
	_, task := muapiSubmitTask(t, server, "same-job")
	for _, test := range []struct {
		name, body, models string
		owner              int
		only               bool
		status             int
	}{
		{"same params reordered", `{"duration":5,"model":"alias","prompt":"a lighthouse"}`, "", fallbackUserID, true, 202},
		{"changed params", `{"duration":5,"model":"alias","prompt":"different"}`, "", fallbackUserID, false, 409},
		{"restricted model", `{"duration":5,"model":"alias","prompt":"a lighthouse"}`, "other-model", fallbackUserID, false, 403},
		{"other spent user", `{"duration":5,"model":"alias","prompt":"a lighthouse"}`, "", fallbackUserID + 1, true, 403},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, w := muapiVideoContext(t, http.MethodPost, "/v1/async/videos", test.body, server.URL, 0, test.owner)
			c.Request.Header.Set("Idempotency-Key", "same-job")
			c.Set(ctxkey.AvailableModels, test.models)
			c.Set("async_task_reattach_only", test.only)
			ReplayAsyncVideoTask(c)
			require.Equal(t, test.status, w.Code, w.Body.String())
			if test.status == 202 {
				require.Contains(t, w.Body.String(), task.ID)
			}
		})
	}
	require.EqualValues(t, 1, server.estimates.Load())
	require.Zero(t, server.creates.Load())
}

// TestAsyncVideoConcurrentReservationHasOneOwner exercises real transaction and
// unique-key arbitration; only one reservation may be created and debited.
func TestAsyncVideoConcurrentReservationHasOneOwner(t *testing.T) {
	muapiTaskSetup(t, 10000000)
	server := newMuapiTaskServer(t)
	_, template := muapiSubmitTask(t, server, "template")
	// Use another receipt so concurrency, rather than a pre-existing row, chooses
	// the winner. A generous balance avoids obscuring uniqueness with quota errors.
	template.DedupKey = dbmodel.AsyncTaskDedupKey(template.UserID, template.UserUUID, "concurrent")
	var wg sync.WaitGroup
	var created atomic.Int32
	errs := make(chan error, 12)
	ids := make(chan string, 12)
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			copy := *template
			copy.ID = ""
			task, fresh, err := dbmodel.ReserveAsyncTask(context.Background(), &copy)
			errs <- err
			if err == nil {
				ids <- task.ID
			}
			if fresh {
				created.Add(1)
			}
		}()
	}
	wg.Wait()
	close(errs)
	close(ids)
	for err := range errs {
		require.NoError(t, err)
	}
	unique := map[string]bool{}
	for id := range ids {
		unique[id] = true
	}
	require.Len(t, unique, 1)
	require.EqualValues(t, 1, created.Load())
	require.EqualValues(t, 9600000, reloadUserQuota(t))
}

// TestAsyncVideoDatabaseUnavailableCannotSubmit proves whole-store unavailability
// fails admission before a paid POST; a second table in the same DB is not a WAL.
func TestAsyncVideoDatabaseUnavailableCannotSubmit(t *testing.T) {
	muapiTaskSetup(t, 10000000)
	server := newMuapiTaskServer(t)
	c, w := muapiVideoContext(t, http.MethodPost, "/v1/async/videos", `{"model":"alias","duration":5}`, server.URL, 10000000, fallbackUserID)
	saved := dbmodel.DB
	dbmodel.DB = nil
	// Restore before assertions so even a failed test cannot poison later cases.
	result := RelayAsyncVideoHelper(c, false)
	dbmodel.DB = saved
	require.Nil(t, result)
	require.Equal(t, 503, w.Code, w.Body.String())
	require.Zero(t, server.creates.Load())
	require.EqualValues(t, 10000000, reloadUserQuota(t))
}

// TestAsyncVideoMalformedPollAndDisabledChannelKeepTask distinguishes a bad
// observation from failure, and proves prepaid work remains retrievable after
// a channel is disabled without rerouting to another provider.
func TestAsyncVideoMalformedPollAndDisabledChannelKeepTask(t *testing.T) {
	muapiTaskSetup(t, 10000000)
	server := newMuapiTaskServer(t)
	_, task := muapiSubmitTask(t, server, "poll")
	muapiRunWorkerStep(t)
	server.poll.Store(`{"id":"wrong-task","status":"completed","outputs":["https://example.com/other.mp4"]}`)
	muapiRunWorkerStep(t)
	current, err := dbmodel.GetOwnedAsyncTask(context.Background(), task.ID, task.UserID, task.UserUUID)
	require.NoError(t, err)
	require.Equal(t, dbmodel.AsyncTaskQueued, current.State)
	require.Equal(t, dbmodel.AsyncBillingHeld, current.BillingState)
	require.Greater(t, current.NextPollAt, time.Now().UnixMilli())
	require.NoError(t, dbmodel.DB.Model(&dbmodel.Channel{}).Where("id = ?", fallbackChannelID).Update("status", dbmodel.ChannelStatusManuallyDisabled).Error)
	server.poll.Store(`{"id":"muapi-job-1","status":"completed","outputs":["https://media.example/video.mp4"]}`)
	muapiRunWorkerStep(t)
	c, w := muapiVideoContext(t, http.MethodGet, "/v1/async/videos/"+task.ID, "", server.URL, 0, fallbackUserID)
	c.Params = gin.Params{{Key: "video_id", Value: task.ID}}
	GetAsyncVideoTask(c)
	require.Equal(t, 200, w.Code)
	require.True(t, strings.Contains(w.Body.String(), `"status":"completed"`))
	require.EqualValues(t, 1, server.creates.Load())
}
