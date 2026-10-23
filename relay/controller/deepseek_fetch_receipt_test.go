package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// deepSeekReceiptTransport routes only the synthetic provider transport to a
// loopback TLS server while retaining the real configured provider identity.
type deepSeekReceiptTransport struct {
	next   http.RoundTripper
	target *url.URL
}

// RoundTrip forwards the original path/body to the owned loopback fixture.
func (t deepSeekReceiptTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	cloned := request.Clone(request.Context())
	target := *request.URL
	target.Scheme = t.target.Scheme
	target.Host = t.target.Host
	cloned.URL = &target
	response, err := t.next.RoundTrip(cloned)
	if err != nil {
		return nil, errors.Wrap(err, "synthetic provider transport")
	}
	return response, nil
}

// TestDeepSeekToolReceiptClaudeFetchHTTP reproduces native Claude Fetch
// admission/counting under actual DeepSeek identity, and preserves free/local
// and unrelated-provider token/cache/physical-ledger behavior.
func TestDeepSeekToolReceiptClaudeFetchHTTP(t *testing.T) {
	const response = `{"id":"fetch_receipt","type":"message","role":"assistant","model":"deepseek-v4-pro","content":[{"type":"server_tool_use","id":"fetch1","name":"web_fetch","input":{}},{"type":"text","text":"ok"}],"stop_reason":"end_turn","vendor_extension":{"precise":9007199254740993},"usage":{"input_tokens":12,"cache_read_input_tokens":4,"output_tokens":3,"server_tool_use":{"web_fetch_requests":1}}}`
	for _, tc := range []struct {
		name, base    string
		price         model.ToolPricingLocal
		local         bool
		charge        int64
	}{
		{"quota_paid", "https://api.deepseek.com/anthropic", model.ToolPricingLocal{QuotaPerCall: 23}, false, 246},
		{"usd_paid", "https://api.deepseek.com/anthropic", model.ToolPricingLocal{UsdPerCall: 0.01}, false, 16 + 10*ratio.QuotaPerUsd/100},
		{"intentional_zero", "https://api.deepseek.com/anthropic", model.ToolPricingLocal{}, false, 16},
		{"local_function_collision", "https://api.deepseek.com/anthropic", model.ToolPricingLocal{QuotaPerCall: 23}, true, 16},
		{"unrelated_anthropic", "https://api.anthropic.com", model.ToolPricingLocal{QuotaPerCall: 23}, false, 39},
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
			tool := `{"type":"web_fetch_20250910","name":"web_fetch","max_uses":1}`
			if tc.local {
				tool = `{"name":"web_fetch","input_schema":{"type":"object"}}`
			}
			body := `{"model":"alias","max_tokens":16,"messages":[{"role":"user","content":"synthetic"}],"tools":[` + tool + `]}`
			c, w, id := protocolContext(t, channeltype.Anthropic, "deepseek-v4-pro", "/v1/messages", body, tc.base, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 2, CachedInputRatio: 0.5})
			raw, _ := c.Get(ctxkey.ChannelModel)
			require.NoError(t, raw.(*model.Channel).SetToolingConfig(&model.ChannelToolingConfig{Pricing: map[string]model.ToolPricingLocal{"web_fetch": tc.price}}))
			require.Nil(t, RelayClaudeMessagesHelper(c))
			require.EqualValues(t, 1, calls.Load(), "priced native Fetch must reach the provider")
			require.Contains(t, <-observed, `"name":"web_fetch"`)
			require.Equal(t, response, w.Body.String(), "native wire extensions remain byte-exact")
			drainCriticalTasks(t)
			require.Equal(t, balance-tc.charge, reloadUserQuota(t))
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			require.Equal(t, balance-tc.charge, token.RemainQuota)
			require.Equal(t, tc.charge, token.UsedQuota)
			var logs []model.Log
			require.NoError(t, model.LOG_DB.Where("request_id = ?", id).Find(&logs).Error)
			require.Equal(t, tc.charge, requestCostQuota(t, id))
			require.Len(t, logs, 1, "native Claude incorporates its tool cost into the consume receipt")
			require.EqualValues(t, tc.charge, logs[0].Quota)
			require.Equal(t, model.LogTypeConsume, logs[0].Type)
			if tc.charge > 16 {
				require.Contains(t, logs[0].Content, "tools cost")
			} else {
				require.NotContains(t, logs[0].Content, "tools cost")
			}
		})
	}
}
