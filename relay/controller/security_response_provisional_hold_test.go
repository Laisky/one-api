package controller

import (
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
)

// TestSecurityResponseProvisionalHoldEOF proves a normal HTTP EOF cannot turn a
// nonterminal receipt into final settlement and release the prepaid allowance.
func TestSecurityResponseProvisionalHoldEOF(t *testing.T) {
	const balance int64 = 100_000
	xaiVideoSetup(t, balance, false)
	var calls atomic.Int32
	var upstreamPath atomic.Value
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		upstreamPath.Store(r.URL.Path)
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: "+`{"type":"response.in_progress","response":{"id":"resp_provisional","status":"in_progress","output":[{"type":"web_search_call","id":"ws_provisional","action":{"type":"search","query":"synthetic"}}],"usage":{"input_tokens":11,"output_tokens":0,"total_tokens":11}}}`+"\n\n")
	}))
	t.Cleanup(server.Close)
	old := client.HTTPClient
	client.HTTPClient = server.Client()
	t.Cleanup(func() { client.HTTPClient = old })
	c, _, id := protocolContext(t, channeltype.OpenAICompatible, "gpt-4o", "/v1/responses", `{"model":"alias","input":"hello","stream":true,"max_output_tokens":128,"tools":[{"type":"web_search"}]}`, server.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
	c.Set(ctxkey.Config, model.ChannelConfig{APIFormat: channeltype.OpenAICompatibleAPIFormatResponse})
	require.True(t, supportsNativeResponseAPI(metalib.GetByContext(c)))
	value, _ := c.Get(ctxkey.ChannelModel)
	channel := value.(*model.Channel)
	require.NoError(t, channel.SetToolingConfig(&model.ChannelToolingConfig{Whitelist: []string{"web_search"}, Pricing: map[string]model.ToolPricingLocal{"web_search": {QuotaPerCall: 17}}}))
	require.Nil(t, RelayResponseAPIHelper(c))
	drainCriticalTasks(t)
	require.EqualValues(t, 1, calls.Load())
	require.Equal(t, "/v1/responses", upstreamPath.Load())
	held := c.GetInt64(ctxkey.PreConsumedQuotaAmount)
	require.Greater(t, held, int64(28), "fixture must distinguish final 11+17 usage from its pending allowance")
	actual := requestCostQuota(t, id)
	if actual != held {
		t.Logf("REPRODUCED_PROVISIONAL_EOF_RELEASE held=%d final=%d", held, actual)
	}
	require.Equal(t, held, actual)
	require.Equal(t, balance-held, reloadUserQuota(t))
	var token model.Token
	require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
	require.Equal(t, balance-held, token.RemainQuota)
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ? AND type IN ?", id, []int{model.LogTypeConsume, model.LogTypeProvisional}).Find(&logs).Error)
	require.Len(t, logs, 1)
	require.Equal(t, model.LogTypeConsume, logs[0].Type)
	require.EqualValues(t, held, logs[0].Quota)
	require.Equal(t, true, logs[0].Metadata["billing_estimated"])
}
