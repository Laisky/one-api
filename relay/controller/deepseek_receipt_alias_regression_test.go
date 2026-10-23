package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/stretchr/testify/require"
)

// TestDeepSeekReceiptAliasLedger checks actual HTTP dispatch and persisted
// accounting for canonical/alias receipts and unrelated-provider controls.
// A synthetic provider must never turn an unsupported DeepSeek receipt into a
// billable Search/Fetch operation.
func TestDeepSeekReceiptAliasLedger(t *testing.T) {
	for _, tc := range []struct {
		name, base, alias, source string
		local                     bool
		want                      int64
	}{
		{"canonical_search_deepseek", "https://api.deepseek.com/anthropic", "web_search", "counter", false, 16},
		{"alias_search_deepseek", "https://api.deepseek.com/anthropic", "web_search_preview", "counter", false, 16},
		{"alias_search_local_function_deepseek", "https://api.deepseek.com/anthropic", "web_search_preview", "block", true, 16},
		{"dated_fetch_deepseek", "https://api.deepseek.com/anthropic", "web_fetch_20250910", "counter", false, 16},
		{"alias_search_unrelated", "https://api.anthropic.com", "web_search_preview", "counter", false, 33},
		{"canonical_fetch_deepseek", "https://api.deepseek.com/anthropic", "web_fetch", "counter", false, 16},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance int64 = 1000000
			xaiVideoSetup(t, balance, false)
			var calls atomic.Int32
			content := `[{"type":"text","text":"ok"}]`
			counter := fmt.Sprintf(`,"server_tool_use":{"%s_requests":1}`, tc.alias)
			if tc.source == "block" {
				content = fmt.Sprintf(`[{"type":"server_tool_use","id":"owned_alias","name":%q,"input":{}},{"type":"text","text":"ok"}]`, tc.alias)
				counter = ""
			}
			response := fmt.Sprintf(`{"id":"alias_receipt","type":"message","role":"assistant","model":"deepseek-v4-pro","content":%s,"stop_reason":"end_turn","usage":{"input_tokens":12,"cache_read_input_tokens":4,"output_tokens":3%s}}`, content, counter)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				if _, err := io.WriteString(w, response); err != nil {
					t.Errorf("fixture response: %v", err)
				}
			}))
			t.Cleanup(server.Close)
			target, err := url.Parse(server.URL)
			require.NoError(t, err)
			previous := client.HTTPClient
			client.HTTPClient = &http.Client{Transport: deepSeekReceiptTransport{next: server.Client().Transport, target: target}}
			t.Cleanup(func() { client.HTTPClient.CloseIdleConnections(); client.HTTPClient = previous })
			tools := ""
			if tc.local {
				tools = `,"tools":[{"name":"web_search","input_schema":{"type":"object"}}]`
			}
			body := `{"model":"alias","max_tokens":16,"messages":[{"role":"user","content":"synthetic"}]` + tools + "}"
			c, w, id := protocolContext(t, channeltype.Anthropic, "deepseek-v4-pro", "/v1/messages", body, tc.base, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 2, CachedInputRatio: 0.5})
			raw, _ := c.Get(ctxkey.ChannelModel)
			require.NoError(t, raw.(*model.Channel).SetToolingConfig(&model.ChannelToolingConfig{Pricing: map[string]model.ToolPricingLocal{"web_search": {QuotaPerCall: 17}, "web_fetch": {QuotaPerCall: 23}}}))
			require.Nil(t, RelayClaudeMessagesHelper(c))
			require.EqualValues(t, 1, calls.Load())
			require.Equal(t, response, w.Body.String())
			drainCriticalTasks(t)
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			var logs []model.Log
			require.NoError(t, model.LOG_DB.Where("request_id = ?", id).Find(&logs).Error)
			require.Len(t, logs, 1)
			gotUser := balance - reloadUserQuota(t)
			gotCost := requestCostQuota(t, id)
			require.Equal(t, tc.want, gotUser, "undocumented DeepSeek aliases must not create paid tool charges")
			require.Equal(t, tc.want, token.UsedQuota)
			require.Equal(t, tc.want, gotCost)
			require.EqualValues(t, tc.want, logs[0].Quota)
		})
	}
}
