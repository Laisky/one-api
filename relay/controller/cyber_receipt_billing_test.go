package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestGPT6CyberReceiptSettlementHTTP exercises native and converted JSON/SSE
// responses through the real controllers and SQLite quota ledger. Parameter t
// owns fixtures; nothing returns. Standard receipts settle exactly, while an
// unsupported actual tier retains the reservation even after a paid failure.
func TestGPT6CyberReceiptSettlementHTTP(t *testing.T) {
	for _, protocol := range []string{"chat", "responses", "messages"} {
		for _, stream := range []bool{false, true} {
			for _, tc := range []struct {
				name, tier string
				failed     bool
			}{
				{name: "standard", tier: "default"},
				{name: "unpriced", tier: "fast"},
				{name: "paid_failure", tier: "fast", failed: true},
			} {
				t.Run(fmt.Sprintf("%s/stream=%t/%s", protocol, stream, tc.name), func(t *testing.T) {
					const balance = int64(100_000)
					xaiVideoSetup(t, balance, false)
					var calls atomic.Int32
					observations := make(chan xaiVideoObservation, 1)
					upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						observation := xaiVideoObservation{Path: r.URL.Path}
						observation.Err = json.NewDecoder(r.Body).Decode(&observation.Body)
						if observation.Err == nil {
							var user model.User
							observation.Err = model.DB.First(&user, fallbackUserID).Error
							observation.UserQuota = user.Quota
						}
						observations <- observation
						status, failure := "completed", ""
						output := `[{"type":"message","id":"msg_cyber","role":"assistant","content":[{"type":"output_text","text":"ack"}]}]`
						if tc.failed {
							status, output = "failed", "[]"
							failure = `,"error":{"type":"server_error","message":"work failed after usage"}`
						}
						body := `{"id":"resp_cyber_billing","object":"response","model":"gpt-6-cyber","status":"` + status +
							`","service_tier":"` + tc.tier + `","output":` + output + `,"usage":{"input_tokens":20,"output_tokens":2,"total_tokens":22}` + failure + `}`
						if stream {
							w.Header().Set("Content-Type", "text/event-stream")
							if !tc.failed {
								_, _ = io.WriteString(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"item_id\":\"msg_cyber\",\"output_index\":0,\"content_index\":0,\"delta\":\"ack\"}\n\n")
							}
							_, _ = io.WriteString(w, "event: response."+status+"\ndata: {\"type\":\"response."+status+"\",\"response\":"+body+"}\n\ndata: [DONE]\n\n")
						} else {
							w.Header().Set("Content-Type", "application/json")
							_, _ = io.WriteString(w, body)
						}
					}))
					t.Cleanup(upstream.Close)
					previous := client.HTTPClient
					client.HTTPClient = upstream.Client()
					t.Cleanup(func() { client.HTTPClient = previous })

					path, body := lyriaProtocolRequest(protocol, stream)
					c, writer, requestID := protocolContext(t, channeltype.OpenAI, "gpt-6-cyber", path, body, upstream.URL, balance, 1, false, nil)
					clearNativeSettledLedger(t, requestID)
					if protocol == "responses" {
						// Native Responses routing uses the actual provider host contract;
						// the endpoint override keeps every HTTP call on the local fixture.
						c.Set(ctxkey.BaseURL, "https://api.openai.com")
						c.Set(ctxkey.Config, model.ChannelConfig{EndpointURLs: map[string]string{
							channeltype.RelayModeToEndpointName(relaymode.ResponseAPI): upstream.URL + "/v1/responses",
						}})
						require.True(t, supportsNativeResponseAPI(metalib.GetByContext(c)))
					}
					apiErr := relayLyriaProtocol(c, protocol)
					drainCriticalTasks(t)
					require.EqualValues(t, 1, calls.Load())
					observation := <-observations
					require.NoError(t, observation.Err)
					require.Equal(t, "/v1/responses", observation.Path)
					require.Equal(t, "gpt-6-cyber", observation.Body["model"])
					require.Equal(t, "default", observation.Body["service_tier"], "all entrypoints must request the qualified tier")
					reserved := balance - observation.UserQuota
					require.Greater(t, reserved, int64(300))
					charge := int64(300) // 20 input at $20/M plus 2 output at $100/M.
					if tc.tier != "default" {
						charge = reserved
					}
					require.Equal(t, balance-charge, reloadUserQuota(t))
					var token model.Token
					require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
					require.Equal(t, balance-charge, token.RemainQuota)
					require.Equal(t, charge, requestCostQuota(t, requestID))
					var logs []model.Log
					require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", requestID, model.LogTypeConsume).Find(&logs).Error)
					require.Len(t, logs, 1)
					require.EqualValues(t, charge, logs[0].Quota)
					reason, _ := logs[0].Metadata["billing_estimate_reason"].(string)
					if tc.tier == "default" {
						require.Empty(t, reason)
					} else {
						require.NotEmpty(t, reason, "an unpriced receipt must be recorded as estimated")
					}
					if !tc.failed {
						require.Nil(t, apiErr)
						require.Contains(t, writer.Body.String(), "ack", "receipt capture must preserve the delivered response")
					}
					require.False(t, BillingAllowsRetry(c), "reported paid work must not be retried and billed twice")
				})
			}
		}
	}
}
