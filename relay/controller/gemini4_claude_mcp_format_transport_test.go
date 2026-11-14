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

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestGemini4ClaudeMCPNativeFormatTransport retains native controls through actual Messages ingress, MCP execution, and two compatible HTTP dispatches.
func TestGemini4ClaudeMCPNativeFormatTransport(t *testing.T) {
	const balance int64 = 1_000_000
	const toolAlias = "gemini4-claude-mcp.probe"
	xaiVideoSetup(t, balance, false)
	previousRounds := config.MCPMaxToolRounds
	config.MCPMaxToolRounds = 2
	t.Cleanup(func() { config.MCPMaxToolRounds = previousRounds })

	var toolCalls atomic.Int32
	toolServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var rpc struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&rpc); err != nil {
			http.Error(w, "invalid local MCP request", http.StatusBadRequest)
			return
		}
		if strings.HasPrefix(rpc.Method, "notifications/") {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		var result any
		switch rpc.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]any{"name": "gemini-schema-probe", "version": "1"}}
		case "tools/call":
			toolCalls.Add(1)
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": "local result"}}}
		default:
			http.Error(w, "unexpected local MCP method", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": rpc.ID, "result": result}); err != nil {
			return
		}
	}))
	t.Cleanup(toolServer.Close)
	stored := &model.MCPServer{Name: "gemini4-claude-mcp", Status: model.MCPServerStatusEnabled, BaseURL: toolServer.URL, ToolWhitelist: model.JSONStringSlice{"probe"}}
	require.NoError(t, model.DB.Create(stored).Error)
	t.Cleanup(func() { require.NoError(t, model.DB.Delete(stored).Error) })
	tool := &model.MCPTool{ServerId: stored.Id, Name: "probe", Status: 1, InputSchema: `{"type":"object","properties":{}}`}
	require.NoError(t, model.DB.Create(tool).Error)
	t.Cleanup(func() { require.NoError(t, model.DB.Delete(tool).Error) })

	var providerCalls atomic.Int32
	observed := make(chan struct {
		body []byte
		path string
	}, 2)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			http.Error(w, "invalid local provider body", http.StatusBadRequest)
			return
		}
		round := providerCalls.Add(1)
		if round > 2 {
			http.Error(w, "unexpected local provider round", http.StatusBadRequest)
			return
		}
		observed <- struct {
			body []byte
			path string
		}{body: body, path: r.URL.Path}
		message, finish := `{"role":"assistant","content":"{\"count\":3}"}`, "stop"
		if round == 1 {
			message, finish = `{"role":"assistant","content":null,"tool_calls":[{"id":"schema-probe-call","type":"function","function":{"name":"probe","arguments":"{}"}}]}`, "tool_calls"
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprintf(w, `{"id":"local-schema-%d","object":"chat.completion","model":"gemini-4-hydrogen","choices":[{"index":0,"message":%s,"finish_reason":"%s"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`, round, message, finish); err != nil {
			return
		}
	}))
	t.Cleanup(provider.Close)
	previousClient := client.HTTPClient
	client.HTTPClient = provider.Client()
	t.Cleanup(func() { client.HTTPClient = previousClient })

	const nativeConfig = `{"effort":"high","format":{"type":"json_schema","name":"native_output","schema":{"type":"object","properties":{"count":{"type":"integer","minimum":1,"maximum":7}},"required":["count"],"additionalProperties":false}}}`
	body := `{"model":"alias","max_tokens":32,"messages":[{"role":"user","content":"Return structured JSON after the probe."}],"tools":[{"type":"` + toolAlias + `","name":"probe"}],"tool_choice":{"type":"tool","name":"probe"},"output_config":` + nativeConfig + `}`
	c, recorder, requestID := protocolContext(t, channeltype.GeminiOpenAICompatible, "gemini-4-hydrogen", "/v1/messages", body, provider.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
	channel, ok := c.MustGet(ctxkey.ChannelModel).(*model.Channel)
	require.True(t, ok)
	require.NoError(t, channel.SetToolingConfig(&model.ChannelToolingConfig{Pricing: map[string]model.ToolPricingLocal{toolAlias: {}}}))
	responseNamespaceCleanLedger(t, requestID)
	apiErr := RelayClaudeMessagesHelper(c)
	drainCriticalTasks(t)
	require.Nil(t, apiErr)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.EqualValues(t, 2, providerCalls.Load(), "the actual controller must run two MCP rounds")
	require.EqualValues(t, 1, toolCalls.Load())
	original, exists := c.Get(ctxkey.OriginalClaudeRequest)
	require.True(t, exists)
	source, ok := original.(*relaymodel.ClaudeRequest)
	require.True(t, ok)
	require.Equal(t, nativeConfig, string(source.OutputConfig))
	require.Equal(t, map[string]any{"type": "tool", "name": "probe"}, source.ToolChoice)
	raw, err := common.GetRequestBody(c)
	require.NoError(t, err)
	require.JSONEq(t, body, string(raw))
	for round := 1; round <= 2; round++ {
		got := <-observed
		var payload map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(got.body, &payload))
		t.Run(fmt.Sprintf("round%d/endpoint", round), func(t *testing.T) {
			require.Equal(t, "/chat/completions", got.path)
		})
		t.Run(fmt.Sprintf("round%d/format", round), func(t *testing.T) {
			require.Contains(t, payload, "response_format")
			require.NotContains(t, payload, "output_config")
			var format struct {
				Schema struct {
					Name   string          `json:"name"`
					Schema json.RawMessage `json:"schema"`
				} `json:"json_schema"`
			}
			require.NoError(t, json.Unmarshal(payload["response_format"], &format))
			require.Equal(t, "native_output", format.Schema.Name)
			require.JSONEq(t, `{"type":"object","properties":{"count":{"type":"integer","minimum":1,"maximum":7}},"required":["count"],"additionalProperties":false}`, string(format.Schema.Schema))
		})
		t.Run(fmt.Sprintf("round%d/tool-and-effort", round), func(t *testing.T) {
			require.JSONEq(t, `{"type":"function","function":{"name":"probe"}}`, string(payload["tool_choice"]))
			var tools []relaymodel.Tool
			require.NoError(t, json.Unmarshal(payload["tools"], &tools))
			require.Len(t, tools, 1)
			require.NotNil(t, tools[0].Function)
			require.Equal(t, "probe", tools[0].Function.Name)
			require.JSONEq(t, `"high"`, string(payload["reasoning_effort"]))
		})
	}
	var downstream struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &downstream))
	require.Len(t, downstream.Content, 1)
	require.Equal(t, "text", downstream.Content[0].Type)
	require.JSONEq(t, `{"count":3}`, downstream.Content[0].Text)
	require.EqualValues(t, 10, requestCostQuota(t, requestID))
	require.Equal(t, balance-10, reloadUserQuota(t))
}
