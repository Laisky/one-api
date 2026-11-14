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

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/mcp"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestGemini4MCPOutputSchemaSurvivesToolRound retains native constraints across two actual compatible provider dispatches.
func TestGemini4MCPOutputSchemaSurvivesToolRound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ensureResponseFallbackFixtures(t)
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
	stored := &model.MCPServer{Name: "gemini4-output-schema-probe", Status: model.MCPServerStatusEnabled, BaseURL: toolServer.URL}
	require.NoError(t, model.DB.Create(stored).Error)
	t.Cleanup(func() { require.NoError(t, model.DB.Delete(stored).Error) })
	tool := &model.MCPTool{ServerId: stored.Id, Name: "probe", InputSchema: `{"type":"object","properties":{}}`}
	require.NoError(t, model.DB.Create(tool).Error)
	t.Cleanup(func() { require.NoError(t, model.DB.Delete(tool).Error) })
	registry := &mcpToolRegistry{
		candidatesByName: map[string][]mcp.ToolCandidate{"probe": {{ResolvedTool: mcp.ResolvedTool{
			Tool: tool, ServerID: stored.Id, ServerLabel: stored.Name, ServerURL: toolServer.URL,
			Policy: mcp.ToolPolicySnapshot{Allowed: true},
		}}}},
		requestHeaders: map[string]map[string]string{}, selectedIndex: map[string]int{},
	}

	var providerCalls atomic.Int32
	observed := make(chan []byte, 2)
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
		observed <- body
		message, finish := `{"role":"assistant","content":"done"}`, "stop"
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

	const nativeConfig = `{"format":{"type":"json_schema","schema":{"type":"object","properties":{"count":{"type":"integer","minimum":1,"maximum":7}},"required":["count"],"additionalProperties":false}}}`
	source := &openai.ResponseAPIRequest{
		Model: "gemini-4-hydrogen", Input: openai.ResponseAPIInput{map[string]any{"role": "user", "content": "extract a count"}},
		OutputConfig: json.RawMessage(nativeConfig),
	}
	request, err := openai.ConvertResponseAPIToChatCompletionRequest(source)
	require.NoError(t, err)
	request.MaxTokens = 32
	request.Tools = []relaymodel.Tool{{Type: "function", Function: &relaymodel.Function{Name: "probe", Parameters: map[string]any{"type": "object", "properties": map[string]any{}}}}}
	channel := &model.Channel{Type: channeltype.GeminiOpenAICompatible}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	gmw.SetLogger(c, logger.Logger)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set(ctxkey.ChannelModel, channel)
	// A zero group multiplier isolates request conversion from paid quota reservation.
	c.Set(ctxkey.ChannelRatio, float64(0))
	info := &metalib.Meta{
		APIType: channeltype.ToAPIType(channeltype.GeminiOpenAICompatible), ChannelType: channeltype.GeminiOpenAICompatible,
		Mode: relaymode.ChatCompletions, ActualModelName: request.Model, OriginModelName: request.Model,
		BaseURL: provider.URL, RequestURLPath: "/v1/chat/completions",
	}
	metalib.Set2Context(c, info)
	response, usage, _, incremental, apiErr := executeChatMCPToolLoop(c, info, request, registry, 0)
	require.Nil(t, apiErr)
	require.NotNil(t, response)
	require.NotNil(t, usage)
	require.Zero(t, incremental)
	require.EqualValues(t, 2, providerCalls.Load(), "the real loop must dispatch the tool continuation")
	require.EqualValues(t, 1, toolCalls.Load())
	require.Equal(t, nativeConfig, string(source.OutputConfig), "the Responses source raw schema stays unchanged")
	for round := 1; round <= 2; round++ {
		body := <-observed
		var payload map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(body, &payload))
		require.NotContains(t, payload, "output_config")
		var format struct {
			Schema struct {
				Schema json.RawMessage `json:"schema"`
			} `json:"json_schema"`
		}
		require.NoError(t, json.Unmarshal(payload["response_format"], &format))
		require.JSONEq(t, `{"type":"object","properties":{"count":{"type":"integer","minimum":1,"maximum":7}},"required":["count"],"additionalProperties":false}`, string(format.Schema.Schema), "round %d must preserve both bounds", round)
	}
}
