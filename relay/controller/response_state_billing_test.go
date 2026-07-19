package controller

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/graceful"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/state"
)

// newCountingUpstream installs a fake upstream that records how many times it was
// contacted and returns its URL plus the hit counter, so a test can prove a local
// state rejection never reached the provider (rows E02, BIL05). Point the request's
// BaseURL at the returned URL.
func newCountingUpstream(t *testing.T) (string, *int64) {
	t.Helper()
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_should_not_be_used","object":"response","status":"completed","output":[]}`))
	}))
	t.Cleanup(srv.Close)
	prev := client.HTTPClient
	client.HTTPClient = srv.Client()
	t.Cleanup(func() { client.HTTPClient = prev })
	return srv.URL, &hits
}

// userQuota reads the persisted quota for the fallback fixture user.
func fallbackUserQuota(t *testing.T) int64 {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.First(&user, fallbackUserID).Error)
	return user.Quota
}

// setupResponseStateBillingContext builds a request context for a state-enabled
// Responses fallback request, mirroring TestRelayResponseAPIHelper_FallbackAzure
// but pointed at the OpenAI-compatible fallback channel. Whether the upstream is
// actually contacted is asserted separately via newCountingUpstream.
func setupResponseStateBillingContext(t *testing.T, recorder *httptest.ResponseRecorder, payload string) *gin.Context {
	t.Helper()
	c, _ := gin.CreateTestContext(recorder)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer compat-key")
	c.Request = req
	gmw.SetLogger(c, logger.Logger)

	c.Set(ctxkey.Channel, channeltype.OpenAICompatible)
	c.Set(ctxkey.ChannelId, fallbackCompatibleChannelID)
	c.Set(ctxkey.TokenId, fallbackTokenID)
	c.Set(ctxkey.TokenName, "fallback-token")
	c.Set(ctxkey.Id, fallbackUserID)
	c.Set(ctxkey.Group, "default")
	c.Set(ctxkey.ModelMapping, map[string]string{})
	c.Set(ctxkey.ChannelRatio, 1.0)
	c.Set(ctxkey.RequestModel, "gpt-4o-mini")
	c.Set(ctxkey.ContentType, "application/json")
	c.Set(ctxkey.RequestId, "req_state_billing")
	c.Set(ctxkey.TokenQuotaUnlimited, true)
	c.Set(ctxkey.TokenQuota, int64(0))
	c.Set(ctxkey.Username, "response-fallback")
	c.Set(ctxkey.UserObj, &model.User{Id: fallbackUserID, Quota: 1_000_000})
	c.Set(ctxkey.ChannelModel, &model.Channel{Id: fallbackCompatibleChannelID, Type: channeltype.OpenAICompatible})
	c.Set(ctxkey.Config, model.ChannelConfig{})
	return c
}

// TestResponseStateBilling_NotPortableChargesNoQuota proves the ST-008 fail-closed
// path is a pure local rejection: a state_not_portable error is returned before
// any upstream call and before any quota is pre-consumed, so the user is not
// charged (BIL05, E02, E06). This is the core "billing is unchanged" guarantee for
// the new state error paths.
func TestResponseStateBilling_NotPortableChargesNoQuota(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ensureResponseFallbackFixtures(t)
	enableStateForTest(t)

	prevRedis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() { common.SetRedisEnabled(prevRedis) })
	prevLogConsume := config.IsLogConsumeEnabled()
	config.SetLogConsumeEnabled(false)
	t.Cleanup(func() { config.SetLogConsumeEnabled(prevLogConsume) })

	before := fallbackUserQuota(t)
	upstreamURL, upstreamHits := newCountingUpstream(t)

	recorder := httptest.NewRecorder()
	// A provider-hosted tool-call item that has no faithful Chat/Claude
	// representation must fail closed BEFORE billing.
	payload := `{"model":"gpt-4o-mini","stream":false,"input":[{"type":"web_search_call","id":"ws_1","status":"completed"}]}`
	c := setupResponseStateBillingContext(t, recorder, payload)
	c.Set(ctxkey.BaseURL, upstreamURL)

	apiErr := RelayResponseAPIHelper(c)
	require.NotNil(t, apiErr, "state_not_portable must be returned")
	require.Equal(t, http.StatusConflict, apiErr.StatusCode)
	require.Equal(t, "state_not_portable", apiErr.Code)

	require.Equal(t, before, fallbackUserQuota(t), "a local state rejection must not consume quota")
	require.Equal(t, int64(0), atomic.LoadInt64(upstreamHits), "a local state rejection must not contact the upstream (E02)")
}

