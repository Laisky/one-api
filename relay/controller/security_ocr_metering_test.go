package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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

// TestSecurityOCRMeasuredSettlement proves native token-priced OCR charges the
// measured 1200 units, rather than one request, against real finite balances.
func TestSecurityOCRMeasuredSettlement(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ensureResponseFallbackFixtures(t)
	oldRedis, oldLogs, oldClient := common.IsRedisEnabled(), config.IsLogConsumeEnabled(), client.HTTPClient
	common.SetRedisEnabled(false)
	config.SetLogConsumeEnabled(true)
	t.Cleanup(func() {
		drainCriticalTasks(t)
		common.SetRedisEnabled(oldRedis)
		config.SetLogConsumeEnabled(oldLogs)
		client.HTTPClient = oldClient
	})
	var dispatches atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dispatches.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"id":"ocr-fixture","model":"glm-ocr","md_results":"fixture","usage":{"prompt_tokens":800,"completion_tokens":400,"total_tokens":1200},"data_info":{"num_pages":1}}`))
		require.NoError(t, err)
	}))
	t.Cleanup(upstream.Close)
	client.HTTPClient = upstream.Client()
	initial := seedDoubleChargeUser(t)
	channel := &model.Channel{Id: fallbackChannelID, Type: channeltype.Zhipu}
	require.NoError(t, channel.SetModelPriceConfigs(map[string]model.ModelConfigLocal{"glm-ocr": {Ratio: 2, CompletionRatio: 1}}))
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/layout_parsing", strings.NewReader(`{"model":"glm-ocr","file":"https://documents.example.test/document.pdf","start_page_id":1,"end_page_id":1}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Authorization", "Bearer fixture-upstream-key")
	gmw.SetLogger(c, logger.Logger)
	for key, value := range map[string]any{
		ctxkey.Channel: channeltype.Zhipu, ctxkey.ChannelId: fallbackChannelID, ctxkey.ChannelModel: channel,
		ctxkey.TokenId: fallbackTokenID, ctxkey.TokenName: "fallback-token", ctxkey.Id: fallbackUserID,
		ctxkey.Group: "default", ctxkey.ModelMapping: map[string]string{}, ctxkey.ChannelRatio: 1.0,
		ctxkey.RequestModel: "glm-ocr", ctxkey.BaseURL: upstream.URL, ctxkey.ContentType: "application/json",
		ctxkey.RequestId: "security-ocr-measured", ctxkey.Username: "response-fallback",
		ctxkey.UserObj: &model.User{Id: fallbackUserID, Quota: initial}, ctxkey.Config: model.ChannelConfig{},
		ctxkey.TokenQuotaUnlimited: false, ctxkey.TokenQuota: initial,
	} {
		c.Set(key, value)
	}
	require.Nil(t, RelayOCRHelper(c))
	drainCriticalTasks(t)
	require.Equal(t, http.StatusOK, writer.Code)
	require.Equal(t, int32(1), dispatches.Load())
	require.EqualValues(t, 2400, initial-reloadUserQuota(t), "1200 measured units at ratio 2 must cost 2400")
	var token model.Token
	require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
	require.EqualValues(t, 2400, initial-token.RemainQuota)
	require.EqualValues(t, 2400, requestCostQuota(t, "security-ocr-measured"))
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", "security-ocr-measured", model.LogTypeConsume).Find(&logs).Error)
	require.Len(t, logs, 1)
	require.EqualValues(t, 2400, logs[0].Quota)
}
