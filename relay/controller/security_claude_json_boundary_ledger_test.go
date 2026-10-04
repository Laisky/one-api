package controller

import (
	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// TestSecurityClaudeJSONReviewBoundaryLedger exercises each reviewer shape through actual HTTP conversion and durable quota settlement.
func TestSecurityClaudeJSONReviewBoundaryLedger(t *testing.T) {
	text := strings.Repeat("synthetic accounting evidence ", 100)
	payload := `{"answer":"` + text + `"}`
	app := `{"text":"x","answer":"` + text + `"}`
	other := `{"other":"` + strings.Repeat("distinct synthetic completion evidence ", 100) + `"}`
	whitespace := `{"answer":` + strings.Repeat(" ", 2000) + `"` + text + `"}`
	cases := []struct {
		name, wire string
		lower      int
		measured   bool
	}{
		{"plain_object_whitespace", "data: " + `{"type":"response.output_json.delta","output_index":0,"delta":` + whitespace + `}` + "\n\n", openai_compatible.CountTokenText(whitespace, "gpt-4"), false},
		{"plain_object_delta", "data: " + `{"type":"response.output_json.delta","output_index":0,"delta":` + payload + `}` + "\n\n", openai_compatible.CountTokenText(payload, "gpt-4"), false},
		{"part_application_text", "data: " + `{"type":"response.output_json.done","output_index":0,"part":{"type":"output_json","json":` + app + `}}` + "\n\n", openai_compatible.CountTokenText(app, "gpt-4"), false},
		{"distinct_items", "data: " + `{"type":"response.output_json.delta","item_id":"a","output_index":0,"delta":{"partial_json":` + jsonReviewQuote(payload) + `}}` + "\n\n" + "data: " + `{"type":"response.output_json.done","item_id":"b","output_index":1,"json":` + other + `}` + "\n\n", openai_compatible.CountTokenText(payload+other, "gpt-4"), false},
		{"measured_control", "data: " + `{"type":"response.output_json.delta","output_index":0,"delta":` + payload + `}` + "\n\n" + `data: {"choices":[],"usage":{"prompt_tokens":7,"completion_tokens":3}}` + "\n\n", 10, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			const balance int64 = 10_000_000
			xaiVideoSetup(t, balance, false)
			var calls atomic.Int32
			upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, tc.wire+"data: [DONE]\n\n")
			}))
			t.Cleanup(upstream.Close)
			old := client.HTTPClient
			client.HTTPClient = upstream.Client()
			t.Cleanup(func() { client.HTTPClient = old })
			c, w, id := protocolContext(t, channeltype.OpenAI, "gpt-4", "/v1/messages", `{"model":"alias","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"hello"}]}`, upstream.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
			require.Nil(t, RelayClaudeMessagesHelper(c))
			drainCriticalTasks(t)
			require.Contains(t, w.Body.String(), text)
			require.EqualValues(t, 1, calls.Load())
			cost := requestCostQuota(t, id)
			if tc.measured {
				require.EqualValues(t, 10, cost)
			} else {
				require.GreaterOrEqual(t, cost, int64(tc.lower))
			}
			require.Equal(t, balance-cost, reloadUserQuota(t))
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			require.Equal(t, balance-cost, token.RemainQuota)
			require.Equal(t, cost, token.UsedQuota)
			var logs []model.Log
			require.NoError(t, model.LOG_DB.Where("request_id = ? AND type IN ?", id, []int{model.LogTypeConsume, model.LogTypeProvisional}).Find(&logs).Error)
			require.Len(t, logs, 1)
			require.Equal(t, model.LogTypeConsume, logs[0].Type)
			require.EqualValues(t, cost, logs[0].Quota)
		})
	}
}

// jsonReviewQuote quotes the deterministic fixture JSON string for a Responses delta envelope.
func jsonReviewQuote(text string) string { return `"` + strings.ReplaceAll(text, `"`, `\"`) + `"` }
