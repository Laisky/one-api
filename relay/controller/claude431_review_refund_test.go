package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestClaude431ReviewTerminalRejectionRefund drives real HTTP dispatch and database billing.
// A complete admission rejection refunds once; ambiguous failures retain the hold and forbid replay.
func TestClaude431ReviewTerminalRejectionRefund(t *testing.T) {
	ensureResponseFallbackFixtures(t)
	oldRedis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() { common.SetRedisEnabled(oldRedis) })
	oldLogging := config.IsLogConsumeEnabled()
	config.SetLogConsumeEnabled(false)
	t.Cleanup(func() { config.SetLogConsumeEnabled(oldLogging) })
	for _, channel := range []int{channeltype.Anthropic, channeltype.Azure} {
		for _, path := range []string{"/v1/messages", "/v1/chat/completions", "/v1/responses"} {
			for _, errorType := range []string{"invalid_request_error", "rate_limit_error", "api_error", "future_error"} {
				t.Run(fmt.Sprintf("channel-%d/%s/%s", channel, path, errorType), func(t *testing.T) {
					start := seedRetryDoubleChargeUser(t)
					var tokenBefore model.Token
					require.NoError(t, model.DB.First(&tokenBefore, fallbackTokenID).Error)
					var hits atomic.Int64
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						hits.Add(1)
						w.Header().Set("Content-Type", "application/json")
						// Model an intermediary that rewrites the HTTP status but preserves the error envelope.
						_ = json.NewEncoder(w).Encode(map[string]any{"type": "error", "error": map[string]string{"type": errorType, "message": "synthetic rejection"}})
					}))
					defer upstream.Close()
					oldClient := client.HTTPClient
					client.HTTPClient = upstream.Client()
					defer func() { client.HTTPClient = oldClient }()
					w := httptest.NewRecorder()
					c := setupClaudeRetryContext(t, w, upstream.URL)
					c.Set(ctxkey.Channel, channel)
					c.Set(ctxkey.ChannelModel, &model.Channel{Id: fallbackAnthropicChannelID, Type: channel})
					c.Set(ctxkey.RequestModel, "claude-sonnet-5-5")
					payload := `{"model":"claude-sonnet-5-5","max_tokens":1024,"messages":[{"role":"user","content":"hello"}]}`
					var handler func(*gin.Context) *relaymodel.ErrorWithStatusCode
					switch path {
					case "/v1/messages":
						handler = RelayClaudeMessagesHelper
					case "/v1/chat/completions":
						handler = RelayTextHelper
					case "/v1/responses":
						payload = `{"model":"claude-sonnet-5-5","max_output_tokens":1024,"input":"hello"}`
						handler = RelayResponseAPIHelper
					}
					c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(payload))
					c.Request.Header.Set("Content-Type", "application/json")
					c.Request.Header.Set("Authorization", "Bearer fixture-key")
					failure := handler(c)
					require.NotNil(t, failure)
					drainCriticalTasks(t)
					require.Equal(t, int64(1), hits.Load(), "no hidden probes or retries")
					require.True(t, c.GetBool(ctxkey.UpstreamRequestPossiblyForwarded))
					reservation := c.GetInt64(ctxkey.PreConsumedQuotaAmount)
					require.Positive(t, reservation, "the test must exercise a real debit, not the trusted-user shortcut")
					rejected := errorType == "invalid_request_error" || errorType == "rate_limit_error"
					want := reservation
					if rejected {
						want = 0
						require.True(t, c.GetBool(ctxkey.PreConsumedQuotaRefundClaimed), "refund must own the hold before retry reset")
					}
					require.Equal(t, want, start-reloadUserQuota(t))
					var tokenAfter model.Token
					require.NoError(t, model.DB.First(&tokenAfter, fallbackTokenID).Error)
					require.Equal(t, want, tokenBefore.RemainQuota-tokenAfter.RemainQuota)
					require.Equal(t, want, tokenAfter.UsedQuota-tokenBefore.UsedQuota)
					require.Equal(t, want, requestCostQuota(t, c.GetString(ctxkey.RequestId)))
					require.Equal(t, rejected, BillingAllowsRetry(c))
					billingAuditSafetyNet(c)
					ResetPerAttemptBillingForRetry(c.Request.Context(), c)
					drainCriticalTasks(t)
					require.Equal(t, want, start-reloadUserQuota(t), "audit or retry reset must not credit twice")
				})
			}
		}
	}
}
