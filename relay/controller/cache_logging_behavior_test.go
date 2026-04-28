package controller

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// setupCacheBillingLogTest prepares an isolated database and quota state for
// billing-path tests that need to verify persisted consume logs.
func setupCacheBillingLogTest(t *testing.T) func() {
	t.Helper()

	gin.SetMode(gin.TestMode)

	dsn := fmt.Sprintf("file:relay_cache_log_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Log{}))

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	originalDB := model.DB
	originalLogDB := model.LOG_DB
	model.DB = db
	model.LOG_DB = db

	originalUsingSQLite := common.UsingSQLite.Load()
	common.UsingSQLite.Store(true)

	originalRedisEnabled := common.IsRedisEnabled()
	common.SetRedisEnabled(false)

	user := &model.User{
		Id:       1,
		Username: "relay-cache-log-user",
		Password: "test-password",
		Role:     model.RoleCommonUser,
		Status:   model.UserStatusEnabled,
		Quota:    1_000_000,
	}
	require.NoError(t, model.DB.Create(user).Error)

	token := &model.Token{
		Id:           1,
		UserId:       user.Id,
		Key:          strings.Repeat("a", 48),
		Status:       model.TokenStatusEnabled,
		Name:         "relay-cache-log-token",
		RemainQuota:  1_000_000,
		CreatedTime:  helper.GetTimestamp(),
		AccessedTime: helper.GetTimestamp(),
	}
	require.NoError(t, model.DB.Create(token).Error)

	return func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.UsingSQLite.Store(originalUsingSQLite)
		common.SetRedisEnabled(originalRedisEnabled)
	}
}

// newCacheBillingContext builds a request-scoped context that carries the
// provisional log identifier consumed by post-billing reconciliation.
func newCacheBillingContext(t *testing.T, requestID string, provisionalLogID int) context.Context {
	t.Helper()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/test", nil)
	c.Set(ctxkey.Id, 1)
	c.Set(ctxkey.TokenId, 1)
	c.Set(ctxkey.RequestId, requestID)
	c.Set(ctxkey.ProvisionalLogId, provisionalLogID)
	gmw.SetLogger(c, logger.Logger)

	return gmw.Ctx(c)
}

// newCacheBillingMeta returns minimal metadata required for billing helpers to
// reconcile provisional logs into persisted consume logs.
func newCacheBillingMeta(channelID int) *metalib.Meta {
	return &metalib.Meta{
		TokenId:     1,
		UserId:      1,
		ChannelId:   channelID,
		TokenName:   "relay-cache-log-token",
		StartTime:   time.Now().Add(-2 * time.Second),
		IsStream:    false,
		ChannelType: -1,
		APIType:     -1,
	}
}

// assertCacheAwareConsumeLog verifies that persisted logs expose cached token
// fields, cache-write metadata, and content strings needed by the UI.
func assertCacheAwareConsumeLog(t *testing.T, logID int, requestID string, wantPromptTokens int, wantCachedPromptTokens int, wantCompletionTokens int) {
	t.Helper()

	var saved model.Log
	require.NoError(t, model.LOG_DB.First(&saved, logID).Error)

	require.Equal(t, model.LogTypeConsume, saved.Type)
	require.Equal(t, requestID, saved.RequestId)
	require.Equal(t, wantPromptTokens, saved.PromptTokens)
	require.Equal(t, wantCompletionTokens, saved.CompletionTokens)
	require.Equal(t, wantCachedPromptTokens, saved.CachedPromptTokens)
	require.Contains(t, saved.Content, fmt.Sprintf("cached_prompt %d", wantCachedPromptTokens))
	require.Contains(t, saved.Content, "cache_write_5m 10")
	require.Contains(t, saved.Content, "cache_write_1h 5")

	cacheWriteAny, ok := saved.Metadata[model.LogMetadataKeyCacheWriteTokens]
	require.True(t, ok)
	cacheWrite, ok := cacheWriteAny.(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(10), cacheWrite[model.LogMetadataKeyCacheWrite5m])
	require.Equal(t, float64(5), cacheWrite[model.LogMetadataKeyCacheWrite1h])
	_, hasProvisional := saved.Metadata[model.LogMetadataKeyProvisional]
	require.False(t, hasProvisional)
}

