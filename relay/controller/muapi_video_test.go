package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/asyncvideo"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// muapiTaskSetup resets real physical balances and durable tables. It builds on
// the existing video fixtures while supplying unexpired, immutable identities.
func muapiTaskSetup(t *testing.T, balance int64) {
	t.Helper()
	xaiVideoSetup(t, balance, false)
	require.NoError(t, dbmodel.DB.AutoMigrate(&dbmodel.AsyncTask{}))
	require.NoError(t, dbmodel.LOG_DB.AutoMigrate(&dbmodel.AsyncTaskLogReceipt{}))
	require.NoError(t, dbmodel.DB.Where("1=1").Delete(&dbmodel.AsyncTask{}).Error)
	require.NoError(t, dbmodel.LOG_DB.Where("1=1").Delete(&dbmodel.AsyncTaskLogReceipt{}).Error)
	require.NoError(t, dbmodel.DB.Model(&dbmodel.User{}).Where("id = ?", fallbackUserID).Updates(map[string]any{"uuid": "00000000-0000-4000-8000-000000000001", "used_quota": 0, "request_count": 0}).Error)
	require.NoError(t, dbmodel.DB.Model(&dbmodel.Token{}).Where("id = ?", fallbackTokenID).Updates(map[string]any{"uuid": "00000000-0000-4000-8000-000000000002", "expired_time": -1, "status": dbmodel.TokenStatusEnabled}).Error)
	require.NoError(t, dbmodel.DB.Model(&dbmodel.Channel{}).Where("id = ?", fallbackChannelID).Updates(map[string]any{"uuid": "00000000-0000-4000-8000-000000000003", "type": channeltype.MuAPI, "used_quota": 0, "key": "muapi-fixture-key"}).Error)
}

// muapiVideoContext creates a routed MuAPI request with the same identities as
// authentication and distribution. No credentials are persisted in the job.
func muapiVideoContext(t *testing.T, method, path, body, base string, balance int64, userID int) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	var channel dbmodel.Channel
	require.NoError(t, dbmodel.DB.Where("id = ?", fallbackChannelID).Take(&channel).Error)
	require.NoError(t, dbmodel.DB.Model(&dbmodel.Channel{}).Where("id = ?", channel.Id).Update("base_url", base).Error)
	channel.BaseURL = &base
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	gmw.SetLogger(c, logger.Logger)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	for key, value := range map[string]any{
		ctxkey.Channel: channeltype.MuAPI, ctxkey.ChannelId: fallbackChannelID, ctxkey.ChannelModel: &channel,
		ctxkey.TokenId: fallbackTokenID, ctxkey.TokenName: "fallback-token", ctxkey.Id: userID,
		ctxkey.UserUUID: "00000000-0000-4000-8000-000000000001", ctxkey.TokenUUID: "00000000-0000-4000-8000-000000000002", ctxkey.ChannelUUID: channel.UUID,
		ctxkey.Group: "default", ctxkey.ModelMapping: map[string]string{"alias": "veo3-fast"}, ctxkey.ChannelRatio: float64(1),
		ctxkey.RequestModel: "alias", ctxkey.BaseURL: base, ctxkey.ContentType: "application/json",
		ctxkey.RequestId: fmt.Sprintf("muapi-%d", userID), ctxkey.Username: "response-fallback",
		ctxkey.UserObj: &dbmodel.User{Id: userID, Quota: balance}, ctxkey.Config: dbmodel.ChannelConfig{},
		ctxkey.TokenQuotaUnlimited: false, ctxkey.TokenQuota: balance,
	} {
		c.Set(key, value)
	}
	meta.Set2Context(c, &meta.Meta{
		Mode: relaymode.AsyncVideos, APIType: channeltype.ToAPIType(channeltype.MuAPI), ChannelType: channeltype.MuAPI, ChannelId: fallbackChannelID,
		TokenId: fallbackTokenID, UserId: userID, OriginModelName: "alias", ActualModelName: "veo3-fast", TokenName: "fallback-token",
		UserUUID: c.GetString(ctxkey.UserUUID), TokenUUID: c.GetString(ctxkey.TokenUUID), ChannelUUID: channel.UUID,
		ModelMapping: map[string]string{"alias": "veo3-fast"}, BaseURL: base, APIKey: "muapi-fixture-key", RequestURLPath: path,
	})
	return c, recorder
}

