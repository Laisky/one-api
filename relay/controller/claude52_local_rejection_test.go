package controller

import (
	"encoding/json"
	"fmt"
	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestClaude52LocalRejectionAccounting drives native controller admission and asserts settled financial records.
func TestClaude52LocalRejectionAccounting(t *testing.T) {
	ensureResponseFallbackFixtures(t)
	oldRedis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() { common.SetRedisEnabled(oldRedis) })
	oldLogging := config.IsLogConsumeEnabled()
	config.SetLogConsumeEnabled(true)
	t.Cleanup(func() { config.SetLogConsumeEnabled(oldLogging) })
	for _, tc := range []struct {
		name                string
		channel             int
		controls, wantError string
	}{
		{"native_fast", channeltype.Anthropic, `,"speed":"fast"`, "standard speed"},
		{"vertex_missing_tariff", channeltype.VertextAI, "", "explicit"},
		{"vertex_fast", channeltype.VertextAI, `,"speed":"fast"`, "standard speed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start := seedRetryDoubleChargeUser(t)
			var before model.Token
			require.NoError(t, model.DB.First(&before, fallbackTokenID).Error)
			var hits atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); w.WriteHeader(500) }))
			defer upstream.Close()
			oldClient := client.HTTPClient
			client.HTTPClient = upstream.Client()
			defer func() { client.HTTPClient = oldClient }()
			c := setupClaudeRetryContext(t, httptest.NewRecorder(), upstream.URL)
			c.Set(ctxkey.Channel, tc.channel)
			ch := &model.Channel{Id: fallbackAnthropicChannelID, Type: tc.channel}
			if tc.name == "vertex_fast" {
				raw, err := json.Marshal(map[string]model.ModelConfigLocal{"claude-fable-5-2": {Ratio: 4, CompletionRatio: 5, CachedInputRatio: .1, CacheWrite5mRatio: 5, CacheWrite1hRatio: 8}})
				require.NoError(t, err)
				saved := string(raw)
				ch.ModelConfigs = &saved
			}
			c.Set(ctxkey.ChannelModel, ch)
			core, logs := observer.New(zapcore.DebugLevel)
			lg, err := glog.New(glog.WithName("claude52-local"), glog.WithZapOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core { return core })))
			require.NoError(t, err)
			gmw.SetLogger(c, lg)
			c.Set(ctxkey.RequestModel, "claude-fable-5-2")
			requestID := "claude52-local-" + tc.name
			c.Set(ctxkey.RequestId, requestID)
			require.NoError(t, model.UpdateUserRequestCostQuotaByRequestID(fallbackUserID, requestID, 0))
			payload := fmt.Sprintf(`{"model":"claude-fable-5-2","max_tokens":64,"messages":[{"role":"user","content":"hello"}]%s}`, tc.controls)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(payload))
			c.Request.Header.Set("Content-Type", "application/json")
			failure := RelayClaudeMessagesHelper(c)
			require.NotNil(t, failure)
			require.ErrorContains(t, failure.RawError, tc.wantError)
			if tc.name == "vertex_missing_tariff" {
				require.Zero(t, c.GetInt64(ctxkey.PreConsumedQuotaAmount), "shared tariff admission must precede reservation")
			} else {
				require.Positive(t, c.GetInt64(ctxkey.PreConsumedQuotaAmount), "late final-body failure must exercise actual reservation")
			}
			require.Zero(t, hits.Load())
			require.False(t, c.GetBool(ctxkey.UpstreamRequestPossiblyForwarded))
			drainCriticalTasks(t)
			require.Equal(t, start, reloadUserQuota(t))
			var after model.Token
			require.NoError(t, model.DB.First(&after, fallbackTokenID).Error)
			require.Equal(t, before.RemainQuota, after.RemainQuota)
			require.Equal(t, before.UsedQuota, after.UsedQuota)
			require.Zero(t, requestCostQuota(t, requestID))
			if tc.name == "vertex_missing_tariff" {
				require.Zero(t, c.GetInt(ctxkey.ProvisionalLogId), "early rejection must not create a consumption log")
			} else {
				var log model.Log
				require.Positive(t, c.GetInt(ctxkey.ProvisionalLogId))
				require.NoError(t, model.LOG_DB.First(&log, c.GetInt(ctxkey.ProvisionalLogId)).Error)
				require.Zero(t, log.Quota, "local rejection must reconcile provisional consume log")
				require.True(t, c.GetBool(ctxkey.BillingReconciled), "normal settlement must own terminal rejection")
				require.True(t, c.GetBool(ctxkey.PreConsumedQuotaRefundClaimed))
			}
			require.Empty(t, logs.FilterMessageSnippet("CRITICAL BILLING AUDIT").All())
			billingAuditSafetyNet(c)
			ResetPerAttemptBillingForRetry(c.Request.Context(), c)
			drainCriticalTasks(t)
			require.Equal(t, start, reloadUserQuota(t), "audit and retry must not double refund")
		})
	}
}