// TestCacheAwareBillingPathsPersistLogFields verifies that ChatCompletion,
// Response API, and Claude billing paths all persist cached token log fields.
func TestCacheAwareBillingPathsPersistLogFields(t *testing.T) {
	testCases := []struct {
		name                   string
		modelName              string
		wantPromptTokens       int
		wantCachedPromptTokens int
		invoke                 func(context.Context, *metalib.Meta, string, string, *relaymodel.Usage, string)
	}{
		{
			name:                   "chat_completion",
			modelName:              "gpt-4o",
			wantPromptTokens:       120,
			wantCachedPromptTokens: 70,
			invoke: func(ctx context.Context, meta *metalib.Meta, requestID string, _ string, usage *relaymodel.Usage, modelName string) {
				postConsumeQuota(
					ctx,
					cloneUsage(usage),
					meta,
					&relaymodel.GeneralOpenAIRequest{Model: modelName},
					0,
					0,
					0,
					1,
					nil,
					1,
					false,
					nil,
					map[string]float64{modelName: 1},
				)
			},
		},
		{
			name:                   "response_api",
			modelName:              "gpt-4o",
			wantPromptTokens:       120,
			wantCachedPromptTokens: 70,
			invoke: func(ctx context.Context, meta *metalib.Meta, requestID string, _ string, usage *relaymodel.Usage, modelName string) {
				postConsumeResponseAPIQuota(
					ctx,
					cloneUsage(usage),
					meta,
					&openai.ResponseAPIRequest{Model: modelName},
					0,
					1,
					nil,
					1,
					nil,
					map[string]float64{modelName: 1},
				)
			},
		},
		{
			name:                   "claude_messages",
			modelName:              "claude-test-model",
			wantPromptTokens:       190,
			wantCachedPromptTokens: 70,
			invoke: func(ctx context.Context, meta *metalib.Meta, requestID string, traceID string, usage *relaymodel.Usage, modelName string) {
				postConsumeClaudeMessagesQuotaWithTraceID(
					ctx,
					requestID,
					traceID,
					cloneUsage(usage),
					meta,
					&ClaudeMessagesRequest{Model: modelName},
					0,
					0,
					0,
					1,
					nil,
					1,
					nil,
					map[string]float64{modelName: 1},
				)
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cleanup := setupCacheBillingLogTest(t)
			defer cleanup()

			requestID := fmt.Sprintf("req-%s-%d", tc.name, time.Now().UnixNano())
			traceID := fmt.Sprintf("trace-%s", tc.name)
			channelID := 11

			ctx := newCacheBillingContext(t, requestID, 0)
			provisionalLogID := model.RecordProvisionalConsumeLog(ctx, &model.Log{
				UserId:    1,
				ChannelId: channelID,
				ModelName: tc.modelName,
				TokenName: "relay-cache-log-token",
				RequestId: requestID,
				TraceId:   traceID,
			}, 1)
			require.Greater(t, provisionalLogID, 0)

			ctx = newCacheBillingContext(t, requestID, provisionalLogID)
			meta := newCacheBillingMeta(channelID)
			usage := &relaymodel.Usage{
				PromptTokens:        120,
				CompletionTokens:    30,
				TotalTokens:         150,
				CacheWrite5mTokens:  10,
				CacheWrite1hTokens:  5,
				PromptTokensDetails: &relaymodel.UsagePromptTokensDetails{CachedTokens: 70},
			}

			tc.invoke(ctx, meta, requestID, traceID, usage, tc.modelName)

			assertCacheAwareConsumeLog(t, provisionalLogID, requestID, tc.wantPromptTokens, tc.wantCachedPromptTokens, usage.CompletionTokens)
		})
	}
}
