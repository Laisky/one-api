package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/stretchr/testify/require"
)

// TestSecurityNativeResponseUnknownContinuationAdmission exercises two real
// native handler calls with an owner-scoped parent saved from a loopback provider.
// Parameters: t owns isolated SQLite and memory state. Returns: none; absent or
// ambiguous provider counters must stop a paid tier continuation before I/O,
// without changing balances or losing the already settled first request.
func TestSecurityNativeResponseUnknownContinuationAdmission(t *testing.T) {
	const name = "native-unknown-continuation-fixture"
	const output = 20
	local := model.ModelConfigLocal{Ratio: 1, CompletionRatio: 2, Tiers: []model.ModelRatioTierLocal{{
		InputTokenThreshold: 100, Ratio: 5, CompletionRatio: 100,
	}}}
	for _, stream := range []bool{false, true} {
		for _, scenario := range []string{"missing_usage", "empty_usage", "missing_input", "missing_output", "zero_input", "zero_output"} {
			t.Run(fmt.Sprintf("%s/stream=%t", scenario, stream), func(t *testing.T) {
				// Fund the first request, but leave less than the complete second
				// quote. The legacy incremental allowance still fits this balance.
				balance := int64(2500)
				if scenario == "missing_output" || scenario == "zero_output" {
					balance = 2930
				}
				xaiVideoSetup(t, balance, false)
				store := enableStateForTest(t)
				var calls atomic.Int32
				var firstPrompt atomic.Int64
				provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					round := calls.Add(1)
					var request openai.ResponseAPIRequest
					if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&request); err != nil {
						http.Error(w, "invalid local fixture body", http.StatusBadRequest)
						return
					}
					if request.Model != name || request.MaxOutputTokens == nil || *request.MaxOutputTokens != output || round > 2 {
						http.Error(w, "invalid local fixture controls", http.StatusBadRequest)
						return
					}
					prompt := getResponseAPIPromptTokens(r.Context(), &request)
					id := "resp_unknown_first"
					if round == 1 {
						firstPrompt.Store(int64(prompt))
					} else {
						if request.PreviousResponseId == nil || *request.PreviousResponseId != "resp_unknown_first" {
							http.Error(w, "invalid local fixture parent", http.StatusBadRequest)
							return
						}
						prompt += int(firstPrompt.Load()) + output
						id = "resp_unknown_second"
					}
					usage := map[string]int{"input_tokens": prompt, "output_tokens": output, "total_tokens": prompt + output}
					receipt := map[string]any{
						"id": id, "object": "response", "status": "completed", "model": name,
						"output": []any{map[string]any{
							"type": "message", "id": "msg_" + id, "status": "completed", "role": "assistant",
							"content": []any{map[string]any{"type": "output_text", "text": "answer", "annotations": []any{}}},
						}}, "usage": usage,
					}
					if round == 1 {
						switch scenario {
						case "missing_usage":
							delete(receipt, "usage")
						case "empty_usage":
							receipt["usage"] = map[string]int{}
						case "missing_input":
							delete(usage, "input_tokens")
						case "missing_output":
							delete(usage, "output_tokens")
						case "zero_input":
							usage["input_tokens"] = 0
						case "zero_output":
							usage["output_tokens"] = 0
						}
					}
					if round == 2 && stream {
						w.Header().Set("Content-Type", "text/event-stream")
						data, err := json.Marshal(map[string]any{"type": "response.completed", "response": receipt})
						if err != nil {
							http.Error(w, "invalid local fixture receipt", http.StatusInternalServerError)
							return
						}
						if _, err := fmt.Fprintf(w, "event: response.completed\ndata: %s\n\n", data); err != nil {
							return
						}
						return
					}
					w.Header().Set("Content-Type", "application/json")
					if err := json.NewEncoder(w).Encode(receipt); err != nil {
						return
					}
				}))
				t.Cleanup(provider.Close)
				old := client.HTTPClient
				client.HTTPClient = provider.Client()
				t.Cleanup(func() { client.HTTPClient = old })

				fields := map[string]any{"model": "alias", "input": strings.Repeat("input ", 200), "max_output_tokens": output, "store": true}
				body, err := json.Marshal(fields)
				require.NoError(t, err)
				c, _, firstID := protocolContext(t, channeltype.OpenAICompatible, name, "/v1/responses", string(body), provider.URL, balance, 1, false, &local)
				firstID += "-first"
				c.Set(ctxkey.RequestId, firstID)
				c.Set(ctxkey.Config, model.ChannelConfig{APIFormat: channeltype.OpenAICompatibleAPIFormatResponse})
				meta := metalib.GetByContext(c)
				meta.OriginModelName, meta.ActualModelName = name, name
				require.Nil(t, RelayResponseAPIHelper(c), "first request must complete even without a complete usage receipt")
				drainCriticalTasks(t)
				require.EqualValues(t, 1, calls.Load())
				record, err := store.GetResponse(context.Background(), stateOwnerFromMeta(meta), "resp_unknown_first")
				require.NoError(t, err, "parent must be saved by the real first handler, not fabricated")
				require.NotNil(t, record.Binding)
				firstCharge := consumeLogQuota(t, firstID)
				beforeUser := reloadUserQuota(t)
				var beforeToken model.Token
				require.NoError(t, model.DB.First(&beforeToken, fallbackTokenID).Error)

				fields["input"], fields["previous_response_id"], fields["stream"] = "next", record.GatewayResponseID, stream
				body, err = json.Marshal(fields)
				require.NoError(t, err)
				second, _, secondID := protocolContext(t, channeltype.OpenAICompatible, name, "/v1/responses", string(body), provider.URL, beforeUser, 1, false, &local)
				second.Set(ctxkey.RequestId, secondID+"-second")
				second.Set(ctxkey.Config, model.ChannelConfig{APIFormat: channeltype.OpenAICompatibleAPIFormatResponse})
				second.Set(ctxkey.TokenQuota, beforeToken.RemainQuota)
				secondMeta := metalib.GetByContext(second)
				secondMeta.OriginModelName, secondMeta.ActualModelName = name, name
				apiErr := RelayResponseAPIHelper(second)
				drainCriticalTasks(t)
				var afterToken model.Token
				require.NoError(t, model.DB.First(&afterToken, fallbackTokenID).Error)
				t.Logf("UNKNOWN_CONTINUATION scenario=%s stream=%t saved_usage=%s first_charge=%d second_hold=%d calls=%d before_user=%d after_user=%d before_token=%d after_token=%d api_error=%v",
					scenario, stream, record.Usage, firstCharge, second.GetInt64(ctxkey.PreConsumedQuotaAmount), calls.Load(), beforeUser, reloadUserQuota(t), beforeToken.RemainQuota, afterToken.RemainQuota, apiErr)
				require.EqualValues(t, 1, calls.Load(), "unknown inherited usage must reject before the second provider call")
				require.NotNil(t, apiErr)
				require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
				require.Zero(t, second.GetInt64(ctxkey.PreConsumedQuotaAmount))
				require.Equal(t, beforeUser, reloadUserQuota(t))
				require.Equal(t, beforeToken.RemainQuota, afterToken.RemainQuota)
				require.Equal(t, beforeToken.UsedQuota, afterToken.UsedQuota)
				require.Equal(t, firstCharge, consumeLogQuota(t, firstID), "first settlement remains intact")
			})
		}
	}
}
