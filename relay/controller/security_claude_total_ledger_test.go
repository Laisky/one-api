package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/stretchr/testify/require"
)

// TestSecurityClaudeTotalOnlyHTTPSettlement observes a real converted stream and the exact durable final debit.
func TestSecurityClaudeTotalOnlyHTTPSettlement(t *testing.T) {
	const balance = int64(10_000_000)
	xaiVideoSetup(t, balance, false)
	var calls atomic.Int32
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"}}]}\n\n"+
			"data: {\"choices\":[],\"usage\":{\"total_tokens\":100}}\n\n"+"data: [DONE]\n\n")
	}))
	defer upstream.Close()
	old := client.HTTPClient
	client.HTTPClient = upstream.Client()
	defer func() { client.HTTPClient = old }()
	c, w, id := protocolContext(t, channeltype.OpenAI, "gpt-4", "/v1/messages", `{"model":"alias","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"hello"}]}`, upstream.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
	require.Nil(t, RelayClaudeMessagesHelper(c))
	drainCriticalTasks(t)
	require.EqualValues(t, 1, calls.Load())
	require.Contains(t, w.Body.String(), "ok")
	t.Logf("Observed admission hold: %d", c.GetInt64(ctxkey.PreConsumedQuotaAmount))
	got := requestCostQuota(t, id)
	if got != 100 {
		t.Log("REPRODUCED_465_TOTAL_ONLY_LEDGER_UNDERCHARGE")
	}
	require.EqualValues(t, 100, got)
	require.EqualValues(t, balance-100, reloadUserQuota(t))
	var token model.Token
	require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
	require.EqualValues(t, balance-100, token.RemainQuota)
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", id, model.LogTypeConsume).Find(&logs).Error)
	require.Len(t, logs, 1, "one final consume row, not a second debit or orphan provisional row")
	require.EqualValues(t, int64(100), logs[0].Quota)
	require.Equal(t, true, logs[0].Metadata["billing_estimated"])
	require.NotEmpty(t, logs[0].Metadata["billing_estimate_reason"])
}
