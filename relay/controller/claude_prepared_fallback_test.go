package controller

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestClaudePreparedFallbackHTTPSettlement checks actual captured image-as-text work retains its prepared hold when usage is absent.
func TestClaudePreparedFallbackHTTPSettlement(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		measured      bool
	}{
		{"missing_empty", "", false},
		{"missing_text", "synthetic visible completion", false},
		{"authoritative", "synthetic visible completion", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance = int64(200000)
			const actualModel = "claude-3-5-haiku-20241022"
			xaiVideoSetup(t, balance, false)
			canonicalAdmissionConfiguration(t)
			pngData := claudeToolBoundaryPNG(t)
			var calls atomic.Int32
			seen := make(chan claudeToolQuotaWire, 1)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
				select {
				case seen <- claudeToolQuotaWire{Body: body, Path: r.URL.Path, Err: err}:
				default:
				}
				w.Header().Set("Content-Type", "application/json")
				response := map[string]any{"id": "synthetic-fallback", "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": tc.content}, "finish_reason": "stop"}}}
				if tc.measured {
					response["usage"] = map[string]any{"prompt_tokens": 11, "completion_tokens": 7, "total_tokens": 18}
				}
				_ = json.NewEncoder(w).Encode(response)
			}))
			t.Cleanup(server.Close)
			oldClient := client.HTTPClient
			client.HTTPClient = server.Client()
			client.HTTPClient.Timeout = 5 * time.Second
			t.Cleanup(func() { client.HTTPClient = oldClient })
			payload := map[string]any{"model": "alias", "max_tokens": 1,
				"messages": []any{
					map[string]any{"role": "user", "content": "Run the synthetic lookup."},
					map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "tool_use", "id": "synthetic-call", "name": "lookup", "input": map[string]any{"query": "fixture"}}}},
					map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": "synthetic-call", "content": []any{map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": pngData}}}}}},
				},
				"tools": []any{map[string]any{"name": "lookup", "input_schema": map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}}}},
			}
			body, err := json.Marshal(payload)
			require.NoError(t, err)
			c, w, id := protocolContext(t, channeltype.OpenAICompatible, actualModel, "/v1/messages", string(body), server.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
			require.Nil(t, RelayClaudeMessagesHelper(c))
			drainCriticalTasks(t)
			require.EqualValues(t, 1, calls.Load())
			require.Equal(t, http.StatusOK, w.Code)
			observed := <-seen
			require.NoError(t, observed.Err)
			require.Equal(t, "/v1/chat/completions", observed.Path)
			var wire relaymodel.GeneralOpenAIRequest
			require.NoError(t, json.Unmarshal(observed.Body, &wire))
			var toolText string
			for _, message := range wire.Messages {
				if message.Role == "tool" {
					toolText = message.StringContent()
				}
			}
			require.Contains(t, toolText, pngData)
			// Independent lower bound comes only from the actual captured provider text, never from the prepared/native estimators.
			wirePromptFloor := openai.CountTokenMessages(context.Background(), []relaymodel.Message{{Role: "tool", Content: toolText}}, actualModel)
			require.Greater(t, wirePromptFloor, 1000)
			hold := c.GetInt64(ctxkey.PreConsumedQuotaAmount)
			require.GreaterOrEqual(t, hold, int64(wirePromptFloor+1))
			userDebit := balance - reloadUserQuota(t)
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			cost := requestCostQuota(t, id)
			var logs []model.Log
			require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", id, model.LogTypeConsume).Find(&logs).Error)
			require.Len(t, logs, 1, "one reconciled consume row must account for the same provider work")
			t.Logf("PREPARED_FALLBACK_LEDGER measured=%v hold=%d captured_tool_floor=%d owner_debit=%d token_debit=%d request_cost=%d log_quota=%d log_prompt=%d provenance=%v", tc.measured, hold, wirePromptFloor, userDebit, balance-token.RemainQuota, cost, logs[0].Quota, logs[0].PromptTokens, logs[0].Metadata)
			want := max(hold, hold-1+int64(openai.CountTokenText(tc.content, actualModel)))
			if tc.measured {
				want = 18
			}
			require.Equal(t, want, userDebit)
			require.Equal(t, want, balance-token.RemainQuota)
			require.Equal(t, want, cost)
			require.EqualValues(t, want, logs[0].Quota)
			require.NotEqual(t, true, logs[0].Metadata[model.LogMetadataKeyProvisional])
			if tc.measured {
				require.Equal(t, 11, logs[0].PromptTokens)
				require.NotEqual(t, true, logs[0].Metadata["billing_estimated"])
			} else {
				require.GreaterOrEqual(t, logs[0].PromptTokens, wirePromptFloor)
				require.Equal(t, true, logs[0].Metadata["billing_estimated"])
				require.NotEmpty(t, logs[0].Metadata["billing_estimate_reason"])
			}
		})
	}
}
