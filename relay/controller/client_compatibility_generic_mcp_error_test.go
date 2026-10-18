package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestClientCompatibilityGenericMCPMergeErrorSettlesAcceptedHolds drives the
// actual Chat controller through one accepted MCP round and one HTTP200 receipt
// whose cross-round prompt-token sum is unrepresentable. It requires an
// explicit failure, no retry, and one reconciled ledger for the already held
// allowance rather than a fictional saturated token charge or an unsafe refund.
func TestClientCompatibilityGenericMCPMergeErrorSettlesAcceptedHolds(t *testing.T) {
	clientCompatibilityGenericMCPLoop(t, true, "chat")
}

// TestClientCompatibilityGenericMCPNormalLoop proves the same owned alias and
// provider schema execute two legitimate rounds before testing failure settlement.
func TestClientCompatibilityGenericMCPNormalLoop(t *testing.T) {
	clientCompatibilityGenericMCPLoop(t, false, "chat")
}

// TestClientCompatibilityGenericMCPResponsesError settles accepted fallback work before returning its receipt error.
func TestClientCompatibilityGenericMCPResponsesError(t *testing.T) {
	clientCompatibilityGenericMCPLoop(t, true, "responses")
}

// TestClientCompatibilityGenericMCPResponsesNormal proves fallback routing completes two provider rounds and one tool call.
func TestClientCompatibilityGenericMCPResponsesNormal(t *testing.T) {
	clientCompatibilityGenericMCPLoop(t, false, "responses")
}

// TestClientCompatibilityGenericMCPClaudeError settles accepted converted Messages work before returning its receipt error.
func TestClientCompatibilityGenericMCPClaudeError(t *testing.T) {
	clientCompatibilityGenericMCPLoop(t, true, "claude")
}

// TestClientCompatibilityGenericMCPClaudeNormal proves converted Messages dispatches only the two owned MCP rounds.
func TestClientCompatibilityGenericMCPClaudeNormal(t *testing.T) {
	clientCompatibilityGenericMCPLoop(t, false, "claude")
}

