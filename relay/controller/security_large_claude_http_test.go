package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestSecurityLargeClaudeHTTPAdmission quotes the actual mapped request before the real HTTP dispatch and quota ledger.
func TestSecurityLargeClaudeHTTPAdmission(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, large := range []bool{false, true} {
			t.Run(fmt.Sprintf("native=%v/large=%v", native, large), func(t *testing.T) {
				const balance = int64(350000)
				xaiVideoSetup(t, balance, false)
				channel, actual := channeltype.OpenAI, "claude-sonnet-4"
				if native {
					channel = channeltype.Anthropic
				}
				text := "hello"
				if large {
					text = strings.Repeat("😀", 280000)
				}
				encoded, err := json.Marshal(text)
				require.NoError(t, err)
				body := `{"model":"alias","max_tokens":1,"messages":[{"role":"user","content":` + string(encoded) + `}]}`
				var calls atomic.Int32
				observed := make(chan map[string]any, 1)
				upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					var request map[string]any
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						http.Error(w, "invalid fixture body", 400)
						return
					}
					observed <- request
					w.Header().Set("Content-Type", "application/json")
					if native {
						_, _ = io.WriteString(w, `{"id":"synthetic-msg","type":"message","role":"assistant","model":"claude-sonnet-4-20250514","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
					} else {
						_, _ = io.WriteString(w, `{"id":"synthetic-chat","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
					}
				}))
				defer upstream.Close()
				oldClient := client.HTTPClient
				client.HTTPClient = upstream.Client()
				defer func() { client.HTTPClient = oldClient }()
				c, _, id := protocolContext(t, channel, actual, "/v1/messages", body, upstream.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
				c.Request.Header.Set("Content-Type", "application/json")
				apiErr := RelayClaudeMessagesHelper(c)
				drainCriticalTasks(t)
				if large {
					if apiErr == nil && calls.Load() == 1 {
						t.Log("REPRODUCED_481_UNDERFUNDED_PROVIDER_DISPATCH")
					}
					require.NotNil(t, apiErr, "underfunded canonical prompt reached the provider")
					require.Equal(t, http.StatusForbidden, apiErr.StatusCode)
					require.Zero(t, calls.Load())
					require.Equal(t, balance, reloadUserQuota(t))
					var token model.Token
					require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
					require.Equal(t, balance, token.RemainQuota)
					var costRows int64
					require.NoError(t, model.DB.Model(&model.UserRequestCost{}).Where("request_id = ?", id).Count(&costRows).Error)
					require.Zero(t, costRows)
				} else {
					require.Nil(t, apiErr)
					require.EqualValues(t, 1, calls.Load())
					require.Equal(t, actual, (<-observed)["model"])
					require.EqualValues(t, 2, requestCostQuota(t, id))
					require.EqualValues(t, balance-2, reloadUserQuota(t))
				}
			})
		}
	}
}
