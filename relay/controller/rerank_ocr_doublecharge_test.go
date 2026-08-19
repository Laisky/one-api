package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// doubleChargeOverrideRatio is the channel-level model-ratio override used by
// both regression tests. It pins totalQuota to a clean, sizable value
// (ceil(ratio*groupRatio) = 1000 with groupRatio=1) so a 1x vs 2x charge is
// large and unambiguous, independent of each model's native (tiny) ratio.
const doubleChargeOverrideRatio = 1000.0

// doubleChargeTotalQuota is the single settled charge expected for one
// successful per-call (rerank/OCR) request given doubleChargeOverrideRatio and
// a groupRatio (ChannelRatio) of 1.0.
const doubleChargeTotalQuota = int64(1000)

// doubleChargeUserQuota is a small finite quota: large enough that pre-consume
// succeeds (userQuota-totalQuota >= 0) yet small enough that the trust-skip
// branch (userQuota > 100*totalQuota) never fires, so PreConsumeTokenQuota
// really deducts and the leak is observable. 50_000 < 100*1000 == 100_000.
const doubleChargeUserQuota = int64(50_000)

// seedDoubleChargeUser overrides the shared fixtures so the user/token have a
// SMALL finite quota and the token is NOT unlimited, then returns the starting
// user quota. This keeps the per-call pre-consume out of the trust path.
func seedDoubleChargeUser(t *testing.T) int64 {
	t.Helper()
	require.NoError(t, model.DB.Model(&model.User{}).
		Where("id = ?", fallbackUserID).
		Update("quota", doubleChargeUserQuota).Error,
		"failed to seed small user quota")
	require.NoError(t, model.DB.Model(&model.Token{}).
		Where("id = ?", fallbackTokenID).
		Updates(map[string]any{
			"unlimited_quota": false,
			"remain_quota":    doubleChargeUserQuota,
		}).Error,
		"failed to seed finite token quota")
	return reloadUserQuota(t)
}

// newDoubleChargeChannel builds a channel fixture carrying a model-ratio
// override for the given model so totalQuota is deterministic and large.
func newDoubleChargeChannel(t *testing.T, channelID, channelType int, modelName string) *model.Channel {
	t.Helper()
	ch := &model.Channel{Id: channelID, Type: channelType}
	require.NoError(t, ch.SetModelPriceConfigs(map[string]model.ModelConfigLocal{
		modelName: {Ratio: doubleChargeOverrideRatio, CompletionRatio: 1},
	}), "failed to set channel model price configs")
	return ch
}

// TestRelayRerankHelper_SuccessDoesNotDoubleCharge drives RelayRerankHelper for
// a NON-TRUSTED token (finite quota so pre-consume actually deducts and the
// trust-skip does NOT fire) against an httptest upstream returning a valid
// Cohere rerank success body. On success the user must be charged for exactly
// ONE settled request (totalQuota), not 2x.
func TestRelayRerankHelper_SuccessDoesNotDoubleCharge(t *testing.T) {
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
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
		  "id": "rerank-doublecharge",
		  "results": [
		    {"index": 0, "relevance_score": 0.99},
		    {"index": 1, "relevance_score": 0.10}
		  ],
		  "meta": {"billed_units": {"search_units": 1}, "tokens": {"input_tokens": 12, "output_tokens": 0}}
		}`))
	}))
	t.Cleanup(upstream.Close)

	prevClient := client.HTTPClient
	client.HTTPClient = upstream.Client()
	t.Cleanup(func() { client.HTTPClient = prevClient })

	startQuota := seedDoubleChargeUser(t)
	require.EqualValues(t, doubleChargeUserQuota, startQuota, "unexpected seeded start quota")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	const model3 = "rerank-v3.5"
	requestPayload := `{"model":"` + model3 + `","query":"hello","documents":["doc a","doc b"]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/rerank", strings.NewReader(requestPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer cohere-key")
	c.Request = req

	gmw.SetLogger(c, logger.Logger)

	c.Set(ctxkey.Channel, channeltype.Cohere)
	c.Set(ctxkey.ChannelId, fallbackChannelID)
	c.Set(ctxkey.ChannelModel, newDoubleChargeChannel(t, fallbackChannelID, channeltype.Cohere, model3))
	c.Set(ctxkey.TokenId, fallbackTokenID)
	c.Set(ctxkey.TokenName, "fallback-token")
	c.Set(ctxkey.Id, fallbackUserID)
	c.Set(ctxkey.Group, "default")
	c.Set(ctxkey.ModelMapping, map[string]string{})
	c.Set(ctxkey.ChannelRatio, 1.0)
	c.Set(ctxkey.RequestModel, model3)
	c.Set(ctxkey.BaseURL, upstream.URL)
	c.Set(ctxkey.ContentType, "application/json")
	c.Set(ctxkey.RequestId, "req_rerank_doublecharge")
	c.Set(ctxkey.Username, "response-fallback")
	c.Set(ctxkey.UserObj, &model.User{Id: fallbackUserID, Quota: doubleChargeUserQuota})
	c.Set(ctxkey.Config, model.ChannelConfig{})

	// CRITICAL: finite (not unlimited) token quota + small user quota keeps
	// preConsumeRerankQuota out of the trust path so PreConsumeTokenQuota
	// really deducts and the double charge is observable.
	c.Set(ctxkey.TokenQuotaUnlimited, false)
	c.Set(ctxkey.TokenQuota, doubleChargeUserQuota)

	apiErr := RelayRerankHelper(c)
	require.Nil(t, apiErr, "RelayRerankHelper returned error: %v", apiErr)
	require.Equal(t, http.StatusOK, recorder.Code, "expected 200 from successful rerank")

	// Drain the async post-billing GoCritical task before reading final quota.
	drainCriticalTasks(t)

	endQuota := reloadUserQuota(t)
	decremented := startQuota - endQuota

	require.EqualValues(t, doubleChargeTotalQuota, decremented,
		"successful rerank must charge for exactly ONE settled request; "+
			"totalQuota=%d actual_decrement=%d (a ~2x decrement indicates the "+
			"pre-consumed quota stayed deducted AND postConsume recharged full totalQuota)",
		doubleChargeTotalQuota, decremented)

	// The settled request cost must also equal a single charge.
	require.EqualValues(t, doubleChargeTotalQuota, requestCostQuota(t, "req_rerank_doublecharge"),
		"settled request cost must equal one totalQuota")
}

