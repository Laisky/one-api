package controller

import (
	"context"
	"encoding/json"
	"fmt"
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
	"github.com/Laisky/one-api/relay/adaptor/gemini"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/realtime"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestGeminiLiveDurableAccounting exercises the real reservation, detached
// controller settlement and SQLite balances/logs. Parameters: t is the test
// handle. Returns: none. No mock replaces pricing or durable quota updates.
func TestGeminiLiveDurableAccounting(t *testing.T) {
	for _, tc := range []struct {
		name                         string
		measured, pending, unlimited bool
		group                        float64
		want                         int64
	}{
		{"measured", true, false, false, 1, 975},
		{"idle_refund", false, false, false, 1, 0},
		{"missing_receipt_estimate", false, true, false, 1, 22500},
		{"partial_estimate", true, true, false, 1, 22500},
		{"group_applied_once", true, false, false, 2, 1950},
		{"free_group", true, false, false, 0, 0},
		{"unlimited_token", true, false, true, 1, 975},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupTokenAuthListModelsEnv(t)
			sqlDB, err := model.DB.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			sqlDB.SetMaxIdleConns(1)
			previousLog, previousBatch := model.LOG_DB, config.BatchUpdateEnabled
			previousLogEnabled := config.IsLogConsumeEnabled()
			previousTraceMode := config.TraceWriteMode
			model.LOG_DB, config.BatchUpdateEnabled = model.DB, false
			config.TraceWriteMode = "batch"
			config.SetLogConsumeEnabled(true)
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				require.NoError(t, graceful.Drain(ctx))
				model.LOG_DB, config.BatchUpdateEnabled = previousLog, previousBatch
				config.TraceWriteMode = previousTraceMode
				config.SetLogConsumeEnabled(previousLogEnabled)
			})
			require.NoError(t, model.DB.AutoMigrate(&model.Log{}))
			const balance int64 = 1_000_000
			userUUID := "018f0000-0000-7000-8000-000000000901"
			tokenUUID := "018f0000-0000-7000-8000-000000000902"
			channelUUID := "018f0000-0000-7000-8000-000000000903"
			require.NoError(t, model.DB.Create(&model.User{Id: 901, UUID: userUUID, Username: "gemini-live-user", Password: "hash", Status: model.UserStatusEnabled, Group: "default", Quota: balance}).Error)
			require.NoError(t, model.DB.Create(&model.Token{Id: 902, UUID: tokenUUID, UserId: 901, UserUUID: &userUUID, Key: "gemini-live-fixture", Name: "live-token", Status: model.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: balance, UnlimitedQuota: tc.unlimited}).Error)
			require.NoError(t, model.DB.Create(&model.Channel{Id: 903, UUID: channelUUID, Type: channeltype.Gemini, Name: "live-channel", Status: model.ChannelStatusEnabled, Group: "default", Models: "gemini-3.8-live"}).Error)
			m := &meta.Meta{UserId: 901, UserUUID: userUUID, TokenId: 902, TokenUUID: tokenUUID, TokenName: "live-token", ChannelId: 903, ChannelUUID: channelUUID, ChannelType: channeltype.Gemini, APIType: apitype.Gemini, Mode: relaymode.Realtime, ActualModelName: "gemini-3.8-live", StartTime: time.Now()}
			reserved, err := estimateRealtimeSessionReservation(m, .75*ratio.MilliTokensUsd, tc.group, nil, &gemini.Adaptor{})
			require.NoError(t, err)
			if reserved > 0 {
				require.NoError(t, model.PreConsumeTokenQuota(context.Background(), m.TokenId, reserved))
			}
			var before model.User
			require.NoError(t, model.DB.First(&before, m.UserId).Error)
			require.Equal(t, balance-reserved, before.Quota, "reservation must reach durable user balance")
			collector := realtime.NewGeminiLedger()
			if tc.measured {
				require.NoError(t, collector.Observe([]byte(`{"serverContent":{"modelTurn":{},"turnComplete":true},"usageMetadata":{"promptTokenCount":100,"responseTokenCount":200,"totalTokenCount":300,"promptTokensDetails":[{"modality":"AUDIO","tokenCount":100}],"responseTokensDetails":[{"modality":"TEXT","tokenCount":100},{"modality":"AUDIO","tokenCount":100}]}}`)))
			}
			ledger := collector.Finish(tc.pending)
			usage := &relaymodel.Usage{Realtime: ledger, PromptTokens: int(ledger.InputTokens), CompletionTokens: int(ledger.OutputTokens)}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("GET", "/v1/realtime?model=gemini-3.8-live", nil)
			requestID := fmt.Sprintf("gemini-accounting-%s", tc.name)
			c.Set(ctxkey.RequestId, requestID)
			gmw.SetLogger(c, logger.Logger)
			ctx, cancel := context.WithCancel(c.Request.Context())
			c.Request = c.Request.WithContext(ctx)
			cancel() // The client is already disconnected when settlement starts.
			got := postConsumeRealtimeQuota(c, m, usage, reserved, .75*ratio.MilliTokensUsd, tc.group, nil, nil, nil, &gemini.Adaptor{}, 0)
			require.EqualValues(t, tc.want, got)
			drain, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			require.NoError(t, graceful.Drain(drain))
			var user model.User
			var token model.Token
			var channel model.Channel
			require.NoError(t, model.DB.First(&user, m.UserId).Error)
			require.NoError(t, model.DB.First(&token, m.TokenId).Error)
			require.NoError(t, model.DB.First(&channel, m.ChannelId).Error)
			require.Equal(t, balance-tc.want, user.Quota)
			require.EqualValues(t, tc.want, user.UsedQuota)
			require.EqualValues(t, tc.want, channel.UsedQuota)
			if tc.unlimited {
				require.Equal(t, balance, token.RemainQuota)
				require.Zero(t, token.UsedQuota)
			} else {
				require.Equal(t, balance-tc.want, token.RemainQuota)
				require.Equal(t, tc.want, token.UsedQuota)
			}
			var logs []model.Log
			require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", requestID, model.LogTypeConsume).Find(&logs).Error)
			require.Len(t, logs, 1, "settlement emits one consume log")
			require.EqualValues(t, tc.want, logs[0].Quota)
			require.Equal(t, !tc.pending, logs[0].Metadata["realtime_billing_complete"])
			require.Equal(t, tc.pending, logs[0].Metadata[model.LogMetadataKeyEstimatedCharge] == true)
			encoded, err := json.Marshal(logs[0].Metadata)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), "gemini-live-fixture")
		})
	}
}
