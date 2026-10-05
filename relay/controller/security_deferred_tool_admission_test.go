package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/stretchr/testify/require"
)

// deferredAdmissionTool returns a synthetic caller tool with a large schema and optional deferral metadata.
func deferredAdmissionTool(flag string) map[string]any {
	tool := map[string]any{
		"name":        "lookup_record",
		"description": "Look up a synthetic record.",
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": strings.Repeat("synthetic schema content ", 4096),
				},
			},
		},
	}
	if flag != "absent" {
		tool["defer_loading"] = flag != "false"
	}
	if flag == "spoofed" {
		// Neither a wire spelling nor Go's exported field spelling may grant trust.
		tool["trusted_deferred_loading"] = true
		tool["TrustedDeferredLoading"] = true
	}
	return tool
}

// TestSecurityDeferredToolCallerQuote verifies that deserialized caller metadata cannot lower schema admission.
func TestSecurityDeferredToolCallerQuote(t *testing.T) {
	var ordinary relaymodel.ClaudeTool
	encoded, err := json.Marshal(deferredAdmissionTool("absent"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(encoded, &ordinary))
	want := countClaudeToolsTokens(context.Background(), []relaymodel.ClaudeTool{ordinary}, "claude-sonnet-4")
	require.Greater(t, want, 1000, "the fixture must exceed the finite admission budget")
	for _, flag := range []string{"absent", "false", "true", "spoofed"} {
		t.Run(flag, func(t *testing.T) {
			body, marshalErr := json.Marshal(deferredAdmissionTool(flag))
			require.NoError(t, marshalErr)
			var callerTool relaymodel.ClaudeTool
			require.NoError(t, json.Unmarshal(body, &callerTool))
			got := countClaudeToolsTokens(context.Background(), []relaymodel.ClaudeTool{callerTool}, "claude-sonnet-4")
			require.Equal(t, want, got, "a caller-controlled loading hint is not billing authority")
		})
	}
}

// TestSecurityDeferredToolHTTPAdmission checks actual upstream dispatch, payload preservation, and physical quota settlement.
func TestSecurityDeferredToolHTTPAdmission(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, funded := range []bool{false, true} {
			for _, flag := range []string{"absent", "false", "true", "spoofed"} {
				t.Run(fmt.Sprintf("native=%v/funded=%v/flag=%s", native, funded, flag), func(t *testing.T) {
					balance := int64(1000)
					if funded {
						balance = 100000
					}
					xaiVideoSetup(t, balance, false)
					channel := channeltype.OpenAI
					if native {
						channel = channeltype.Anthropic
					}
					tool := deferredAdmissionTool(flag)
					payload, err := json.Marshal(map[string]any{
						"model":      "alias",
						"max_tokens": 64,
						"messages":   []any{map[string]any{"role": "user", "content": "hello"}},
						"tools":      []any{tool},
					})
					require.NoError(t, err)
					var calls atomic.Int32
					observed := make(chan map[string]any, 1)
					upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						var request map[string]any
						if decodeErr := json.NewDecoder(r.Body).Decode(&request); decodeErr != nil {
							http.Error(w, "invalid synthetic request", http.StatusBadRequest)
							return
						}
						select {
						case observed <- request:
						default:
							// The call counter detects duplicates without blocking cleanup.
						}
						w.Header().Set("Content-Type", "application/json")
						if native {
							_, _ = io.WriteString(w, `{"id":"synthetic-message","type":"message","role":"assistant","model":"claude-sonnet-4","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":11,"output_tokens":7}}`)
							return
						}
						_, _ = io.WriteString(w, `{"id":"synthetic-chat","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`)
					}))
					defer upstream.Close()
					previous := client.HTTPClient
					client.HTTPClient = upstream.Client()
					defer func() { client.HTTPClient = previous }()
					c, _, id := protocolContext(t, channel, "claude-sonnet-4", "/v1/messages", string(payload), upstream.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
					c.Request.Header.Set("Content-Type", "application/json")
					apiErr := RelayClaudeMessagesHelper(c)
					drainCriticalTasks(t)
					var token model.Token
					require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
					if !funded {
						if apiErr == nil && calls.Load() != 0 {
							t.Log("REPRODUCED_463_UNDERFUNDED_DEFERRED_SCHEMA_DISPATCH")
						}
						require.NotNil(t, apiErr, "underfunded schema must be rejected before dispatch")
						require.Equal(t, http.StatusForbidden, apiErr.StatusCode)
						require.Zero(t, calls.Load())
						require.Equal(t, balance, reloadUserQuota(t))
						require.Equal(t, balance, token.RemainQuota)
						require.Zero(t, token.UsedQuota)
						var rows int64
						require.NoError(t, model.DB.Model(&model.UserRequestCost{}).Where("request_id = ?", id).Count(&rows).Error)
						require.Zero(t, rows)
						return
					}
					require.Nil(t, apiErr)
					require.EqualValues(t, 1, calls.Load())
					require.EqualValues(t, 18, requestCostQuota(t, id))
					require.Equal(t, balance-18, reloadUserQuota(t))
					require.Equal(t, balance-18, token.RemainQuota)
					require.EqualValues(t, 18, token.UsedQuota)
					var request map[string]any
					select {
					case request = <-observed:
					case <-time.After(5 * time.Second):
						t.Fatal("the funded control did not reach the upstream fixture")
					}
					require.Equal(t, "claude-sonnet-4", request["model"])
					tools, ok := request["tools"].([]any)
					require.True(t, ok)
					require.Len(t, tools, 1)
					wireTool, ok := tools[0].(map[string]any)
					require.True(t, ok)
					if native {
						require.Equal(t, tool["input_schema"], wireTool["input_schema"])
						if flag != "absent" {
							require.Equal(t, tool["defer_loading"], wireTool["defer_loading"], "preserve provider loading semantics")
						}
					} else {
						function, isFunction := wireTool["function"].(map[string]any)
						require.True(t, isFunction)
						require.Equal(t, tool["input_schema"], function["parameters"], "conversion sends the full schema")
					}
				})
			}
		}
	}
}
