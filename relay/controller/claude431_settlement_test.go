package controller

import (
	"encoding/json"
	"fmt"
	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/stretchr/testify/require"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestClaude431HTTPSettlement verifies actual database debits for native and converted HTTP receipts.
// Truncation and delivery failures retain consumed tokens; unknown receipts retain the reservation.
func TestClaude431HTTPSettlement(t *testing.T) {
	ensureResponseFallbackFixtures(t)
	oldRedis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() { common.SetRedisEnabled(oldRedis) })
	oldLogging := config.IsLogConsumeEnabled()
	config.SetLogConsumeEnabled(false)
	t.Cleanup(func() { config.SetLogConsumeEnabled(oldLogging) })
	for _, native := range []bool{false, true} {
		for _, scenario := range []struct {
			name                                           string
			stream, truncated, writeFail, missing, partial bool
		}{
			{name: "json"}, {name: "stream", stream: true}, {name: "truncated", stream: true, truncated: true},
			{name: "partial_stream", stream: true, partial: true},
			{name: "failed_delivery", writeFail: true}, {name: "missing_json", missing: true}, {name: "missing_stream", missing: true, stream: true},
		} {
			t.Run(fmt.Sprintf("native-%t/%s", native, scenario.name), func(t *testing.T) {
				start := seedRetryDoubleChargeUser(t)
				c := setupClaudeRetryContext(t, httptest.NewRecorder(), "")
				m := meta.GetByContext(c)
				m.APIType = apitype.Anthropic
				m.IsStream = scenario.stream
				m.ActualModelName = "claude-sonnet-5-5"
				m.OriginModelName = m.ActualModelName
				c.Set(ctxkey.RequestModel, m.ActualModelName)
				var request ClaudeMessagesRequest
				require.NoError(t, json.Unmarshal([]byte(`{"model":"claude-sonnet-5-5","max_tokens":1024,"messages":[{"role":"user","content":"Hello"}]}`), &request))
				cfg := anthropic.ModelRatios[m.ActualModelName]
				// A deliberately larger image allowance must never become the final measured token charge.
				reservation, admissionErr := preConsumeClaudeMessagesQuota(c, &request, 4784, cfg.Ratio, cfg.CompletionRatio, m)
				require.Nil(t, admissionErr)
				require.Positive(t, reservation)
				markPreConsumed(c, reservation)
				body := `{"id":"msg_settlement","type":"message","role":"assistant","model":"claude-sonnet-5-5","content":[{"type":"text","text":"Hello"}],"stop_reason":"end_turn","usage":` + awsSettlementReceipt + `}`
				if scenario.stream {
					events := []string{
						`{"type":"message_start","message":{"id":"msg_settlement","type":"message","model":"claude-sonnet-5-5","content":[],"usage":` + awsSettlementReceipt + `}}`,
						`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":` + awsSettlementReceipt + `}`,
					}
					if scenario.partial {
						events = events[:1]
					}
					if !scenario.truncated && !scenario.partial {
						events = append(events, `{"type":"message_stop"}`)
					}
					body = "data: " + strings.Join(events, "\n\ndata: ") + "\n\n"
				}
				if scenario.missing {
					body = ""
				}
				if scenario.writeFail {
					c.Writer = &awsSettlementFailedWriter{ResponseWriter: c.Writer}
				}
				response := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}
				var usage *relaymodel.Usage
				var responseErr *relaymodel.ErrorWithStatusCode
				if scenario.stream {
					if native {
						responseErr, usage = anthropic.ClaudeNativeStreamHandler(c, response)
					} else {
						responseErr, usage = anthropic.StreamHandler(c, response)
					}
				} else {
					if native {
						responseErr, usage = anthropic.ClaudeNativeHandler(c, response, 4784, m.ActualModelName)
					} else {
						responseErr, usage = anthropic.Handler(c, response, 4784, m.ActualModelName)
					}
				}
				require.NotNil(t, usage)
				if scenario.truncated || scenario.writeFail || scenario.missing || scenario.partial {
					require.NotNil(t, responseErr)
				} else {
					require.Nil(t, responseErr)
				}
				markResponseSettlement(c, usage, responseErr)
				markBillingReconciled(c)
				bctx := detachForBilling(c)
				charged := postConsumeClaudeMessagesQuotaWithTraceID(bctx, "req_http_settlement_"+scenario.name, "", usage, m, &request, cfg.Ratio, reservation, 0, cfg.Ratio, nil, 1, nil, nil)
				drainCriticalTasks(t)
				want := int64(math.Ceil(21*cfg.Ratio + 50*cfg.Ratio*cfg.CompletionRatio + 1000*cfg.CachedInputRatio + 100*cfg.CacheWrite5mRatio + 200*cfg.CacheWrite1hRatio))
				if scenario.missing || scenario.partial {
					want = reservation
					require.NotEmpty(t, usage.BillingEstimateReason)
				}
				require.Equal(t, want, charged)
				require.Equal(t, charged, start-reloadUserQuota(t))
				var token model.Token
				require.NoError(t, model.DB.Where("id = ?", fallbackTokenID).First(&token).Error)
				require.Equal(t, charged, start-token.RemainQuota)
				require.False(t, BillingAllowsRetry(c))
				ResetPerAttemptBillingForRetry(bctx, c)
				drainCriticalTasks(t)
				require.Equal(t, charged, start-reloadUserQuota(t), "retry reset must not refund settled usage")
			})
		}
	}
}
