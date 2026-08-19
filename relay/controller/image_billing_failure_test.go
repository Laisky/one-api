package controller

import (
	"context"
	"net/http/httptest"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
)

// TestImageUpstreamFailure_ForwardedChargeRemainsVisible verifies the
// conservative no-underbilling policy at the observable accounting boundary.
// A forwarded upstream failure keeps the debit, so the consume log and request
// cost must both retain that amount instead of reporting a false refund.
func TestImageUpstreamFailure_ForwardedChargeRemainsVisible(t *testing.T) {
	cleanup := setupCacheBillingLogTest(t)
	defer cleanup()
	require.NoError(t, model.DB.AutoMigrate(&model.UserRequestCost{}))

	previousLogSetting := config.IsLogConsumeEnabled()
	config.SetLogConsumeEnabled(true)
	t.Cleanup(func() { config.SetLogConsumeEnabled(previousLogSetting) })
	previousRedisSetting := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() { common.SetRedisEnabled(previousRedisSetting) })

	const (
		requestID = "image-forwarded-400"
		charged   = int64(18_000)
	)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/images/generations", nil)
	gmw.SetLogger(c, logger.Logger)
	c.Set(ctxkey.Id, 1)
	c.Set(ctxkey.TokenId, 1)
	c.Set(ctxkey.RequestId, requestID)
	c.Set(ctxkey.UpstreamRequestPossiblyForwarded, true)

	require.NoError(t, model.PostConsumeTokenQuota(context.Background(), 1, charged))
	provisionalLogID := model.RecordProvisionalConsumeLog(gmw.Ctx(c), &model.Log{
		UserId:    1,
		ModelName: "gpt-image-1-mini",
		TokenName: "relay-cache-log-token",
		RequestId: requestID,
	}, charged)
	require.Positive(t, provisionalLogID)
	require.NoError(t, model.UpdateUserRequestCostQuotaByRequestID(1, requestID, charged))

	reconcileImageFailureBilling(c, context.Background(), 1, charged, provisionalLogID, "upstream_http_error")

	var user model.User
	require.NoError(t, model.DB.First(&user, 1).Error)
	require.Equal(t, int64(1_000_000)-charged, user.Quota,
		"a forwarded upstream failure must keep the physical debit")

	var logEntry model.Log
	require.NoError(t, model.LOG_DB.First(&logEntry, provisionalLogID).Error)
	require.Equal(t, model.LogTypeConsume, logEntry.Type)
	require.Equal(t, int(charged), logEntry.Quota)
	require.Contains(t, logEntry.Content, "charge retained")
	_, stillProvisional := logEntry.Metadata[model.LogMetadataKeyProvisional]
	require.False(t, stillProvisional)

	var requestCost model.UserRequestCost
	require.NoError(t, model.DB.Where("request_id = ?", requestID).First(&requestCost).Error)
	require.Equal(t, charged, requestCost.Quota,
		"request cost must match the quota actually debited")
}
