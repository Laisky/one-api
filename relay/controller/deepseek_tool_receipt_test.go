package controller

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestDeepSeekToolReceiptAdmission ensures positively priced native DeepSeek
// search stays available and reserves a conservative minimum tool charge.
// Legacy deepseek-chat uses the Chat fallback and is covered by the
// separate Chat endpoint regression with its own provider response format.
func TestDeepSeekToolReceiptAdmission(t *testing.T) {
	for _, actual := range []string{"deepseek-v4-pro", "deepseek-flash"} {
		for _, choice := range []string{`"auto"`, `{"type":"web_search"}`} {
			for _, tariff := range []string{"quota", "usd"} {
				t.Run(actual+"/"+choice+"/"+tariff, func(t *testing.T) {
					const balance int64 = 1000000
					xaiVideoSetup(t, balance, false)
					var calls atomic.Int32
					server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, `{"id":"priced","object":"response","status":"completed","output":[{"id":"ws1","type":"web_search_call"}],"usage":{"input_tokens":12,"output_tokens":3,"total_tokens":15,"input_tokens_details":{"cached_tokens":4}}}`)
					}))
					t.Cleanup(server.Close)
					previous := client.HTTPClient
					client.HTTPClient = server.Client()
					t.Cleanup(func() { client.HTTPClient = previous })
					c, _, id := protocolContext(t, channeltype.DeepSeek, actual, "/v1/responses",
						`{"model":"alias","input":"synthetic","tools":[{"type":"web_search"}],"tool_choice":`+choice+`}`,
						server.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 2, CachedInputRatio: 0.5})
					raw, ok := c.Get(ctxkey.ChannelModel)
					require.True(t, ok)
					channel := raw.(*model.Channel)
					price := model.ToolPricingLocal{QuotaPerCall: 17}
					if tariff == "usd" {
						price = model.ToolPricingLocal{UsdPerCall: 0.01}
					}
					require.NoError(t, channel.SetToolingConfig(&model.ChannelToolingConfig{Pricing: map[string]model.ToolPricingLocal{"web_search": price}}))
					require.Nil(t, RelayResponseAPIHelper(c))
					drainCriticalTasks(t)
					require.EqualValues(t, 1, calls.Load(), "priced search must reach the upstream")
					perCall := int64(17)
					if tariff == "usd" {
						perCall = int64(0.01 * float64(ratio.QuotaPerUsd))
					}
					expected := int64(16) + 10*perCall
					require.Equal(t, expected, balance-reloadUserQuota(t))
					require.Equal(t, expected, requestCostQuota(t, id))
					var token model.Token
					require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
					require.Equal(t, expected, token.UsedQuota)
					var logs []model.Log
					require.NoError(t, model.LOG_DB.Where("request_id = ?", id).Find(&logs).Error)
					require.NotEmpty(t, logs)
					var consume, tools int
					for _, entry := range logs {
						switch entry.Type {
						case model.LogTypeConsume:
							consume++
							require.EqualValues(t, expected, entry.Quota)
						case model.LogTypeTool:
							tools++
							require.EqualValues(t, perCall, entry.Quota)
						}
					}
					require.Equal(t, 1, consume)
					require.Equal(t, 10, tools, "tool usage is recorded once for each estimated invocation")
				})
			}
		}
	}
}

