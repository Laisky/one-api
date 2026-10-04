package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/stretchr/testify/require"
)

// TestSecurityClaudeJSONObjectLedger bills object arguments delivered through the real Claude relay and durable owner/token ledger.
func TestSecurityClaudeJSONObjectLedger(t *testing.T) {
	const balance int64 = 10_000_000
	xaiVideoSetup(t, balance, false)
	text := strings.Repeat("synthetic accounting evidence ", 100)
	payload := `{"answer":"` + text + `"}`
	var calls atomic.Int32
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: "+`{"choices":[{"index":0,"delta":{"tool_calls":[{"id":"call_local","function":{"name":"lookup","arguments":`+payload+`}}]}}]}`+"\n\n"+"data: [DONE]\n\n")
	}))
	t.Cleanup(upstream.Close)
	previous := client.HTTPClient
	client.HTTPClient = upstream.Client()
	t.Cleanup(func() { client.HTTPClient = previous })
	c, w, id := protocolContext(t, channeltype.OpenAI, "gpt-4", "/v1/messages", `{"model":"alias","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"hello"}]}`, upstream.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
	require.Nil(t, RelayClaudeMessagesHelper(c))
	drainCriticalTasks(t)
	require.EqualValues(t, 1, calls.Load())
	require.Contains(t, w.Body.String(), text)
	cost := requestCostQuota(t, id)
	require.GreaterOrEqual(t, cost, int64(200), "delivered long object arguments exceed the small admission hold")
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
	require.Equal(t, true, logs[0].Metadata["billing_estimated"])
	require.NotEmpty(t, logs[0].Metadata["billing_estimate_reason"])
}
