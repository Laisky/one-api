package controller

import (
	"encoding/json"
	"fmt"
	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSecuritySharedPartialReceiptPreservesQuote is a positive safety control:
// allowing a measured zero output must not let an absent output counter release
// the original quote. The same real provider/ledger route covers both protocols.
func TestSecuritySharedPartialReceiptPreservesQuote(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		for _, scenario := range []string{"input_only", "null_output", "missing_usage", "measured_zero_input", "output_after_receipt"} {
			t.Run(fmt.Sprintf("%v/%s", fallback, scenario), func(t *testing.T) {
				const balance = int64(1000)
				xaiVideoSetup(t, balance, false)
				oldPre := config.PreConsumedQuota
				config.PreConsumedQuota = 20
				t.Cleanup(func() { config.PreConsumedQuota = oldPre })
				event := map[string]any{"choices": []any{}, "usage": map[string]any{"prompt_tokens": 11}}
				switch scenario {
				case "null_output":
					event["usage"] = map[string]any{"prompt_tokens": 11, "completion_tokens": nil}
				case "missing_usage":
					delete(event, "usage")
				case "measured_zero_input":
					event["usage"] = map[string]any{"prompt_tokens": 0, "completion_tokens": 7, "total_tokens": 7}
				case "output_after_receipt":
					event["usage"] = map[string]any{"prompt_tokens": 11, "completion_tokens": 0, "total_tokens": 11}
				}
				payload, err := json.Marshal(event)
				require.NoError(t, err)
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, _ = io.Copy(io.Discard, r.Body)
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
					if scenario == "output_after_receipt" {
						_, _ = io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"later\"}}]}\n\n")
					}
					_, _ = io.WriteString(w, "data: [DONE]\n\n")
				}))
				t.Cleanup(server.Close)
				old := client.HTTPClient
				client.HTTPClient = server.Client()
				t.Cleanup(func() { client.HTTPClient = old })
				path, body := "/v1/chat/completions", `{"model":"alias","messages":[{"role":"user","content":"hello"}],"stream":true,"max_tokens":1}`
				if fallback {
					path, body = "/v1/responses", `{"model":"alias","input":"hello","stream":true,"max_output_tokens":1}`
				}
				c, _, id := protocolContext(t, channeltype.DeepSeek, "deepseek-chat", path, body, server.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
				if fallback {
					require.Nil(t, RelayResponseAPIHelper(c))
				} else {
					require.Nil(t, RelayTextHelper(c))
				}
				drainCriticalTasks(t)
				expected := int64(metalib.GetByContext(c).PromptTokens + 21)
				if scenario == "measured_zero_input" {
					expected = 7
				}
				charge := requestCostQuota(t, id)
				require.Equal(t, expected, charge)
				require.Equal(t, balance-charge, reloadUserQuota(t))
				var token model.Token
				require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
				require.Equal(t, balance-charge, token.RemainQuota)
				var logs []model.Log
				require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", id, model.LogTypeConsume).Find(&logs).Error)
				require.Len(t, logs, 1)
				if scenario == "measured_zero_input" {
					require.NotEqual(t, true, logs[0].Metadata["billing_estimated"])
				} else {
					require.Equal(t, true, logs[0].Metadata["billing_estimated"])
				}
			})
		}
	}
}