// TestDeepSeekToolReceiptFreeAndLocalControls preserves explicit zero tariffs,
// ordinary default auto-pruning, client tools and exact token/cache settlement.
func TestDeepSeekToolReceiptFreeAndLocalControls(t *testing.T) {
	for _, tc := range []struct {
		name, actual, tools string
		free                bool
		wantTools           int
	}{
		{"zero_pro", "deepseek-v4-pro", `[{"type":"web_search"}]`, true, 1},
		{"zero_flash", "deepseek-flash", `[{"type":"web_search"}]`, true, 1},
		{"default_auto_pro", "deepseek-v4-pro", `[{"type":"web_search"}]`, false, 0},
		{"default_auto_flash", "deepseek-flash", `[{"type":"web_search"}]`, false, 0},
		{"function_name_collision", "deepseek-v4-pro", `[{"type":"function","name":"web_search","parameters":{"type":"object"}}]`, false, 1},
		{"client_tool_search", "deepseek-v4-pro", `[{"type":"tool_search","execution":"client"}]`, false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance int64 = 1000000
			xaiVideoSetup(t, balance, false)
			observed := make(chan map[string]any, 1)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					body = map[string]any{"decode_error": err.Error()}
				}
				observed <- body
				w.Header().Set("Content-Type", "application/json")
				if _, err := io.WriteString(w, `{"id":"free_receipt","object":"response","status":"completed","output":[{"id":"ws1","type":"web_search_call","status":"completed"}],"usage":{"input_tokens":12,"output_tokens":3,"total_tokens":15,"input_tokens_details":{"cached_tokens":4}}}`); err != nil {
					t.Errorf("write synthetic provider response: %v", err)
				}
			}))
			t.Cleanup(server.Close)
			previous := client.HTTPClient
			client.HTTPClient = server.Client()
			t.Cleanup(func() { client.HTTPClient = previous })
			c, _, id := protocolContext(t, channeltype.DeepSeek, tc.actual, "/v1/responses",
				`{"model":"alias","input":"synthetic","tools":`+tc.tools+`,"tool_choice":"auto"}`,
				server.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 2, CachedInputRatio: 0.5})
			if tc.free {
				raw, _ := c.Get(ctxkey.ChannelModel)
				require.NoError(t, raw.(*model.Channel).SetToolingConfig(&model.ChannelToolingConfig{Pricing: map[string]model.ToolPricingLocal{"web_search": {}}}))
			}
			require.Nil(t, RelayResponseAPIHelper(c))
			drainCriticalTasks(t)
			body := <-observed
			require.NotContains(t, body, "decode_error")
			require.Equal(t, tc.actual, body["model"])
			wireTools, _ := body["tools"].([]any)
			require.Len(t, wireTools, tc.wantTools)
			// Eight uncached input + four cached at half rate + three output at two.
			require.Equal(t, balance-16, reloadUserQuota(t))
			require.EqualValues(t, 16, requestCostQuota(t, id))
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			require.Equal(t, balance-16, token.RemainQuota)
			require.EqualValues(t, 16, token.UsedQuota)
			var logs []model.Log
			require.NoError(t, model.LOG_DB.Where("request_id = ?", id).Find(&logs).Error)
			require.Len(t, logs, 1)
			require.Equal(t, 12, logs[0].PromptTokens)
			require.Equal(t, 3, logs[0].CompletionTokens)
			require.Equal(t, 4, logs[0].CachedPromptTokens)
			require.Equal(t, 16, logs[0].Quota)
			require.Equal(t, model.LogTypeConsume, logs[0].Type)
			require.Equal(t, 1, c.GetInt(ctxkey.WebSearchCallCount), "observing a wire event does not price an unadmitted tool")
		})
	}
}