// TestRelayOCRHelper_SuccessDoesNotDoubleCharge drives RelayOCRHelper for a
// NON-TRUSTED token against an httptest upstream returning a valid Zhipu OCR
// success body. On success the user must be charged for exactly ONE settled
// request (totalQuota), not 2x.
func TestRelayOCRHelper_SuccessDoesNotDoubleCharge(t *testing.T) {
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
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
		  "id": "ocr-doublecharge",
		  "created": 1700000000,
		  "model": "glm-ocr",
		  "md_results": "# Heading\n\nParagraph text",
		  "layout_details": [[{"index": 1, "label": "text"}]],
		  "usage": {"prompt_tokens": 80, "completion_tokens": 150, "total_tokens": 230}
		}`))
	}))
	t.Cleanup(upstream.Close)

	prevClient := client.HTTPClient
	client.HTTPClient = upstream.Client()
	t.Cleanup(func() { client.HTTPClient = prevClient })

	startQuota := seedDoubleChargeUser(t)
	require.EqualValues(t, doubleChargeUserQuota, startQuota, "unexpected seeded start quota")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	const modelOCR = "glm-ocr"
	requestPayload := `{"model":"` + modelOCR + `","file":"https://example.com/test.pdf"}`
	req := httptest.NewRequest(http.MethodPost, "/api/paas/v4/layout_parsing", strings.NewReader(requestPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer zhipu-key")
	c.Request = req

	gmw.SetLogger(c, logger.Logger)

	c.Set(ctxkey.Channel, channeltype.Zhipu)
	c.Set(ctxkey.ChannelId, fallbackChannelID)
	c.Set(ctxkey.ChannelModel, newDoubleChargeChannel(t, fallbackChannelID, channeltype.Zhipu, modelOCR))
	c.Set(ctxkey.TokenId, fallbackTokenID)
	c.Set(ctxkey.TokenName, "fallback-token")
	c.Set(ctxkey.Id, fallbackUserID)
	c.Set(ctxkey.Group, "default")
	c.Set(ctxkey.ModelMapping, map[string]string{})
	c.Set(ctxkey.ChannelRatio, 1.0)
	c.Set(ctxkey.RequestModel, modelOCR)
	c.Set(ctxkey.BaseURL, upstream.URL)
	c.Set(ctxkey.ContentType, "application/json")
	c.Set(ctxkey.RequestId, "req_ocr_doublecharge")
	c.Set(ctxkey.Username, "response-fallback")
	c.Set(ctxkey.UserObj, &model.User{Id: fallbackUserID, Quota: doubleChargeUserQuota})
	c.Set(ctxkey.Config, model.ChannelConfig{})

	// CRITICAL: finite (not unlimited) token quota + small user quota keeps
	// preConsumeOCRQuota out of the trust path so PreConsumeTokenQuota really
	// deducts and the double charge is observable.
	c.Set(ctxkey.TokenQuotaUnlimited, false)
	c.Set(ctxkey.TokenQuota, doubleChargeUserQuota)

	apiErr := RelayOCRHelper(c)
	require.Nil(t, apiErr, "RelayOCRHelper returned error: %v", apiErr)
	require.Equal(t, http.StatusOK, recorder.Code, "expected 200 from successful OCR")

	// Drain the async post-billing GoCritical task before reading final quota.
	drainCriticalTasks(t)

	endQuota := reloadUserQuota(t)
	decremented := startQuota - endQuota

	require.EqualValues(t, doubleChargeTotalQuota, decremented,
		"successful OCR must charge for exactly ONE settled request; "+
			"totalQuota=%d actual_decrement=%d (a ~2x decrement indicates the "+
			"pre-consumed quota stayed deducted AND postConsume recharged full totalQuota)",
		doubleChargeTotalQuota, decremented)

	require.EqualValues(t, doubleChargeTotalQuota, requestCostQuota(t, "req_ocr_doublecharge"),
		"settled request cost must equal one totalQuota")
}
