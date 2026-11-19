package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/realtime"
)

// vertexLive38ChannelJSON is the operator's channel model_configs for the
// Vertex AI pricing row "Gemini 3.8 Live API", Non-global
// (https://cloud.google.com/vertex-ai/generative-ai/pricing, fetched
// 2026-11-18): text $0.75 in / $4.50 out, audio $3.00 in / $12.00 out,
// video/image $1.00 in and "Output: video (avatar)" $1.00 per 1M tokens. The
// %s verb receives the optional video block.
const vertexLive38ChannelJSON = `{"gemini-3.8-live": {"ratio": 0.375, "completion_ratio": 6,
	"audio": {"prompt_ratio": 4, "completion_ratio": 4}, "image": {"prompt_ratio": 1.3333333333333333}%s}}`

// TestVertexLiveVideoOutputDurableSettlement settles an avatar receipt (100
// text-in, 400 text-out and 1,000 VIDEO-out tokens) through the production
// collector, reservation and detached settlement against a SQLite ledger, with
// channel pricing persisted from operator JSON. With video.output_token_ratio
// the session settles exactly and completely; without it the charge and the
// incomplete-billing flag are unchanged. It accepts t and returns no values.
func TestVertexLiveVideoOutputDurableSettlement(t *testing.T) {
	const receipt = `{"serverContent":{"modelTurn":{},"turnComplete":true},"usageMetadata":{` +
		`"promptTokenCount":100,"responseTokenCount":1400,"totalTokenCount":1500,` +
		`"promptTokensDetails":[{"modality":"TEXT","tokenCount":100}],` +
		`"responseTokensDetails":[{"modality":"TEXT","tokenCount":400},{"modality":"VIDEO","tokenCount":1000}]}}`
	for _, tc := range []struct {
		name, video string
		complete    bool
		want        int64
	}{
		// 100 x $0.75 + 400 x $4.50 + 1000 x $1.00 = $0.002875 = 1437.5 quota.
		{"priced", `, "video": {"output_token_ratio": 1.3333333333333333}`, true, 1438},
		// Legacy: the 1000 avatar tokens at the $4.50 text-output rate = 3187.5.
		{"legacy", "", false, 3188},
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
			userUUID := "018f0000-0000-7000-8000-000000000951"
			tokenUUID := "018f0000-0000-7000-8000-000000000952"
			channelUUID := "018f0000-0000-7000-8000-000000000953"
			require.NoError(t, model.DB.Create(&model.User{Id: 951, UUID: userUUID, Username: "vertex-avatar-user", Password: "hash", Status: model.UserStatusEnabled, Group: "default", Quota: balance}).Error)
			require.NoError(t, model.DB.Create(&model.Token{Id: 952, UUID: tokenUUID, UserId: 951, UserUUID: &userUUID, Key: "vertex-avatar-fixture", Name: "vertex-avatar-token", Status: model.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: balance}).Error)
			var authored map[string]model.ModelConfigLocal
			require.NoError(t, json.Unmarshal([]byte(fmt.Sprintf(vertexLive38ChannelJSON, tc.video)), &authored))
			channel := &model.Channel{Id: 953, UUID: channelUUID, Type: channeltype.VertextAI, Name: "vertex-avatar-channel", Status: model.ChannelStatusEnabled, Group: "default", Models: "gemini-3.8-live"}
			require.NoError(t, channel.SetModelPriceConfigs(authored))
			require.NoError(t, model.DB.Create(channel).Error)
			var persisted model.Channel
			require.NoError(t, model.DB.First(&persisted, channel.Id).Error)
			prices := persisted.GetModelPriceConfigsWithContext(context.Background())

			m := vertexLiveFixtureMeta("gemini-3.8-live")
			m.UserId, m.UserUUID, m.TokenId, m.TokenUUID = 951, userUUID, 952, tokenUUID
			m.TokenName, m.ChannelId, m.ChannelUUID, m.StartTime = "vertex-avatar-token", 953, channelUUID, time.Now()
			provider := resolveRealtimePricingAdaptor(m)
			require.IsType(t, &vertexai.Adaptor{}, provider)
			reserved, err := estimateRealtimeSessionReservation(m, 1, 1, prices, provider)
			require.NoError(t, err)
			require.Greater(t, reserved, tc.want, "the admission hold covers the receipt")
			require.NoError(t, model.PreConsumeTokenQuota(context.Background(), m.TokenId, reserved))

			collector := realtime.NewGeminiLedger()
			require.NoError(t, collector.Observe([]byte(receipt)))
			ledger := collector.Finish(false)
			usage := &relaymodel.Usage{Realtime: ledger, PromptTokens: int(ledger.InputTokens), CompletionTokens: int(ledger.OutputTokens)}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime?model=alias", nil)
			requestID := "vertex-avatar-" + tc.name
			c.Set(ctxkey.RequestId, requestID)
			gmw.SetLogger(c, logger.Logger)
			got := postConsumeRealtimeQuota(c, m, usage, reserved, 1, 1, nil, prices, nil, provider, 0)
			require.EqualValues(t, tc.want, got)
			drain, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			require.NoError(t, graceful.Drain(drain))

			var user model.User
			var token model.Token
			require.NoError(t, model.DB.First(&user, m.UserId).Error)
			require.NoError(t, model.DB.First(&token, m.TokenId).Error)
			require.Equal(t, balance-tc.want, user.Quota)
			require.Equal(t, balance-tc.want, token.RemainQuota)
			var logs []model.Log
			require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", requestID, model.LogTypeConsume).Find(&logs).Error)
			require.Len(t, logs, 1)
			require.Equal(t, tc.want, int64(logs[0].Quota))
			require.Equal(t, tc.complete, logs[0].Metadata["realtime_billing_complete"])
			issues, err := json.Marshal(logs[0].Metadata["realtime_billing_issues"])
			require.NoError(t, err)
			require.Equal(t, !tc.complete, strings.Contains(string(issues), "video output"), "billing issues: %s", issues)
		})
	}
}
