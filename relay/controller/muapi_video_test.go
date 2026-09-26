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

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/middleware"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// muapiVideoContext builds a routed MuAPI video request for controller tests. Parameters are the test handle, HTTP method, request path/body, upstream base URL, account balance, and user ID. It returns the Gin context and response recorder.
func muapiVideoContext(t *testing.T, method, path, body, base string, balance int64, userID int) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	channel := &dbmodel.Channel{Id: fallbackChannelID, Type: channeltype.MuAPI, Name: "muapi-video-test", Key: "muapi-fixture-key"}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	gmw.SetLogger(c, logger.Logger)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Authorization", "Bearer muapi-fixture-key")
	for key, value := range map[string]any{
		ctxkey.Channel: channeltype.MuAPI, ctxkey.ChannelId: fallbackChannelID, ctxkey.ChannelModel: channel,
		ctxkey.TokenId: fallbackTokenID, ctxkey.TokenName: "fallback-token", ctxkey.Id: userID,
		ctxkey.Group: "default", ctxkey.ModelMapping: map[string]string{"alias": "veo3-fast"}, ctxkey.ChannelRatio: float64(1),
		ctxkey.RequestModel: "alias", ctxkey.BaseURL: base, ctxkey.ContentType: "application/json",
		ctxkey.RequestId: fmt.Sprintf("muapi-%d", userID), ctxkey.Username: "response-fallback",
		ctxkey.UserObj: &dbmodel.User{Id: userID, Quota: balance}, ctxkey.Config: dbmodel.ChannelConfig{},
		ctxkey.TokenQuotaUnlimited: false, ctxkey.TokenQuota: balance,
	} {
		c.Set(key, value)
	}
	meta.Set2Context(c, &meta.Meta{
		Mode: relaymode.Videos, APIType: channeltype.ToAPIType(channeltype.MuAPI), ChannelType: channeltype.MuAPI, ChannelId: fallbackChannelID,
		TokenId: fallbackTokenID, UserId: userID, OriginModelName: "alias", ActualModelName: "veo3-fast",
		ModelMapping: map[string]string{"alias": "veo3-fast"}, BaseURL: base, APIKey: "muapi-fixture-key", RequestURLPath: path,
	})
	return c, recorder
}