// TestResponseStateBilling_PreviousResponseNotFoundChargesNoQuota proves an
// unresolved previous_response_id is likewise a pure local rejection with no quota
// impact (E03/E06).
func TestResponseStateBilling_PreviousResponseNotFoundChargesNoQuota(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ensureResponseFallbackFixtures(t)
	enableStateForTest(t)

	prevRedis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() { common.SetRedisEnabled(prevRedis) })
	prevLogConsume := config.IsLogConsumeEnabled()
	config.SetLogConsumeEnabled(false)
	t.Cleanup(func() { config.SetLogConsumeEnabled(prevLogConsume) })

	before := fallbackUserQuota(t)
	upstreamURL, upstreamHits := newCountingUpstream(t)

	recorder := httptest.NewRecorder()
	payload := `{"model":"gpt-4o-mini","stream":false,"previous_response_id":"resp_ffffffffffffffffffffffffffffffff","input":"continue"}`
	c := setupResponseStateBillingContext(t, recorder, payload)
	c.Set(ctxkey.BaseURL, upstreamURL)

	apiErr := RelayResponseAPIHelper(c)
	require.NotNil(t, apiErr)
	require.Equal(t, "previous_response_not_found", apiErr.Code)
	require.Equal(t, before, fallbackUserQuota(t), "an unresolved parent must not consume quota")
	require.Equal(t, int64(0), atomic.LoadInt64(upstreamHits), "an unresolved parent must not contact the upstream (E02)")
}

// resetFallbackUserQuota restores the fixture user's persisted quota so a second
// billed request starts from the same baseline.
func resetFallbackUserQuota(t *testing.T, quota int64) {
	t.Helper()
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", fallbackUserID).Update("quota", quota).Error)
}

// runFallbackBillingRequest runs one successful stateless Responses fallback
// request against a fake upstream and returns the quota actually consumed, after
// draining the async post-billing task.
func runFallbackBillingRequest(t *testing.T, upstreamURL string) int64 {
	t.Helper()
	resetFallbackUserQuota(t, 1_000_000)
	before := fallbackUserQuota(t)

	recorder := httptest.NewRecorder()
	payload := `{"model":"gpt-4o-mini","stream":false,"input":[{"role":"user","content":[{"type":"input_text","text":"bill me once"}]}]}`
	c := setupResponseStateBillingContext(t, recorder, payload)
	c.Set(ctxkey.BaseURL, upstreamURL)

	apiErr := RelayResponseAPIHelper(c)
	require.Nil(t, apiErr, "successful fallback must not error")
	require.Equal(t, http.StatusOK, recorder.Code)

	// Drain the async post-billing goroutine so the persisted quota is final.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, graceful.Drain(ctx))

	return before - fallbackUserQuota(t)
}

// TestResponseStateBilling_EnabledMatchesDisabled proves the strongest billing
// neutrality guarantee: an identical successful request bills the exact same quota
// whether the gateway state feature is off or on. Enabling state adds a state
// commit but must not add, drop, or double any charge (BIL02, BIL06).
func TestResponseStateBilling_EnabledMatchesDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ensureResponseFallbackFixtures(t)

	prevRedis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() { common.SetRedisEnabled(prevRedis) })
	prevLogConsume := config.IsLogConsumeEnabled()
	config.SetLogConsumeEnabled(false)
	t.Cleanup(func() { config.SetLogConsumeEnabled(prevLogConsume) })

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
		  "id": "chatcmpl-neutral",
		  "object": "chat.completion",
		  "model": "gpt-4o-mini",
		  "choices": [{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
		  "usage": {"prompt_tokens": 11, "completion_tokens": 7, "total_tokens": 18}
		}`))
	}))
	defer upstream.Close()
	prevClient := client.HTTPClient
	client.HTTPClient = upstream.Client()
	t.Cleanup(func() { client.HTTPClient = prevClient })

	// Baseline: feature disabled.
	require.False(t, state.Enabled())
	disabledCharge := runFallbackBillingRequest(t, upstream.URL)
	require.Positive(t, disabledCharge, "a successful request must charge some quota")

	// Same request with the state feature enabled (commit runs, billing must match).
	enableStateForTest(t)
	enabledCharge := runFallbackBillingRequest(t, upstream.URL)

	require.Equal(t, disabledCharge, enabledCharge,
		"enabling gateway state must not change the billed quota for an identical request")
}
