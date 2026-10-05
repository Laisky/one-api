package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestSecurityWholeStreamFrameBudget measures all consumed choices/tool arguments
// before delivery. Oversized synthetic output is a defensive transport fixture,
// not evidence that a real provider ignores its requested output cap.
func TestSecurityWholeStreamFrameBudget(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		for _, kind := range []string{"tools", "large_tools", "choices", "authoritative_receipt"} {
			t.Run(fmt.Sprintf("fallback=%v/%s", fallback, kind), func(t *testing.T) {
				const balance = int64(150)
				xaiVideoSetup(t, balance, false)
				oldPre := config.PreConsumedQuota
				config.PreConsumedQuota = 20
				t.Cleanup(func() { config.PreConsumedQuota = oldPre })
				fragment := strings.Repeat("frame output ", 200)
				args := `{"payload":"` + strings.Repeat("tool argument ", 200) + `"}`
				if kind == "large_tools" {
					args = `{"payload":"` + strings.Repeat("tool argument ", 10_000) + `"}`
				}
				choices := []any{}
				outputTokens := 0
				for i := 0; i < 3; i++ {
					delta := map[string]any{"content": fragment}
					if strings.Contains(kind, "tools") {
						delta = map[string]any{"tool_calls": []any{map[string]any{"index": i, "id": fmt.Sprintf("call-%d", i), "type": "function", "function": map[string]any{"name": "synthetic", "arguments": args}}}}
						outputTokens += openai.CountTokenText(args, "gpt-4")
					} else {
						outputTokens += openai.CountTokenText(fragment, "gpt-4")
					}
					choices = append(choices, map[string]any{"index": i, "delta": delta})
				}
				event := map[string]any{"choices": choices}
				if kind == "authoritative_receipt" {
					event["usage"] = map[string]any{"prompt_tokens": 11, "completion_tokens": 7, "total_tokens": 18}
				}
				payload, err := json.Marshal(event)
				require.NoError(t, err)
				var calls atomic.Int32
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					_, _ = io.Copy(io.Discard, r.Body)
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", payload)
				}))
				t.Cleanup(server.Close)
				old := client.HTTPClient
				client.HTTPClient = server.Client()
				t.Cleanup(func() { client.HTTPClient = old })
				path, body := "/v1/chat/completions", `{"model":"alias","messages":[{"role":"user","content":"hello"}],"stream":true,"max_tokens":1}`
				if fallback {
					path, body = "/v1/responses", `{"model":"alias","input":"hello","stream":true,"max_output_tokens":1}`
				}
				c, w, id := protocolContext(t, channeltype.OpenAICompatible, "gpt-4", path, body, server.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
				var apiErr *relaymodel.ErrorWithStatusCode
				if fallback {
					apiErr = RelayResponseAPIHelper(c)
				} else {
					apiErr = RelayTextHelper(c)
				}
				drainCriticalTasks(t)
				require.EqualValues(t, 1, calls.Load())
				expected := int64(metalib.GetByContext(c).PromptTokens + outputTokens)
				if kind == "authoritative_receipt" {
					expected = 18
					require.Nil(t, apiErr, "a smaller authoritative receipt remains authoritative even for a large visible frame")
				} else {
					require.NotNil(t, apiErr)
				}
				charge := requestCostQuota(t, id)
				if charge != expected {
					t.Logf("REPRODUCED_WHOLE_FRAME_UNDERCHARGE actual=%d expected=%d", charge, expected)
				}
				require.Equal(t, expected, charge)
				require.Equal(t, balance-charge, reloadUserQuota(t))
				var token model.Token
				require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
				require.Equal(t, balance-charge, token.RemainQuota)
				var logs []model.Log
				require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", id, model.LogTypeConsume).Find(&logs).Error)
				require.Len(t, logs, 1)
				require.EqualValues(t, charge, logs[0].Quota)
				if kind != "authoritative_receipt" {
					require.NotContains(t, w.Body.String(), args)
					require.NotContains(t, w.Body.String(), fragment)
					if fallback {
						require.Contains(t, w.Body.String(), "event: response.failed")
						require.NotContains(t, w.Body.String(), "event: response.completed")
					}
				}
			})
		}
	}
}

// TestSecuritySharedMeasuredZero distinguishes a measured zero output counter
// from absent receipts; only absent evidence retains the conservative quote.
func TestSecuritySharedMeasuredZero(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		for _, measured := range []bool{false, true} {
			t.Run(fmt.Sprintf("fallback=%v/measured=%v", fallback, measured), func(t *testing.T) {
				const balance = int64(1000)
				xaiVideoSetup(t, balance, false)
				oldPre := config.PreConsumedQuota
				config.PreConsumedQuota = 20
				t.Cleanup(func() { config.PreConsumedQuota = oldPre })
				event := map[string]any{"choices": []any{}}
				if measured {
					event["usage"] = map[string]any{"prompt_tokens": 11, "completion_tokens": 0, "total_tokens": 11}
				}
				payload, err := json.Marshal(event)
				require.NoError(t, err)
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, _ = io.Copy(io.Discard, r.Body)
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", payload)
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
				if measured {
					expected = 11
				}
				charge := requestCostQuota(t, id)
				if measured && charge != 11 {
					t.Logf("REPRODUCED_MEASURED_ZERO_RELABELED actual=%d", charge)
				}
				require.Equal(t, expected, charge)
				require.Equal(t, balance-charge, reloadUserQuota(t))
				var token model.Token
				require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
				require.Equal(t, balance-charge, token.RemainQuota)
				var logs []model.Log
				require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", id, model.LogTypeConsume).Find(&logs).Error)
				require.Len(t, logs, 1)
				if measured {
					require.NotEqual(t, true, logs[0].Metadata["billing_estimated"])
				} else {
					require.Equal(t, true, logs[0].Metadata["billing_estimated"])
				}
			})
		}
	}
}
