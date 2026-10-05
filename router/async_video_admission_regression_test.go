package router

import (
	"errors"
	"io"
	"strings"
	"testing"

	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/model"
)

// TestAsyncVideoRejectedAdmissionHasNoPhantomTask drives shipped authentication,
// pricing and reservation. Rejected requests cannot advertise rolled-back tasks.
func TestAsyncVideoRejectedAdmissionHasNoPhantomTask(t *testing.T) {
	for _, path := range []string{"/v1/async/videos", "/v1/videos/generations"} {
		for _, fault := range []string{"owner_quota", "token_quota", "task_insert"} {
			t.Run(path+"/"+fault, func(t *testing.T) {
				var quotes, generations atomic.Int32
				provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, _ = io.Copy(io.Discard, r.Body)
					w.Header().Set("Content-Type", "application/json")
					if strings.HasSuffix(r.URL.Path, "/estimate-cost") {
						quotes.Add(1)
						_, _ = io.WriteString(w, `{"cost":0.4,"currency":"USD"}`)
						return
					}
					generations.Add(1)
					_, _ = io.WriteString(w, `{"request_id":"unexpected-paid-job"}`)
				}))
				defer provider.Close()
				engine, key, userID, tokenID, _ := disconnectBillingRouter(t, provider.URL)
				client.HTTPClient = provider.Client()
				wantUser, wantToken, status := int64(200000), int64(200000), http.StatusForbidden
				switch fault {
				case "owner_quota":
					wantUser = 1
					require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", userID).Update("quota", wantUser).Error)
				case "token_quota":
					wantToken = 1
					require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).Update("remain_quota", wantToken).Error)
				case "task_insert":
					status = http.StatusServiceUnavailable
					const callback = "reject_async_task_insert"
					require.NoError(t, model.DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
						if tx.Statement.Table == "async_tasks" {
							tx.AddError(errors.New("injected admission insert failure"))
						}
					}))
					t.Cleanup(func() { require.NoError(t, model.DB.Callback().Create().Remove(callback)) })
				}
				for attempt := range 2 {
					req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"veo3-fast","duration":5}`))
					req.Header.Set("Authorization", "Bearer sk-"+key)
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("Idempotency-Key", "rejected-job")
					recorder := httptest.NewRecorder()
					engine.ServeHTTP(recorder, req)
					require.Equal(t, status, recorder.Code, recorder.Body.String())
					var response struct {
						Error map[string]any `json:"error"`
					}
					require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
					require.NotContains(t, response.Error, "task_id", "an uncommitted UUID is not a resumable task, attempt %d", attempt)
					require.Empty(t, recorder.Header().Get("X-Async-Task-Id"))
					require.Empty(t, recorder.Header().Get("Location"))
				}
				var count int64
				require.NoError(t, model.DB.Model(&model.AsyncTask{}).Count(&count).Error)
				require.Zero(t, count)
				var owner model.User
				var token model.Token
				require.NoError(t, model.DB.First(&owner, userID).Error)
				require.NoError(t, model.DB.First(&token, tokenID).Error)
				require.EqualValues(t, wantUser, owner.Quota)
				require.EqualValues(t, wantToken, token.RemainQuota)
				require.Zero(t, token.UsedQuota)
				require.EqualValues(t, 2, quotes.Load())
				require.Zero(t, generations.Load(), "unpaid work must not be dispatched")
			})
		}
	}
}
