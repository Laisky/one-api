package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/graceful"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/vertexai"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/realtime"
)

// TestVertexLiveDurableAccounting verifies real SQLite balances and consume logs
// using channel-persisted prices and detached production settlement. Parameters:
// t owns database isolation. Returns: none; provider access is not simulated by
// permission guesses, and neither pricing nor durable updates are mocked.
func TestVertexLiveDurableAccounting(t *testing.T) {
	for _, tc := range []struct {
		name, model       string
		measured, pending bool
		group             float64
		want              int64
	}{
		{"measured", "gemini-3.8-live", true, false, 1, 4000},
		{"unlisted", "operator-configured-live-id", true, false, 1, 4000},
		{"idle", "gemini-3.8-live", false, false, 1, 0},
		{"missing_receipt", "gemini-3.8-live", false, true, 1, 72000},
		{"partial", "gemini-3.8-live", true, true, 1, 72000},
		{"group_once", "gemini-3.8-live", true, false, 2, 8000},
		{"free", "gemini-3.8-live", true, false, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupTokenAuthListModelsEnv(t)
			sqlDB, err := model.DB.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			sqlDB.SetMaxIdleConns(1)
			previousLog, previousBatch := model.LOG_DB, config.BatchUpdateEnabled
			previousLogEnabled, previousTraceMode := config.IsLogConsumeEnabled(), config.TraceWriteMode
			model.LOG_DB, config.BatchUpdateEnabled, config.TraceWriteMode = model.DB, false, "batch"
			config.SetLogConsumeEnabled(true)
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				require.NoError(t, graceful.Drain(ctx))
				model.LOG_DB, config.BatchUpdateEnabled, config.TraceWriteMode = previousLog, previousBatch, previousTraceMode
				config.SetLogConsumeEnabled(previousLogEnabled)
			})
			require.NoError(t, model.DB.AutoMigrate(&model.Log{}))
			const balance int64 = 1_000_000
			userUUID := "018f0000-0000-7000-8000-000000000941"
			tokenUUID := "018f0000-0000-7000-8000-000000000942"
			channelUUID := "018f0000-0000-7000-8000-000000000943"
			require.NoError(t, model.DB.Create(&model.User{Id: 941, UUID: userUUID, Username: "vertex-live-user", Password: "hash", Status: model.UserStatusEnabled, Group: "default", Quota: balance}).Error)
			require.NoError(t, model.DB.Create(&model.Token{Id: 942, UUID: tokenUUID, UserId: 941, UserUUID: &userUUID, Key: "vertex-live-fixture", Name: "vertex-token", Status: model.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: balance}).Error)
			channel := &model.Channel{Id: 943, UUID: channelUUID, Type: channeltype.VertextAI, Name: "vertex-channel", Status: model.ChannelStatusEnabled, Group: "default", Models: tc.model}
			require.NoError(t, channel.SetModelPriceConfigs(map[string]model.ModelConfigLocal{tc.model: vertexLivePrices()}))
			require.NoError(t, model.DB.Create(channel).Error)
			var persisted model.Channel
			require.NoError(t, model.DB.First(&persisted, channel.Id).Error)
			prices := persisted.GetModelPriceConfigsWithContext(context.Background())
			m := vertexLiveFixtureMeta(tc.model)
			m.UserId, m.UserUUID, m.TokenId, m.TokenUUID = 941, userUUID, 942, tokenUUID
			m.TokenName, m.ChannelId, m.ChannelUUID, m.StartTime = "vertex-token", 943, channelUUID, time.Now()
			provider := resolveRealtimePricingAdaptor(m)
			require.IsType(t, &vertexai.Adaptor{}, provider)
			reserved, err := estimateRealtimeSessionReservation(m, 1, tc.group, prices, provider)
			require.NoError(t, err)
			if reserved > 0 {
				require.NoError(t, model.PreConsumeTokenQuota(context.Background(), m.TokenId, reserved))
			}
			collector := realtime.NewGeminiLedger()
			if tc.measured {
				require.NoError(t, collector.Observe([]byte(`{"serverContent":{"modelTurn":{},"turnComplete":true},"usageMetadata":{"promptTokenCount":400,"responseTokenCount":200,"totalTokenCount":600,"thoughtsTokenCount":20,"promptTokensDetails":[{"modality":"TEXT","tokenCount":100},{"modality":"AUDIO","tokenCount":100},{"modality":"IMAGE","tokenCount":100},{"modality":"VIDEO","tokenCount":100}],"responseTokensDetails":[{"modality":"TEXT","tokenCount":100},{"modality":"AUDIO","tokenCount":100}]}}`)))
			}
			ledger := collector.Finish(tc.pending)
			usage := &relaymodel.Usage{Realtime: ledger, PromptTokens: int(ledger.InputTokens), CompletionTokens: int(ledger.OutputTokens)}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime?model=alias", nil)
			requestID := "vertex-accounting-" + tc.name
			c.Set(ctxkey.RequestId, requestID)
			gmw.SetLogger(c, logger.Logger)
			ctx, cancel := context.WithCancel(c.Request.Context())
			c.Request = c.Request.WithContext(ctx)
			cancel()
			got := postConsumeRealtimeQuota(c, m, usage, reserved, 1, tc.group, nil, prices, nil, provider, 0)
			require.EqualValues(t, tc.want, got)
			drain, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			require.NoError(t, graceful.Drain(drain))
			var user model.User
			var token model.Token
			require.NoError(t, model.DB.First(&user, m.UserId).Error)
			require.NoError(t, model.DB.First(&token, m.TokenId).Error)
			require.NoError(t, model.DB.First(&persisted, m.ChannelId).Error)
			require.Equal(t, balance-tc.want, user.Quota)
			require.EqualValues(t, tc.want, user.UsedQuota)
			require.Equal(t, balance-tc.want, token.RemainQuota)
			require.Equal(t, tc.want, token.UsedQuota)
			require.EqualValues(t, tc.want, persisted.UsedQuota)
			var logs []model.Log
			require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", requestID, model.LogTypeConsume).Find(&logs).Error)
			require.Len(t, logs, 1)
			require.EqualValues(t, tc.want, logs[0].Quota)
			require.Equal(t, !tc.pending, logs[0].Metadata["realtime_billing_complete"])
			require.Equal(t, tc.pending, logs[0].Metadata[model.LogMetadataKeyEstimatedCharge] == true)
			encoded, err := json.Marshal(logs[0].Metadata)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), "vertex-live-fixture")
		})
	}
}

// TestVertexLiveUsesActualRequestMetadata verifies that ordinary metadata
// construction does not replace the configured region with a REST default.
// Parameters: t owns the request fixture. Returns: none; no endpoint is dialed.
func TestVertexLiveUsesActualRequestMetadata(t *testing.T) {
	t.Parallel()
	fixture := vertexLiveFixtureMeta("gemini-3.8-live")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime?model=alias", nil)
	gmw.SetLogger(c, logger.Logger)
	c.Set(ctxkey.Channel, channeltype.VertextAI)
	c.Set(ctxkey.Config, fixture.Config)
	c.Set(ctxkey.RequestModel, "alias")
	c.Set(ctxkey.ModelMapping, map[string]string{"alias": fixture.ActualModelName})
	m := meta.GetByContext(c)
	require.Equal(t, fixture.ActualModelName, m.ActualModelName)
	endpoint, err := vertexai.LiveRequestURL(m)
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("wss://%s-aiplatform.googleapis.com/ws/google.cloud.aiplatform.v1.LlmBidiService/BidiGenerateContent", fixture.Config.Region), endpoint)
}