// muapiTaskServer serves local provider protocol fixtures with atomic call counts.
// The mutable observation is deliberately atomic for race-enabled worker tests.
type muapiTaskServer struct {
	URL       string
	creates   atomic.Int32
	estimates atomic.Int32
	polls     atomic.Int32
	badAuth   atomic.Bool
	badBody   atomic.Bool
	submit    atomic.Value
	poll      atomic.Value
	quote     atomic.Value
}

// newMuapiTaskServer returns a local provider; no real generation is submitted.
func newMuapiTaskServer(t *testing.T) *muapiTaskServer {
	t.Helper()
	fixture := &muapiTaskServer{}
	fixture.submit.Store(`{"request_id":"muapi-job-1","status":"processing"}`)
	fixture.poll.Store(`{"id":"muapi-job-1","status":"completed","outputs":["https://media.example/video.mp4"],"cost":{"refunded":false},"provider_secret":"never expose"}`)
	fixture.quote.Store(`{"cost":0.40,"currency":"USD"}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("x-api-key") != "muapi-fixture-key" {
			fixture.badAuth.Store(true)
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/estimate-cost"):
			fixture.estimates.Add(1)
			_, _ = io.WriteString(w, fixture.quote.Load().(string))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/veo3-fast":
			fixture.creates.Add(1)
			var body map[string]any
			if json.NewDecoder(r.Body).Decode(&body) != nil || body["model"] != nil || body["duration"] != float64(5) {
				fixture.badBody.Store(true)
			}
			_, _ = io.WriteString(w, fixture.submit.Load().(string))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/predictions/muapi-job-1/result":
			fixture.polls.Add(1)
			_, _ = io.WriteString(w, fixture.poll.Load().(string))
		default:
			http.NotFound(w, r)
		}
	}))
	fixture.URL = server.URL
	previousClient := client.HTTPClient
	client.HTTPClient = server.Client()
	t.Cleanup(func() { server.Close(); client.HTTPClient = previousClient })
	return fixture
}

// muapiRunWorkerStep makes persisted work due and executes the real provider
// resolver/worker once. It does not mock quota or task database functions.
func muapiRunWorkerStep(t *testing.T) {
	t.Helper()
	require.NoError(t, dbmodel.DB.Model(&dbmodel.AsyncTask{}).Where("billing_state = ?", dbmodel.AsyncBillingHeld).Update("next_poll_at", 0).Error)
	worked, err := asyncvideo.ProcessOne(context.Background(), ResolveAsyncVideoProvider, time.Now().UTC())
	require.NoError(t, err)
	require.True(t, worked)
}

// muapiSubmitTask admits through the public async controller and returns its
// owner-scoped persisted record, not a provider ID.
func muapiSubmitTask(t *testing.T, server *muapiTaskServer, key string) (*gin.Context, *dbmodel.AsyncTask) {
	t.Helper()
	c, recorder := muapiVideoContext(t, http.MethodPost, "/v1/async/videos", `{"model":"alias","prompt":"a lighthouse","duration":5}`, server.URL, 10000000, fallbackUserID)
	c.Request.Header.Set("Idempotency-Key", key)
	require.Nil(t, RelayAsyncVideoHelper(c, false))
	require.Equal(t, http.StatusAccepted, recorder.Code, recorder.Body.String())
	var response asyncVideoTaskResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	task, err := dbmodel.GetOwnedAsyncTask(context.Background(), response.ID, fallbackUserID, c.GetString(ctxkey.UserUUID))
	require.NoError(t, err)
	return c, task
}

// TestMuAPIVideoLifecycle preserves the contributor's full wire/accounting test,
// now exercising the durable job controller, worker and provider-neutral result.
func TestMuAPIVideoLifecycle(t *testing.T) {
	const balance int64 = 10000000
	muapiTaskSetup(t, balance)
	server := newMuapiTaskServer(t)
	c, task := muapiSubmitTask(t, server, "lifecycle")
	require.EqualValues(t, 1, server.estimates.Load())
	require.Zero(t, server.creates.Load(), "paid work cannot start before durable admission")
	require.Equal(t, balance-200000, reloadUserQuota(t))
	require.Equal(t, dbmodel.AsyncTaskReserved, task.State)
	muapiRunWorkerStep(t)
	muapiRunWorkerStep(t)
	require.EqualValues(t, 1, server.creates.Load())
	require.EqualValues(t, 1, server.polls.Load())
	require.False(t, server.badAuth.Load())
	require.False(t, server.badBody.Load())
	require.NoError(t, dbmodel.FlushAsyncTaskLogs(context.Background()))
	require.EqualValues(t, 200000, requestCostQuota(t, c.GetString(ctxkey.RequestId)))
	poll, recorder := muapiVideoContext(t, http.MethodGet, "/v1/async/videos/"+task.ID, "", server.URL, balance, fallbackUserID)
	poll.Params = gin.Params{{Key: "video_id", Value: task.ID}}
	GetAsyncVideoTask(poll)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"status":"completed"`)
	require.Contains(t, recorder.Body.String(), `"url":"https://media.example/video.mp4"`)
	require.NotContains(t, recorder.Body.String(), "provider_secret")
	require.NotContains(t, recorder.Body.String(), "muapi-job-1")
	require.NotContains(t, recorder.Body.String(), "lighthouse")
	require.Equal(t, balance-200000, reloadUserQuota(t))
	wrong, denied := muapiVideoContext(t, http.MethodGet, "/v1/async/videos/"+task.ID, "", server.URL, balance, fallbackUserID+1)
	wrong.Params = poll.Params
	GetAsyncVideoTask(wrong)
	require.Equal(t, http.StatusNotFound, denied.Code)
	require.EqualValues(t, 1, server.polls.Load(), "retrieval never invokes the provider")
}

