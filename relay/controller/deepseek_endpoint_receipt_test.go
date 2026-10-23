package controller

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/Laisky/one-api/relay/tooling"
)

// TestDeepSeekToolReceiptEndpointHTTP verifies conservative paid-tool charging
// against actual endpoint overrides while preserving free and unrelated ledgers.
func TestDeepSeekToolReceiptEndpointHTTP(t *testing.T) {
	const response = `{"id":"override_receipt","object":"response","status":"completed","vendor_extension":{"precise":9007199254740993},"output":[{"id":"ws1","type":"web_search_call","status":"completed"}],"usage":{"input_tokens":12,"output_tokens":3,"total_tokens":15,"input_tokens_details":{"cached_tokens":4}}}`
	for _, tc := range []struct {
		name, base, override string
		price                *model.ToolPricingLocal
		local, reject        bool
		charge               int64
	}{
		{"paid_quota", "https://api.openai.com", "https://api.deepseek.com/v1/responses", &model.ToolPricingLocal{QuotaPerCall: 17}, false, false, 186},
		{"paid_overflow_usd", "https://api.openai.com", "https://api.deepseek.com/v1/responses", &model.ToolPricingLocal{UsdPerCall: 1e100}, false, true, 0},
		{"inherited_default", "https://api.openai.com", "https://api.deepseek.com/v1/responses", nil, false, false, -1},
		{"intentional_zero", "https://api.openai.com", "https://api.deepseek.com/v1/responses", &model.ToolPricingLocal{}, false, false, 16},
		{"local_function", "https://api.openai.com", "https://api.deepseek.com/v1/responses", &model.ToolPricingLocal{QuotaPerCall: 17}, true, false, 16},
		{"reverse_base", "https://api.deepseek.com", "https://api.openai.com/v1/responses", &model.ToolPricingLocal{QuotaPerCall: 17}, false, false, 33},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance int64 = 1000000
			xaiVideoSetup(t, balance, false)
			var calls atomic.Int32
			observed := make(chan string, 1)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				body, err := io.ReadAll(r.Body)
				if err != nil {
					observed <- "fixture_read_error"
				} else {
					observed <- string(body)
				}
				w.Header().Set("Content-Type", "application/json")
				if _, err := io.WriteString(w, response); err != nil {
					t.Errorf("write synthetic provider response: %v", err)
				}
			}))
			t.Cleanup(server.Close)
			target, err := url.Parse(server.URL)
			require.NoError(t, err)
			previous := client.HTTPClient
			client.HTTPClient = &http.Client{Transport: deepSeekReceiptTransport{next: server.Client().Transport, target: target}}
			t.Cleanup(func() { client.HTTPClient.CloseIdleConnections(); client.HTTPClient = previous })
			tool := `{"type":"web_search"}`
			if tc.local {
				tool = `{"type":"function","name":"web_search","parameters":{"type":"object"}}`
			}
			c, w, id := protocolContext(t, channeltype.OpenAI, "deepseek-v4-pro", "/v1/responses",
				`{"model":"alias","input":"synthetic","tools":[`+tool+`],"tool_choice":"auto"}`,
				tc.base, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 2, CachedInputRatio: 0.5})
			c.Set(ctxkey.Config, model.ChannelConfig{EndpointURLs: map[string]string{channeltype.RelayModeToEndpointName(relaymode.ResponseAPI): tc.override}})
			if tc.price != nil {
				raw, _ := c.Get(ctxkey.ChannelModel)
				require.NoError(t, raw.(*model.Channel).SetToolingConfig(&model.ChannelToolingConfig{Pricing: map[string]model.ToolPricingLocal{"web_search": *tc.price}}))
			}
			apiErr := RelayResponseAPIHelper(c)
			if tc.reject {
				require.NotNil(t, apiErr)
				require.NotEqual(t, "tool_billing_not_supported", apiErr.Error.Code)
				require.Zero(t, calls.Load(), "unfundable tariff must not dispatch")
				require.Zero(t, c.GetInt64(ctxkey.PreConsumedQuotaAmount))
			} else {
				require.Nil(t, apiErr)
				require.EqualValues(t, 1, calls.Load())
				require.Contains(t, <-observed, `"type":`)
				require.Equal(t, response, w.Body.String())
				require.Equal(t, tc.override, metalib.GetByContext(c).UpstreamRequestURL)
			}
			drainCriticalTasks(t)
			expected := tc.charge
			if expected < 0 {
				expected = 16 + tooling.ConservativeDeepSeekToolQuota(c)
				require.Greater(t, expected, int64(16), "provider default tool price must be funded")
			}
			require.Equal(t, balance-expected, reloadUserQuota(t))
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			require.Equal(t, balance-expected, token.RemainQuota)
			require.Equal(t, expected, token.UsedQuota)
			var costs []model.UserRequestCost
			require.NoError(t, model.DB.Where("request_id = ?", id).Find(&costs).Error)
			var logs []model.Log
			require.NoError(t, model.LOG_DB.Where("request_id = ?", id).Find(&logs).Error)
			if tc.reject {
				require.Empty(t, costs)
				require.Empty(t, logs)
			} else {
				require.Len(t, costs, 1)
				require.Equal(t, expected, requestCostQuota(t, id))
				var consume, tools []model.Log
				for _, entry := range logs {
					switch entry.Type {
					case model.LogTypeConsume:
						consume = append(consume, entry)
					case model.LogTypeTool:
						tools = append(tools, entry)
					default:
						t.Fatalf("unexpected ledger row type %d", entry.Type)
					}
				}
				require.Len(t, consume, 1)
				require.EqualValues(t, expected, consume[0].Quota)
				require.Equal(t, 12, consume[0].PromptTokens)
				require.Equal(t, 3, consume[0].CompletionTokens)
				require.Equal(t, 4, consume[0].CachedPromptTokens)
				require.Equal(t, 1, c.GetInt(ctxkey.WebSearchCallCount), "provider output is observed")
				if expected == 16 {
					require.Len(t, logs, 1)
					require.Empty(t, tools)
				} else {
					toolFloor := tooling.ConservativeDeepSeekToolQuota(c)
					count := 1
					if toolFloor > 0 {
						count = 10
					}
					require.Len(t, logs, 1+count)
					require.Len(t, tools, count)
					var sum int64
					for _, tool := range tools {
						require.Equal(t, "web_search", tool.ModelName)
						sum += int64(tool.Quota)
					}
					require.Equal(t, expected-16, sum, "tool log rows must reconcile to the total quoted tool charge")
				}
			}
		})
	}
}

