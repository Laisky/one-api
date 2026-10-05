package router

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/asyncvideo"
	relaycontroller "github.com/Laisky/one-api/relay/controller"
)

// TestAsyncBillingHeaderChargeReachesWallet exercises the shipped synchronous
// route, a disconnected waiter, real provider HTTP, and physical accounting rows.
// A charge in headers must be collected even if no usable response body survives.
func TestAsyncBillingHeaderChargeReachesWallet(t *testing.T) {
	for _, stage := range []string{"submit", "submit_truncated", "submit_short_http", "poll", "poll_truncated"} {
		t.Run(stage, func(t *testing.T) {
			var creates, polls atomic.Int32
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				if strings.HasSuffix(r.URL.Path, "/estimate-cost") {
					_, _ = io.WriteString(w, `{"cost":0.4,"currency":"USD"}`)
					return
				}
				body := `{"request_id":"header-wallet-job"}`
				if r.Method == http.MethodPost {
					creates.Add(1)
					if strings.HasPrefix(stage, "submit") {
						w.Header().Set("X-MuAPI-Cost-USD", "0.800000")
					}
					if stage == "submit_truncated" {
						body = `{"request_id":`
					}
					if stage == "submit_truncated" || stage == "submit_short_http" {
						w.Header().Set("Content-Length", strconv.Itoa(len(body)+100))
					}
				} else {
					n := polls.Add(1)
					body = `{"id":"header-wallet-job","status":"completed","outputs":["https://media.example/headers.mp4"]}`
					if strings.HasPrefix(stage, "poll") && n == 1 {
						w.Header().Set("X-MuAPI-Cost-USD", "0.800000")
					}
					if stage == "poll_truncated" && n == 1 {
						body = `{"id":`
						w.Header().Set("Content-Length", "100")
					}
				}
				_, _ = io.WriteString(w, body)
			}))
			defer provider.Close()
			engine, key, userID, tokenID, ended := disconnectBillingRouter(t, provider.URL)
			client.HTTPClient = provider.Client()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			body := `{"model":"veo3-fast","duration":5,"prompt":"header billing"}`
			request := httptest.NewRequest(http.MethodPost, "/v1/videos/generations", strings.NewReader(body)).WithContext(ctx)
			request.Header.Set("Authorization", "Bearer sk-"+key)
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Idempotency-Key", "header-wallet")
			recorder := httptest.NewRecorder()
			done := make(chan struct{})
			go func() { defer close(done); engine.ServeHTTP(recorder, request) }()
			var task model.AsyncTask
			require.Eventually(t, func() bool { return model.DB.Where("user_id = ?", userID).Take(&task).Error == nil }, 3*time.Second, 10*time.Millisecond)
			cancel()
			require.ErrorIs(t, <-ended, context.Canceled)
			<-done
			step := func() {
				require.NoError(t, model.DB.Model(&model.AsyncTask{}).Where("id = ?", task.ID).Update("next_poll_at", 0).Error)
				worked, err := asyncvideo.ProcessOne(context.Background(), relaycontroller.ResolveAsyncVideoProvider, time.Now())
				require.NoError(t, err)
				require.True(t, worked)
			}
			step()
			if stage != "submit_truncated" {
				step()
			}
			require.NoError(t, model.DB.Where("id = ?", task.ID).Take(&task).Error)
			if stage == "poll_truncated" {
				require.Equal(t, model.AsyncBillingHeld, task.BillingState)
				require.Empty(t, task.ResultJSON)
				require.EqualValues(t, 400000, task.Quota, "bad output cannot hide an observed charge")
				step() // A later receipt omitting the cost must not erase the earlier amount.
			}
			require.NoError(t, model.DB.Where("id = ?", task.ID).Take(&task).Error)
			require.NoError(t, model.FlushAsyncTaskLogs(context.Background()))
			require.NoError(t, model.FlushAsyncTaskLogs(context.Background()))
			var user model.User
			var token model.Token
			var channel model.Channel
			require.NoError(t, model.DB.Where("id = ?", userID).Take(&user).Error)
			require.NoError(t, model.DB.Where("id = ?", tokenID).Take(&token).Error)
			require.NoError(t, model.DB.Where("id = ?", task.ChannelID).Take(&channel).Error)
			require.EqualValues(t, 400000, task.Quota)
			require.EqualValues(t, -200000, user.Quota)
			require.EqualValues(t, -200000, token.RemainQuota)
			require.EqualValues(t, 400000, token.UsedQuota)
			require.EqualValues(t, 1, creates.Load())
			if stage == "submit_truncated" {
				require.Equal(t, model.AsyncTaskUnknown, task.State)
				require.Equal(t, model.AsyncBillingHeld, task.BillingState)
				require.Zero(t, polls.Load())
				worked, err := asyncvideo.ProcessOne(context.Background(), relaycontroller.ResolveAsyncVideoProvider, time.Now().Add(time.Hour))
				require.NoError(t, err)
				require.False(t, worked)
			} else {
				require.Equal(t, model.AsyncTaskCompleted, task.State)
				require.Equal(t, model.AsyncBillingSettled, task.BillingState)
				require.EqualValues(t, 400000, user.UsedQuota)
				require.EqualValues(t, 400000, channel.UsedQuota)
				require.EqualValues(t, 1, user.RequestCount)
				replay := httptest.NewRequest(http.MethodPost, "/v1/videos/generations", strings.NewReader(body))
				replay.Header = request.Header.Clone()
				replay.Header.Set("Authorization", "Bearer sk-"+key) // Distributor rewrites the original upstream auth.
				rr := httptest.NewRecorder()
				engine.ServeHTTP(rr, replay)
				require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
				require.Contains(t, rr.Body.String(), "headers.mp4")
				require.EqualValues(t, 1, creates.Load())
			}
			var costs []model.UserRequestCost
			var logs []model.Log
			require.NoError(t, model.DB.Find(&costs).Error)
			require.NoError(t, model.LOG_DB.Find(&logs).Error)
			require.Len(t, costs, 1)
			require.Len(t, logs, 1)
			require.EqualValues(t, 400000, costs[0].Quota)
			require.EqualValues(t, 400000, logs[0].Quota)
		})
	}
}
