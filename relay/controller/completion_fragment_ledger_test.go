package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestDeepSeekCompletionFragmentLedger verifies the registered DeepSeek adapter
// settles all four short choices once in the physical owner and finite-token
// ledger, preserves measured receipts, and labels only missing-usage estimates.
func TestDeepSeekCompletionFragmentLedger(t *testing.T) {
	for _, thinking := range []bool{false, true} {
		for _, measured := range []bool{false, true} {
			name := "ordinary"
			if thinking {
				name = "thinking"
			}
			if measured {
				name += "/measured"
			} else {
				name += "/estimated"
			}
			t.Run(name, func(t *testing.T) {
				const balance = int64(100000)
				xaiVideoSetup(t, balance, false)
				// Disable the optional admission buffer so it cannot conceal lost output fragments.
				previousPre := config.PreConsumedQuota
				config.PreConsumedQuota = 0
				t.Cleanup(func() { config.PreConsumedQuota = previousPre })
				require.Equal(t, apitype.DeepSeek, channeltype.ToAPIType(channeltype.DeepSeek))
				var calls atomic.Int32
				type observation struct {
					path, method, model string
					err                 error
				}
				seen := make(chan observation, 1)
				upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					var request map[string]any
					err := json.NewDecoder(r.Body).Decode(&request)
					actual, _ := request["model"].(string)
					seen <- observation{r.URL.Path, r.Method, actual, err}
					choices := make([]any, 0, 4)
					for index := range 4 {
						choices = append(choices, map[string]any{"index": index, "message": map[string]any{"role": "assistant", "content": "abc"}, "finish_reason": "stop"})
					}
					usage := map[string]any{"prompt_tokens": 10}
					if measured {
						usage["completion_tokens"] = 7
						usage["total_tokens"] = 17
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]any{"id": "fragment-fixture", "choices": choices, "usage": usage})
				}))
				defer upstream.Close()
				previous := client.HTTPClient
				client.HTTPClient = upstream.Client()
				defer func() { client.HTTPClient = previous }()
				path := "/v1/chat/completions"
				if thinking {
					path += "?thinking=true&reasoning_format=reasoning_content"
				}
				c, w, id := protocolContext(t, channeltype.DeepSeek, "deepseek-chat", path,
					"{\"model\":\"alias\",\"messages\":[{\"role\":\"user\",\"content\":\"hello\"}]}", upstream.URL,
					balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
				require.Nil(t, RelayTextHelper(c))
				drainCriticalTasks(t)
				require.EqualValues(t, 1, calls.Load())
				observed := <-seen
				require.NoError(t, observed.err)
				require.Equal(t, http.MethodPost, observed.method)
				require.Equal(t, "/v1/chat/completions", observed.path)
				require.Equal(t, "deepseek-chat", observed.model)
				completion := 3
				if measured {
					completion = 7
				}
				charge := int64(10 + completion)
				require.Equal(t, charge, requestCostQuota(t, id), "fragmented completion must reach the durable request ledger")
				var delivered openai_compatible.SlimTextResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &delivered))
				require.Len(t, delivered.Choices, 4)
				for _, choice := range delivered.Choices {
					require.Equal(t, "abc", choice.Message.StringContent())
				}
				require.Equal(t, completion, delivered.Usage.CompletionTokens)
				require.Equal(t, balance-charge, reloadUserQuota(t))
				var token model.Token
				require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
				require.Equal(t, fallbackUserID, token.UserId)
				require.Equal(t, balance-charge, token.RemainQuota)
				require.Equal(t, charge, token.UsedQuota)
				var logs []model.Log
				require.NoError(t, model.LOG_DB.Where("request_id = ? AND type IN ?", id, []int{model.LogTypeConsume, model.LogTypeProvisional}).Find(&logs).Error)
				require.Len(t, logs, 1, "one reconciled consume row owns the final debit")
				require.Equal(t, model.LogTypeConsume, logs[0].Type)
				require.Equal(t, fallbackUserID, logs[0].UserId)
				require.Equal(t, fallbackChannelID, logs[0].ChannelId)
				require.EqualValues(t, charge, logs[0].Quota)
				require.Equal(t, completion, logs[0].CompletionTokens)
				require.Equal(t, !measured, logs[0].Metadata["billing_estimated"] == true)
			})
		}
	}
}
