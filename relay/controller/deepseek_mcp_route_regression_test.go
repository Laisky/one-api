package controller

import (
	"encoding/json"
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
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/stretchr/testify/require"
)

// TestDeepSeekClaudeMCPEndpointAdmission binds raw and converted tool admission
// to the endpoint used by the actual MCP Chat round, not the client-facing
// Claude Messages endpoint. Both mismatch directions are regression cases.
func TestDeepSeekClaudeMCPEndpointAdmission(t *testing.T) {
	for _, tc := range []struct {
		name, messages, chat string
		reject               bool
	}{
		{"control_both_deepseek", "https://api.deepseek.com/v1/responses", "https://api.deepseek.com/v1/responses", true},
		{"control_both_unrelated", "https://api.openai.com/v1/responses", "https://api.openai.com/v1/responses", false},
		{"mcp_switches_to_deepseek", "https://api.openai.com/v1/responses", "https://api.deepseek.com/v1/responses", true},
		{"mcp_switches_to_unrelated", "https://api.deepseek.com/v1/responses", "https://api.openai.com/v1/responses", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance int64 = 1000000
			xaiVideoSetup(t, balance, false)
			stored := &model.MCPServer{Name: "pr589-" + tc.name, Status: model.MCPServerStatusEnabled, BaseURL: "https://example.com/unused-mcp", ToolWhitelist: model.JSONStringSlice{"review_lookup"}}
			require.NoError(t, model.DB.Create(stored).Error)
			t.Cleanup(func() {
				require.NoError(t, model.DB.Where("server_id = ?", stored.Id).Delete(&model.MCPTool{}).Error)
				require.NoError(t, model.DB.Delete(stored).Error)
			})
			require.NoError(t, model.DB.Create(&model.MCPTool{ServerId: stored.Id, Name: "review_lookup", Description: "Owned test lookup", InputSchema: `{"type":"object","properties":{}}`}).Error)
			var calls atomic.Int32
			observed := make(chan string, 2)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read request: %v", err)
				}
				observed <- string(body)
				w.Header().Set("Content-Type", "application/json")
				if _, err := io.WriteString(w, claudeToolPolicyResponsesJSON); err != nil {
					t.Errorf("write: %v", err)
				}
			}))
			t.Cleanup(server.Close)
			target, err := url.Parse(server.URL)
			require.NoError(t, err)
			previous := client.HTTPClient
			client.HTTPClient = &http.Client{Transport: deepSeekReceiptTransport{next: server.Client().Transport, target: target}}
			t.Cleanup(func() { client.HTTPClient.CloseIdleConnections(); client.HTTPClient = previous })
			body := `{"model":"alias","max_tokens":16,"messages":[{"role":"user","content":"synthetic"}],"tools":[{"type":"web_search","name":"web_search"},{"type":"review_lookup","name":"review_lookup"}]}`
			actual := "deepseek-v4-pro"
			if tc.chat == "https://api.openai.com/v1/responses" {
				actual = "gpt-4o"
			}
			c, _, id := protocolContext(t, channeltype.OpenAI, actual, "/v1/messages", body, "https://api.openai.com", balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 2})
			c.Set(ctxkey.Config, model.ChannelConfig{EndpointURLs: map[string]string{
				channeltype.RelayModeToEndpointName(relaymode.ClaudeMessages):  tc.messages,
				channeltype.RelayModeToEndpointName(relaymode.ChatCompletions): tc.chat,
			}})
			raw, _ := c.Get(ctxkey.ChannelModel)
			require.NoError(t, raw.(*model.Channel).SetToolingConfig(&model.ChannelToolingConfig{Pricing: map[string]model.ToolPricingLocal{"web_search": {QuotaPerCall: 17}, "review_lookup": {}}}))
			apiErr := RelayClaudeMessagesHelper(c)
			drainCriticalTasks(t)
			if calls.Load() > 0 {
				forwarded := <-observed
				var wire map[string]any
				require.NoError(t, json.Unmarshal([]byte(forwarded), &wire))
				require.Equal(t, actual, wire["model"])
				require.Contains(t, wire, "input", "the internal Chat round was converted to a Responses payload")
				require.NotContains(t, wire, "messages")
				require.Contains(t, forwarded, `"type":"web_search"`)
				require.Equal(t, tc.chat, metalib.GetByContext(c).UpstreamRequestURL)
			}
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			if tc.reject {
				require.Zero(t, calls.Load(), "paid DeepSeek tool must not reach upstream")
				require.NotNil(t, apiErr)
				require.Equal(t, "tool_billing_not_supported", apiErr.Error.Code)
				require.Zero(t, c.GetInt64(ctxkey.PreConsumedQuotaAmount))
				require.Equal(t, balance, reloadUserQuota(t))
				require.Equal(t, balance, token.RemainQuota)
				require.Zero(t, token.UsedQuota)
				var costs, logs int64
				require.NoError(t, model.DB.Model(&model.UserRequestCost{}).Where("request_id = ?", id).Count(&costs).Error)
				require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("request_id = ?", id).Count(&logs).Error)
				require.Zero(t, costs)
				require.Zero(t, logs)
			} else {
				require.EqualValues(t, 1, calls.Load())
				require.Nil(t, apiErr)
				require.Less(t, reloadUserQuota(t), balance)
				require.Greater(t, token.UsedQuota, int64(0))
			}
		})
	}
}