// TestMuAPIVideoLifecycle exercises request normalization, exact quoted
// pricing, quota admission, a single paid submit, durable task binding, and
// polling through the bound MuAPI channel.
func TestMuAPIVideoLifecycle(t *testing.T) {
	const balance int64 = 10000000
	xaiVideoSetup(t, balance, false)
	var estimates, creates, polls atomic.Int32
	var badAuth, badCreateBody atomic.Bool
	var createBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("x-api-key") != "muapi-fixture-key" {
			badAuth.Store(true)
		}
		switch {
		case r.URL.Path == "/api/v1/models/veo3-fast/estimate-cost":
			estimates.Add(1)
			_, _ = io.WriteString(w, `{"cost":0.40,"currency":"USD"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/veo3-fast":
			creates.Add(1)
			if err := json.NewDecoder(r.Body).Decode(&createBody); err != nil {
				badCreateBody.Store(true)
			}
			_, _ = io.WriteString(w, `{"request_id":"muapi-job-1","status":"processing"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/predictions/muapi-job-1/result":
			polls.Add(1)
			_, _ = io.WriteString(w, `{"status":"completed","outputs":[]}`)
		default:
			t.Errorf("unexpected MuAPI request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	previousClient := client.HTTPClient
	client.HTTPClient = server.Client()
	t.Cleanup(func() { client.HTTPClient = previousClient })

	c, recorder := muapiVideoContext(t, http.MethodPost, "/v1/videos", `{"model":"alias","prompt":"a lighthouse","duration":5}`, server.URL, balance, fallbackUserID)
	require.Nil(t, RelayVideoHelper(c))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.EqualValues(t, 1, estimates.Load())
	require.EqualValues(t, 1, creates.Load())
	require.False(t, badAuth.Load())
	require.False(t, badCreateBody.Load())
	require.NotContains(t, createBody, "model")
	require.Equal(t, float64(5), createBody["duration"])
	drainCriticalTasks(t)
	require.Equal(t, balance-200000, reloadUserQuota(t))
	require.EqualValues(t, 200000, requestCostQuota(t, c.GetString(ctxkey.RequestId)))
	binding, err := dbmodel.GetAsyncTaskBindingByTaskID(context.Background(), "muapi-job-1")
	require.NoError(t, err)
	require.Equal(t, fallbackUserID, binding.UserID)
	require.Equal(t, fallbackChannelID, binding.ChannelID)
	require.Equal(t, channeltype.MuAPI, binding.ChannelType)
	require.Equal(t, "alias", binding.OriginModel)
	require.Equal(t, "veo3-fast", binding.ActualModel)

	poll, pollRecorder := muapiVideoContext(t, http.MethodGet, "/v1/videos/muapi-job-1", "", server.URL, balance, fallbackUserID)
	poll.Params = gin.Params{{Key: "video_id", Value: "muapi-job-1"}}
	middleware.BindAsyncTaskChannel()(poll)
	require.Equal(t, fallbackChannelID, poll.GetInt(ctxkey.SpecificChannelId))
	require.Equal(t, "alias", poll.GetString(ctxkey.RequestModel))
	require.Nil(t, RelayVideoHelper(poll))
	require.Equal(t, http.StatusOK, pollRecorder.Code)
	require.EqualValues(t, 1, polls.Load())
	drainCriticalTasks(t)
	require.Equal(t, balance-200000, reloadUserQuota(t), "polling must not charge generation again")

	wrongUser, deniedRecorder := muapiVideoContext(t, http.MethodGet, "/v1/videos/muapi-job-1", "", server.URL, balance, fallbackUserID+1)
	wrongUser.Params = gin.Params{{Key: "video_id", Value: "muapi-job-1"}}
	middleware.BindAsyncTaskChannel()(wrongUser)
	require.Equal(t, http.StatusNotFound, deniedRecorder.Code)
	require.EqualValues(t, 1, polls.Load(), "a different user must not poll the stored task")
}

// TestMuAPIVideoLifecycleFailsClosedBeforeSubmission checks that unavailable
// or malformed dynamic quotes and insufficient user quota never submit a paid
// MuAPI creation request.
func TestMuAPIVideoLifecycleFailsClosedBeforeSubmission(t *testing.T) {
	for _, test := range []struct {
		name    string
		balance int64
		quote   string
	}{
		{name: "malformed quote", balance: 10000000, quote: `{"cost":"invalid","currency":"USD"}`},
		{name: "insufficient quota", balance: 1, quote: `{"cost":0.40,"currency":"USD"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			xaiVideoSetup(t, test.balance, false)
			var creates atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if strings.HasSuffix(r.URL.Path, "/estimate-cost") {
					_, _ = io.WriteString(w, test.quote)
					return
				}
				creates.Add(1)
				_, _ = io.WriteString(w, `{"request_id":"unexpected"}`)
			}))
			defer server.Close()
			previousClient := client.HTTPClient
			client.HTTPClient = server.Client()
			t.Cleanup(func() { client.HTTPClient = previousClient })

			c, _ := muapiVideoContext(t, http.MethodPost, "/v1/videos", `{"model":"alias","prompt":"a lighthouse","duration":5}`, server.URL, test.balance, fallbackUserID)
			apiErr := RelayVideoHelper(c)
			require.NotNil(t, apiErr)
			require.Zero(t, creates.Load())
			drainCriticalTasks(t)
			require.Equal(t, test.balance, reloadUserQuota(t))
		})
	}
}

// TestMuAPIVideoAcceptedDisconnectKeepsChargeAndBinding verifies a downstream
// disconnect cannot refund a MuAPI generation that was already accepted.
func TestMuAPIVideoAcceptedDisconnectKeepsChargeAndBinding(t *testing.T) {
	const balance int64 = 10000000
	xaiVideoSetup(t, balance, false)
	var creates atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/estimate-cost") {
			_, _ = io.WriteString(w, `{"cost":0.40,"currency":"USD"}`)
			return
		}
		creates.Add(1)
		_, _ = io.WriteString(w, `{"request_id":"muapi-disconnected-job","status":"processing"}`)
	}))
	defer server.Close()
	previousClient := client.HTTPClient
	client.HTTPClient = server.Client()
	t.Cleanup(func() { client.HTTPClient = previousClient })

	c, _ := muapiVideoContext(t, http.MethodPost, "/v1/videos", `{"model":"alias","prompt":"a lighthouse","duration":5}`, server.URL, balance, fallbackUserID)
	requestID := c.GetString(ctxkey.RequestId)
	c.Writer = xaiDisconnectedWriter{c.Writer}
	apiErr := RelayVideoHelper(c)
	require.NotNil(t, apiErr)
	require.EqualValues(t, 1, creates.Load())
	drainCriticalTasks(t)
	require.Equal(t, balance-200000, reloadUserQuota(t))
	require.EqualValues(t, 200000, requestCostQuota(t, requestID))
	binding, err := dbmodel.GetAsyncTaskBindingByTaskID(context.Background(), "muapi-disconnected-job")
	require.NoError(t, err)
	require.Equal(t, fallbackUserID, binding.UserID)
	require.Equal(t, fallbackChannelID, binding.ChannelID)
}