// clientCompatibilityGenericMCPLoop runs an owned local alias through the real
// Chat or Responses controller. Parameters: fail selects receipt overflow and protocol selects the downstream format.
// Returns: nothing; assertions cover transport ownership and durable accounting.
func clientCompatibilityGenericMCPLoop(t *testing.T, fail bool, protocol string) {
	t.Helper()
	oldRounds := config.MCPMaxToolRounds
	config.MCPMaxToolRounds = 2
	t.Cleanup(func() { config.MCPMaxToolRounds = oldRounds })
	const balance = int64(1_000_000)
	const name = "compatibility_merge_probe"
	xaiVideoSetup(t, balance, false)
	var toolCalls atomic.Int32
	mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var rpc struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&rpc); err != nil {
			http.Error(w, "bad fixture", 400)
			return
		}
		if strings.HasPrefix(rpc.Method, "notifications/") {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		var result any
		switch rpc.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]any{"name": "local-merge-error-fixture", "version": "1"}}
		case "tools/call":
			toolCalls.Add(1)
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": "local fixture result"}}}
		default:
			http.Error(w, "unexpected fixture method", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": rpc.ID, "result": result}); err != nil {
			return
		}
	}))
	t.Cleanup(mcpServer.Close)
	stored := &model.MCPServer{Name: "client-compatibility-generic-merge", Status: model.MCPServerStatusEnabled, BaseURL: mcpServer.URL, ToolWhitelist: model.JSONStringSlice{name}}
	require.NoError(t, model.DB.Create(stored).Error)
	t.Cleanup(func() { require.NoError(t, model.DB.Delete(stored).Error) })
	tool := &model.MCPTool{ServerId: stored.Id, Name: name, InputSchema: `{"type":"object","properties":{}}`, Status: 1}
	require.NoError(t, model.DB.Create(tool).Error)
	t.Cleanup(func() { require.NoError(t, model.DB.Delete(tool).Error) })
	var providerCalls, ownedRounds atomic.Int32
	holds := make(chan int64, 4)
	offered := make(chan string, 4)
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var outgoing struct {
			Tools []struct {
				Type     string `json:"type"`
				Function *struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tools"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&outgoing); err != nil {
			http.Error(w, "bad fixture body", 400)
			return
		}
		functionName := ""
		if len(outgoing.Tools) > 0 && outgoing.Tools[0].Function != nil && outgoing.Tools[0].Type == "function" {
			functionName = outgoing.Tools[0].Function.Name
		}
		offered <- functionName
		providerCalls.Add(1)
		var user model.User
		if err := model.DB.First(&user, fallbackUserID).Error; err != nil {
			http.Error(w, "fixture balance unavailable", 500)
			return
		}
		holds <- balance - user.Quota
		w.Header().Set("Content-Type", "application/json")
		// An unexpanded converted Messages request can expose the legacy extra
		// pre-loop dispatch. Accept it with a benign response so the strict total
		// call gate, rather than a mock rejection, detects duplicated work.
		if functionName == "" && protocol == "claude" {
			_, _ = io.WriteString(w, `{"id":"chat_unexpanded","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"initial response"},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`)
			return
		}
		round := ownedRounds.Add(1)
		if round == 1 {
			if functionName == "" {
				http.Error(w, "fixture expected a gateway-expanded function tool", 400)
				return
			}
			encodedName, err := json.Marshal(functionName)
			if err != nil {
				http.Error(w, "fixture name encode failed", 500)
				return
			}
			_, _ = fmt.Fprintf(w, `{"id":"chat_mcp_first","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","tool_calls":[{"id":"call_local","type":"function","function":{"name":%s,"arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`, encodedName)
			return
		}
		if !fail {
			_, _ = io.WriteString(w, `{"id":"chat_mcp_second","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`)
			return
		}
		_, _ = fmt.Fprintf(w, `{"id":"chat_mcp_second","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],"usage":{"prompt_tokens":%d,"completion_tokens":0,"total_tokens":%d}}`, math.MaxInt, math.MaxInt)
	}))
	t.Cleanup(provider.Close)
	previous := client.HTTPClient
	client.HTTPClient = provider.Client()
	t.Cleanup(func() { client.HTTPClient = previous })
	payload := `{"model":"alias","max_tokens":32,"messages":[{"role":"user","content":"hello"}],"tools":[{"type":"client-compatibility-generic-merge.compatibility_merge_probe"}]}`
	// OpenAICompatible defaults to Chat; the OpenAI channel intentionally
	// converts unknown models to Responses, which requires a different wire fixture.
	path := "/v1/chat/completions"
	if protocol == "responses" {
		path = "/v1/responses"
		payload = `{"model":"alias","max_output_tokens":32,"input":[{"type":"message","role":"user","content":"hello"}],"tools":[{"type":"client-compatibility-generic-merge.compatibility_merge_probe"}]}`
	}
	if protocol == "claude" {
		path = "/v1/messages"
		payload = `{"model":"alias","max_tokens":32,"messages":[{"role":"user","content":"hello"}],"tools":[{"type":"client-compatibility-generic-merge.compatibility_merge_probe"}]}`
	}
	c, recorder, requestID := protocolContext(t, channeltype.OpenAICompatible, "client-compatibility-mcp-model", path, payload, provider.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1, CacheWrite1hRatio: 1})
	if protocol == "claude" {
		// Unknown provider tool types remain fail-closed. This disposable
		// channel explicitly opts into its synthetic gateway alias at zero fee.
		channel, ok := c.MustGet(ctxkey.ChannelModel).(*model.Channel)
		require.True(t, ok)
		require.NoError(t, channel.SetToolingConfig(&model.ChannelToolingConfig{
			Pricing: map[string]model.ToolPricingLocal{"client-compatibility-generic-merge.compatibility_merge_probe": {}},
		}))
	}
	responseNamespaceCleanLedger(t, requestID)
	relay := RelayTextHelper
	if protocol == "responses" {
		relay = RelayResponseAPIHelper
	}
	if protocol == "claude" {
		relay = RelayClaudeMessagesHelper
	}
	apiErr := relay(c)
	drainCriticalTasks(t)
	// These are synthetic fixture responses only; no real request data or secrets.
	t.Logf("synthetic downstream body: %s", recorder.Body.String())
	if converted, exists := c.Get(ctxkey.ConvertedResponse); exists {
		encoded, encodeErr := json.Marshal(converted)
		require.NoError(t, encodeErr)
		t.Logf("synthetic converted response (%T): %s", converted, encoded)
	}
	offeredNames := make([]string, 0, len(offered))
	for len(offered) > 0 {
		offeredNames = append(offeredNames, <-offered)
	}
	t.Logf("synthetic offered tool names: %q", offeredNames)
	require.EqualValues(t, 2, providerCalls.Load(), "API=%v tool_calls=%d", apiErr, toolCalls.Load())
	require.EqualValues(t, 1, toolCalls.Load())
	require.Equal(t, name, offeredNames[0], "provider must call the exact expanded gateway-owned alias")
	if !fail {
		require.Nil(t, apiErr)
		require.EqualValues(t, 36, requestCostQuota(t, requestID))
		require.Equal(t, balance-36, reloadUserQuota(t))
		return
	}
	require.NotNil(t, apiErr)
	require.Equal(t, "invalid_mcp_usage", apiErr.Code)
	require.False(t, BillingAllowsRetry(c), "paid accepted rounds must not replay after a receipt failure")
	firstHeld, allHeld := <-holds, <-holds
	require.Greater(t, allHeld, firstHeld, "both provider dispatches must exercise real reservations")
	require.Equal(t, allHeld, requestCostQuota(t, requestID), "reconcile the existing held allowance, not a wrapped or saturated token charge")
	require.Equal(t, balance-allHeld, reloadUserQuota(t))
	var token model.Token
	require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
	require.Equal(t, balance-allHeld, token.RemainQuota)
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", requestID, model.LogTypeConsume).Find(&logs).Error)
	require.Len(t, logs, 1)
	require.EqualValues(t, allHeld, logs[0].Quota)
}