// TestDeepSeekToolReceiptOtherProtocols verifies paid tools dispatch through
// Chat and Claude while definitive upstream errors retain existing refunds.
func TestDeepSeekToolReceiptOtherProtocols(t *testing.T) {
	for _, tc := range []struct{ name, path, body string }{
		{"chat", "/v1/chat/completions", `{"model":"alias","messages":[{"role":"user","content":"synthetic"}],"tools":[{"type":"web_search"}],"tool_choice":"auto"}`},
		{"claude", "/v1/messages", `{"model":"alias","max_tokens":16,"messages":[{"role":"user","content":"synthetic"}],"tools":[{"type":"web_search_20250305","name":"web_search"}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance int64 = 1000000
			xaiVideoSetup(t, balance, false)
			var calls atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(503) }))
			t.Cleanup(server.Close)
			previous := client.HTTPClient
			client.HTTPClient = server.Client()
			t.Cleanup(func() { client.HTTPClient = previous })
			c, _, id := protocolContext(t, channeltype.DeepSeek, "deepseek-v4-pro", tc.path, tc.body, server.URL, balance, 1, false, nil)
			raw, _ := c.Get(ctxkey.ChannelModel)
			require.NoError(t, raw.(*model.Channel).SetToolingConfig(&model.ChannelToolingConfig{Pricing: map[string]model.ToolPricingLocal{"web_search": {QuotaPerCall: 17}}}))
			var apiErr *relaymodel.ErrorWithStatusCode
			if tc.name == "claude" {
				apiErr = RelayClaudeMessagesHelper(c)
			} else {
				apiErr = RelayTextHelper(c)
			}
			require.NotNil(t, apiErr, "the owned upstream intentionally returns 503")
			require.NotEqual(t, "tool_billing_not_supported", apiErr.Error.Code)
			drainCriticalTasks(t)
			require.EqualValues(t, 1, calls.Load(), "priced Chat and Claude tool requests must dispatch")
			charge := balance - reloadUserQuota(t)
			require.GreaterOrEqual(t, charge, int64(170), "forwarded work retains the conservative tool floor on a 503")
			require.Equal(t, charge, requestCostQuota(t, id), "retained user debits must remain visible in request costs")
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			require.Equal(t, charge, token.UsedQuota)
		})
	}
}

// TestDeepSeekToolReceiptRetryProviderSwitch checks the real per-attempt reset
// and cached-metadata refresh used after selecting a different retry channel.
// The abandoned state is synthetic, has no hold, and performs no paid work.
func TestDeepSeekToolReceiptRetryProviderSwitch(t *testing.T) {
	for index, channel := range []int{channeltype.DeepSeek, channeltype.OpenAI} {
		c, _, _ := protocolContext(t, channeltype.OpenAI, "deepseek-v4-pro", "/v1/responses",
			`{"model":"alias","input":"synthetic"}`, "https://api.openai.com", 1000000, 1, false, nil)
		current := metalib.GetByContext(c)
		c.Set(ctxkey.WebSearchCallCount, 9)
		c.Set(ctxkey.ToolInvocationCounts, map[string]int{"web_search": 9, "web_fetch": 2})
		c.Set(ctxkey.ToolInvocationSummary, &model.ToolUsageSummary{Counts: map[string]int{"web_search": 9}})

		require.True(t, BillingAllowsRetry(c), "only an abandoned attempt may reset its tool state")
		ResetPerAttemptBillingForRetry(context.Background(), c)
		require.Zero(t, c.GetInt(ctxkey.WebSearchCallCount))
		raw, exists := c.Get(ctxkey.ToolInvocationCounts)
		require.True(t, exists)
		require.Empty(t, raw)
		raw, _ = c.Get(ctxkey.ToolInvocationSummary)
		require.Nil(t, raw.(*model.ToolUsageSummary))
		c.Set(ctxkey.ChannelId, fallbackChannelID+index+1)
		c.Set(ctxkey.Channel, channel)
		base := "https://api.openai.com"
		if channel == channeltype.DeepSeek {
			base = "https://api.deepseek.com"
		}
		c.Set(ctxkey.BaseURL, base)
		refreshed := metalib.GetByContext(c)
		require.Same(t, current, refreshed, "the refreshed cached object is the dispatch metadata")
		require.Equal(t, channel, refreshed.ChannelType)
		require.Equal(t, base, refreshed.BaseURL)
		require.Equal(t, "deepseek-v4-pro", refreshed.ActualModelName)
		body := `{"id":"retry","object":"response","status":"completed","output":[{"id":"ws","type":"web_search_call"}],"usage":{"input_tokens":12,"output_tokens":3,"total_tokens":15}}`
		response := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
		apiErr, usage := openai.ResponseAPIDirectHandler(c, response, 12, refreshed.ActualModelName)
		require.Nil(t, apiErr)
		require.Equal(t, 12, usage.PromptTokens)
		require.Equal(t, 3, usage.CompletionTokens)
		require.Equal(t, 1, c.GetInt(ctxkey.WebSearchCallCount), "wire observations remain available after every retry")
		require.False(t, BillingAllowsRetry(c), "completed responses are not retryable")
	}
}
