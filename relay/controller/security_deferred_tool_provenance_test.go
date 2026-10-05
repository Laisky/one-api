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

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestSecurityDeferredToolProvenance keeps trust out of caller JSON, including
// decoded reuse, while preserving native loading hints and explicit activation.
func TestSecurityDeferredToolProvenance(t *testing.T) {
	flag := true
	tool := relaymodel.ClaudeTool{
		Name: "catalog_lookup", InputSchema: map[string]any{"type": "object"},
		DeferLoading: &flag, TrustedDeferredLoading: true,
	}
	require.Zero(t, countClaudeToolsTokens(context.Background(), []relaymodel.ClaudeTool{tool}, "claude-sonnet-4"))
	encoded, err := json.Marshal(tool)
	require.NoError(t, err)
	var wire map[string]any
	require.NoError(t, json.Unmarshal(encoded, &wire))
	require.NotContains(t, wire, "TrustedDeferredLoading")
	require.NotContains(t, wire, "trusted_deferred_loading")
	require.Equal(t, true, wire["defer_loading"])
	for _, input := range []string{
		string(encoded),
		`{"name":"caller","defer_loading":true,"TrustedDeferredLoading":true}`,
		`{"name":"caller","defer_loading":true,"trusted_deferred_loading":true}`,
	} {
		reused := tool
		require.NoError(t, json.Unmarshal([]byte(input), &reused))
		require.False(t, reused.TrustedDeferredLoading, "JSON must clear previously trusted provenance")
		require.Greater(t, countClaudeToolsTokens(context.Background(), []relaymodel.ClaudeTool{reused}, "claude-sonnet-4"), 0)
	}
	flag = false
	require.Greater(t, countClaudeToolsTokens(context.Background(), []relaymodel.ClaudeTool{tool}, "claude-sonnet-4"), 0, "activated tools consume context")
	tool.DeferLoading = nil
	require.Greater(t, countClaudeToolsTokens(context.Background(), []relaymodel.ClaudeTool{tool}, "claude-sonnet-4"), 0)
}

// TestSecurityDeferredToolCatalogHTTP verifies actual catalog injection, native
// HTTP dispatch and settlement, non-native admission, and caller-name collisions.
func TestSecurityDeferredToolCatalogHTTP(t *testing.T) {
	for _, tc := range []struct {
		name              string
		native, collision bool
	}{
		{"native_catalog", true, false},
		{"converted_catalog", false, false},
		{"caller_collision", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance = int64(1000)
			xaiVideoSetup(t, balance, false)
			stored := &model.MCPServer{Name: "deferral-" + tc.name, Status: model.MCPServerStatusEnabled, BaseURL: "https://example.com/synthetic-unused-mcp"}
			require.NoError(t, model.DB.Create(stored).Error)
			t.Cleanup(func() { require.NoError(t, model.DB.Delete(stored).Error) })
			callerTool := deferredAdmissionTool("spoofed")
			schema, err := json.Marshal(callerTool["input_schema"])
			require.NoError(t, err)
			storedTool := &model.MCPTool{ServerId: stored.Id, Name: "lookup_record", Description: "Catalog lookup", InputSchema: string(schema)}
			require.NoError(t, model.DB.Create(storedTool).Error)
			t.Cleanup(func() { require.NoError(t, model.DB.Delete(storedTool).Error) })
			tools := []any{map[string]any{"type": "tool_search_tool_regex_20251119", "name": "tool_search_tool_regex"}}
			if tc.collision {
				tools = append(tools, callerTool)
			}
			payload, err := json.Marshal(map[string]any{"model": "alias", "max_tokens": 64, "messages": []any{map[string]any{"role": "user", "content": "hello"}}, "tools": tools})
			require.NoError(t, err)
			var calls atomic.Int32
			observed := make(chan []byte, 1)
			upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				body, readErr := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
				if readErr != nil {
					http.Error(w, "invalid synthetic request", 400)
					return
				}
				select {
				case observed <- body:
				default:
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"synthetic-catalog","type":"message","role":"assistant","model":"claude-sonnet-4","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":11,"output_tokens":7}}`)
			}))
			t.Cleanup(upstream.Close)
			previous := client.HTTPClient
			client.HTTPClient = upstream.Client()
			t.Cleanup(func() { client.HTTPClient = previous })
			channel := channeltype.Anthropic
			if !tc.native {
				channel = channeltype.OpenAI
			}
			c, _, id := protocolContext(t, channel, "claude-sonnet-4", "/v1/messages", string(payload), upstream.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
			apiErr := RelayClaudeMessagesHelper(c)
			drainCriticalTasks(t)
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			if !tc.native || tc.collision {
				require.NotNil(t, apiErr)
				require.Equal(t, http.StatusForbidden, apiErr.StatusCode, fmt.Sprint(apiErr))
				require.Zero(t, calls.Load())
				require.Equal(t, balance, reloadUserQuota(t))
				require.Equal(t, balance, token.RemainQuota)
				require.Zero(t, token.UsedQuota)
				return
			}
			require.Nil(t, apiErr)
			require.EqualValues(t, 1, calls.Load())
			body := <-observed
			require.True(t, strings.Contains(string(body), `"defer_loading":true`))
			require.Contains(t, string(body), "synthetic schema content")
			require.NotContains(t, string(body), "TrustedDeferredLoading")
			require.NotContains(t, string(body), "trusted_deferred_loading")
			require.EqualValues(t, 18, requestCostQuota(t, id))
			require.Equal(t, balance-18, reloadUserQuota(t))
			require.Equal(t, balance-18, token.RemainQuota)
			require.EqualValues(t, 18, token.UsedQuota)
		})
	}
}