// TestMuAPIVideoLifecycleFailsClosedBeforeSubmission checks quote and quota
// failures before any durable job or paid provider operation can exist.
func TestMuAPIVideoLifecycleFailsClosedBeforeSubmission(t *testing.T) {
	for _, test := range []struct {
		name    string
		balance int64
		quote   string
	}{
		{"malformed quote", 10000000, `{"cost":"invalid","currency":"USD"}`},
		{"insufficient quota", 1, `{"cost":0.40,"currency":"USD"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			muapiTaskSetup(t, test.balance)
			server := newMuapiTaskServer(t)
			server.quote.Store(test.quote)
			body := `{"model":"alias","prompt":"a lighthouse","duration":5}`
			c, recorder := muapiVideoContext(t, http.MethodPost, "/v1/async/videos", body, server.URL, test.balance, fallbackUserID)
			apiErr := RelayAsyncVideoHelper(c, false)
			if test.name == "malformed quote" {
				require.NotNil(t, apiErr)
			} else {
				require.Nil(t, apiErr)
				require.Equal(t, 403, recorder.Code, recorder.Body.String())
			}
			require.Equal(t, []byte(body), c.MustGet(ctxkey.KeyRequestBody), "safe retries must retain the ORIGINAL model and body")
			require.Zero(t, server.creates.Load())
			require.Equal(t, test.balance, reloadUserQuota(t))
			var count int64
			require.NoError(t, dbmodel.DB.Model(&dbmodel.AsyncTask{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

// TestMuAPIVideoAcceptedDisconnectKeepsChargeAndBinding verifies that a failed
// response write cannot erase durable work or refund its reserved funds.
func TestMuAPIVideoAcceptedDisconnectKeepsChargeAndBinding(t *testing.T) {
	muapiTaskSetup(t, 10000000)
	server := newMuapiTaskServer(t)
	c, _ := muapiVideoContext(t, http.MethodPost, "/v1/async/videos", `{"model":"alias","prompt":"a lighthouse","duration":5}`, server.URL, 10000000, fallbackUserID)
	c.Writer = xaiDisconnectedWriter{c.Writer}
	require.Nil(t, RelayAsyncVideoHelper(c, false))
	require.NotEmpty(t, c.GetString(asyncvideo.DurableTaskKey))
	task, err := dbmodel.GetOwnedAsyncTask(context.Background(), c.GetString(asyncvideo.DurableTaskKey), fallbackUserID, c.GetString(ctxkey.UserUUID))
	require.NoError(t, err)
	require.Equal(t, dbmodel.AsyncBillingHeld, task.BillingState)
	muapiRunWorkerStep(t)
	require.EqualValues(t, 1, server.creates.Load())
	require.EqualValues(t, 9800000, reloadUserQuota(t))
}
