package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/stretchr/testify/require"
)

// TestSecurityReasoningHTTPSettlement reaches the production relay, adaptor and durable owner/token ledger.
func TestSecurityReasoningHTTPSettlement(t *testing.T) {
	const balance = int64(10_000_000)
	xaiVideoSetup(t, balance, false)
	require.Equal(t, apitype.DeepSeek, channeltype.ToAPIType(channeltype.DeepSeek), "fixture must reach the registered shared-response adaptor")
	reasoning := strings.Repeat("careful reasoning explanation ", 2000)
	raw := "<think>" + reasoning + "</think>answer"
	expected := int64(10 + openai_compatible.CountTokenText(raw, "deepseek-chat"))
	var calls atomic.Int32
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/v1/chat/completions" {
			http.Error(w, "unexpected provider protocol", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "synthetic-reasoning", "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": raw}, "finish_reason": "stop"}}, "usage": map[string]any{"prompt_tokens": 10}})
	}))
	defer upstream.Close()
	old := client.HTTPClient
	client.HTTPClient = upstream.Client()
	defer func() { client.HTTPClient = old }()
	c, w, id := protocolContext(t, channeltype.DeepSeek, "deepseek-chat", "/v1/chat/completions?thinking=true&reasoning_format=reasoning_content", `{"model":"alias","messages":[{"role":"user","content":"hello"}]}`, upstream.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
	require.Nil(t, RelayTextHelper(c))
	drainCriticalTasks(t)
	require.EqualValues(t, 1, calls.Load())
	var delivered openai_compatible.SlimTextResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &delivered))
	require.Len(t, delivered.Choices, 1)
	require.Equal(t, "answer", delivered.Choices[0].Message.StringContent())
	require.NotNil(t, delivered.Choices[0].Message.ReasoningContent)
	require.Equal(t, strings.TrimSpace(reasoning), *delivered.Choices[0].Message.ReasoningContent)
	got := requestCostQuota(t, id)
	if got < expected {
		t.Log("REPRODUCED_468_REASONING_LEDGER_UNDERCHARGE")
	}
	require.Equal(t, expected, got)
	require.Equal(t, balance-expected, reloadUserQuota(t))
	var token model.Token
	require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
	require.Equal(t, balance-expected, token.RemainQuota)
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", id, model.LogTypeConsume).Find(&logs).Error)
	require.Len(t, logs, 1, "one final consume row, not a second debit or orphan provisional row")
	require.EqualValues(t, expected, logs[0].Quota)
	require.Equal(t, true, logs[0].Metadata["billing_estimated"])
	require.NotEmpty(t, logs[0].Metadata["billing_estimate_reason"])
}