// TestDeepSeekToolReceiptRetryEndpointSwitch retains stale observed URLs while
// refreshing the actual dispatch Meta.Config and clearing per-attempt counts.
func TestDeepSeekToolReceiptRetryEndpointSwitch(t *testing.T) {
	for index, endpoint := range []string{"https://api.deepseek.com/v1/responses", "https://api.openai.com/v1/responses", "https://tools.deepseek.com/responses"} {
		t.Run(endpoint, func(t *testing.T) {
			xaiVideoSetup(t, 1000000, false)
			c, _, _ := protocolContext(t, channeltype.OpenAI, "deepseek-v4-pro", "/v1/responses",
				`{"model":"alias","input":"synthetic"}`, "https://api.openai.com", 1000000, 1, false, nil)
			current := metalib.GetByContext(c)
			current.Mode = relaymode.ResponseAPI
			stale := "https://api.openai.com/v1/responses"
			if index == 1 {
				stale = "https://api.deepseek.com/v1/responses"
			}
			current.UpstreamRequestURL = stale
			c.Set(ctxkey.WebSearchCallCount, 9)
			c.Set(ctxkey.ToolInvocationCounts, map[string]int{"web_search": 9})
			require.True(t, BillingAllowsRetry(c), "synthetic abandoned attempt has no hold or delivered response")
			ResetPerAttemptBillingForRetry(context.Background(), c)
			require.Zero(t, c.GetInt(ctxkey.WebSearchCallCount))
			c.Set(ctxkey.ChannelId, fallbackChannelID+index+1)
			c.Set(ctxkey.Config, model.ChannelConfig{EndpointURLs: map[string]string{channeltype.RelayModeToEndpointName(relaymode.ResponseAPI): endpoint}})
			refreshed := metalib.GetByContext(c)
			require.Same(t, current, refreshed)
			require.Equal(t, endpoint, refreshed.UpstreamEndpointURLOverride())
			require.Equal(t, stale, refreshed.UpstreamRequestURL, "the existing Meta refresh intentionally retains observed URL")
			response := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"retry_override","object":"response","status":"completed","output":[{"id":"ws","type":"web_search_call"}],"usage":{"input_tokens":12,"output_tokens":3,"total_tokens":15}}`))}
			apiErr, usage := openai.ResponseAPIDirectHandler(c, response, 12, refreshed.ActualModelName)
			require.Nil(t, apiErr)
			require.Equal(t, 12, usage.PromptTokens)
			require.Equal(t, 3, usage.CompletionTokens)
			require.Equal(t, 1, c.GetInt(ctxkey.WebSearchCallCount), "all provider outputs remain observable; billing uses the selected target")
		})
	}
}
