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
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/mcp"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// largeToolDescription is a provider-visible tool description large enough
// that omitting it from a prompt estimate is visible in every quote below.
var largeToolDescription = strings.Repeat("schema field documentation ", 3_000)

// largeToolSchemaTokens returns the independent token oracle for one function
// tool carrying largeToolDescription. Parameter model selects the tokenizer.
// It returns the token count of the tool's provider JSON.
func largeToolSchemaTokens(t *testing.T, model string, tool any) int {
	t.Helper()
	encoded, err := json.Marshal(tool)
	require.NoError(t, err)
	tokens := openai.CountTokenText(string(encoded), model)
	require.Greater(t, tokens, 10_000, "the fixture must dominate the small message")
	return tokens
}

// TestPromptEstimatesCountToolDefinitions pins the shared prompt estimators:
// Chat counts tool and function definitions, response_format schemas and
// assistant tool-call arguments, and Responses counts tools and text formats,
// because providers bill all of them as input. Parameter t owns assertions.
func TestPromptEstimatesCountToolDefinitions(t *testing.T) {
	ctx := context.Background()
	const name = "gpt-6.2-astra"
	tool := relaymodel.Tool{Type: "function", Function: &relaymodel.Function{Name: "lookup", Description: largeToolDescription,
		Parameters: map[string]any{"type": "object", "properties": map[string]any{}}}}
	toolTokens := largeToolSchemaTokens(t, name, tool)
	plain := &relaymodel.GeneralOpenAIRequest{Model: name, Messages: []relaymodel.Message{{Role: "user", Content: "hi"}}}
	base := getPromptTokens(ctx, plain, relaymode.ChatCompletions)
	require.Less(t, base, 50)

	withTools := *plain
	withTools.Tools = []relaymodel.Tool{tool}
	require.GreaterOrEqual(t, getPromptTokens(ctx, &withTools, relaymode.ChatCompletions), base+toolTokens)

	withFunctions := *plain
	withFunctions.Functions = []relaymodel.Function{*tool.Function}
	require.GreaterOrEqual(t, getPromptTokens(ctx, &withFunctions, relaymode.ChatCompletions),
		base+largeToolSchemaTokens(t, name, withFunctions.Functions))

	withFormat := *plain
	withFormat.ResponseFormat = &relaymodel.ResponseFormat{Type: "json_schema", JsonSchema: &relaymodel.JSONSchema{
		Name: "answer", Description: largeToolDescription, Schema: map[string]any{"type": "object"}}}
	require.GreaterOrEqual(t, getPromptTokens(ctx, &withFormat, relaymode.ChatCompletions),
		base+largeToolSchemaTokens(t, name, withFormat.ResponseFormat))

	withCall := *plain
	withCall.Messages = append([]relaymodel.Message(nil), plain.Messages...)
	withCall.Messages = append(withCall.Messages, relaymodel.Message{Role: "assistant", ToolCalls: []relaymodel.Tool{{
		Id: "call_1", Type: "function", Function: &relaymodel.Function{Name: "lookup", Arguments: largeToolDescription}}}})
	require.GreaterOrEqual(t, getPromptTokens(ctx, &withCall, relaymode.ChatCompletions),
		base+largeToolSchemaTokens(t, name, withCall.Messages[1].ToolCalls),
		"assistant tool-call arguments are replayed provider input")

	responses := &openai.ResponseAPIRequest{Model: name, Input: openai.ResponseAPIInput{"hi"}}
	responseBase := getResponseAPIPromptTokens(ctx, responses)
	responseTool := map[string]any{"type": "function", "name": "lookup", "description": largeToolDescription,
		"parameters": map[string]any{"type": "object", "properties": map[string]any{}}}
	responseToolTokens := largeToolSchemaTokens(t, name, responseTool)
	responses.Tools = []openai.ResponseAPITool{{Type: "function", Name: "lookup", Description: largeToolDescription,
		Parameters: map[string]any{"type": "object", "properties": map[string]any{}}}}
	require.Equal(t, 10, responseBase, "a tiny input reaches the estimator floor")
	responseCount := getResponseAPIPromptTokens(ctx, responses)
	require.GreaterOrEqual(t, responseCount, responseToolTokens)
	require.InDelta(t, toolTokens, responseCount, 30,
		"the same definition costs the same on both surfaces; the nested function duplicate is not counted")
	nested := &openai.ResponseAPIRequest{Model: name, Input: openai.ResponseAPIInput{"hi"}, Tools: []openai.ResponseAPITool{{
		Type: "namespace", Name: "suite", Tools: []openai.ResponseAPITool{{Type: "function", Function: tool.Function}}}}}
	require.InDelta(t, toolTokens, getResponseAPIPromptTokens(ctx, nested), 30, "namespace children are counted once")

	formatted := &openai.ResponseAPIRequest{Model: name, Input: openai.ResponseAPIInput{"hi"}, Text: &openai.ResponseTextConfig{
		Format: &openai.ResponseTextFormat{Type: "json_schema", Name: "answer", Description: largeToolDescription, Schema: map[string]any{"type": "object"}}}}
	// responseBase is the estimator's 10-token floor, so compare the schema alone.
	require.GreaterOrEqual(t, getResponseAPIPromptTokens(ctx, formatted), largeToolSchemaTokens(t, name, formatted.Text))
}

// TestRelayPromptHoldCoversToolDefinitions proves through the production
// Chat and Responses helpers that the admission hold funds tool schemas sent
// to the provider. A native flat write override makes the whole prompt,
// including the schema, a 30m write at 6 quota per token. Parameter t owns the
// SQLite ledger and the fake OpenAI upstream.
func TestRelayPromptHoldCoversToolDefinitions(t *testing.T) {
	const name = "gpt-6.2-astra"
	chatTool := relaymodel.Tool{Type: "function", Function: &relaymodel.Function{Name: "lookup", Description: largeToolDescription,
		Parameters: map[string]any{"type": "object", "properties": map[string]any{}}}}
	toolTokens := largeToolSchemaTokens(t, name, chatTool)
	tools, err := json.Marshal([]relaymodel.Tool{chatTool})
	require.NoError(t, err)
	responseTools := `[{"type":"function","name":"lookup","description":"` + largeToolDescription + `","parameters":{"type":"object","properties":{}}}]`
	claudeTools := `[{"name":"lookup","description":"` + largeToolDescription + `","input_schema":{"type":"object","properties":{}}}]`
	for _, tc := range []struct{ protocol, path, body string }{
		{protocol: "chat", path: "/v1/chat/completions", body: `{"model":"alias","max_tokens":128,"messages":[{"role":"user","content":"hi"}],"tools":` + string(tools) + `}`},
		{protocol: "responses", path: "/v1/responses", body: `{"model":"alias","max_output_tokens":128,"input":"hi","tools":` + responseTools + `}`},
		// Claude Messages already counted tools; it is the control.
		{protocol: "messages", path: "/v1/messages", body: `{"model":"alias","max_tokens":128,"messages":[{"role":"user","content":"hi"}],"tools":` + claudeTools + `}`},
	} {
		t.Run(tc.protocol, func(t *testing.T) {
			const balance = int64(100_000_000)
			xaiVideoSetup(t, balance, false)
			var calls atomic.Int32
			upstream, observations := implicitCacheUpstream(t, name, 20_000, 128, &calls)
			local := nativeFlatOverride
			result := relayNativeCacheRequest(t, name, tc.protocol, tc.path, tc.body, upstream, balance, &local)
			require.False(t, result.failed, result.message)
			observation := <-observations
			require.NoError(t, observation.err)
			require.Contains(t, observation.body, "tools", "the fixture must forward the schema")
			reserved := balance - observation.userQuota
			// Each surface wraps the same definition in a slightly different
			// envelope, so the floor allows a few wrapper tokens.
			schemaFloor := toolTokens - 30
			require.GreaterOrEqual(t, result.promptTokens, schemaFloor, "the admitted prompt must include the tool schema")
			require.Less(t, result.promptTokens, 2*schemaFloor, "the tool schema is counted once")
			require.GreaterOrEqual(t, reserved, int64(schemaFloor)*6+128*12,
				"the hold must fund the tool schema as written prompt input")
			requireExactSettlement(t, result, 20_000*6+128*12)
		})
	}
}

// TestMCPChatRoundQuoteCountsToolDefinitions proves every Chat MCP round
// funds the gateway-injected tool schema that the round actually sends. The
// OpenRouter flat cache-write contract prices the whole prompt, schema included,
// at 5 quota per token; the message and output limit stay tiny. Parameter t
// owns the SQLite ledger, the local MCP server and the fake provider.
func TestMCPChatRoundQuoteCountsToolDefinitions(t *testing.T) {
	const name = "openai/gpt-6.2-astra"
	const balance = int64(100_000_000)
	xaiVideoSetup(t, balance, false)
	priorRounds := config.MCPMaxToolRounds
	config.MCPMaxToolRounds = 2
	t.Cleanup(func() { config.MCPMaxToolRounds = priorRounds })

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
			result = map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]any{"name": "tool-token-probe", "version": "1"}}
		case "tools/call":
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
	stored := &model.MCPServer{Name: "tool-token-probe", Status: model.MCPServerStatusEnabled, BaseURL: toolServer.URL}
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

	type roundObservation struct {
		tools     int
		userQuota int64
		err       error
	}
	var providerCalls atomic.Int32
	observed := make(chan roundObservation, 2)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		round := providerCalls.Add(1)
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<20))
		got := roundObservation{err: err}
		var payload struct {
			Tools json.RawMessage `json:"tools"`
		}
		if got.err == nil {
			got.err = json.Unmarshal(body, &payload)
		}
		if got.err == nil {
			got.tools = openai.CountTokenText(string(payload.Tools), name)
			var user model.User
			got.err = model.DB.First(&user, fallbackUserID).Error
			got.userQuota = user.Quota
		}
		observed <- got
		if round > 2 {
			http.Error(w, "unexpected local provider round", http.StatusBadRequest)
			return
		}
		message, finish := `{"role":"assistant","content":"done"}`, "stop"
		if round == 1 {
			message, finish = `{"role":"assistant","content":null,"tool_calls":[{"id":"probe-call","type":"function","function":{"name":"probe","arguments":"{}"}}]}`, "tool_calls"
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprintf(w, `{"id":"tool-token-%d","object":"chat.completion","model":%q,"choices":[{"index":0,"message":%s,"finish_reason":"%s"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`, round, name, message, finish); err != nil {
			return
		}
	}))
	t.Cleanup(provider.Close)
	previousClient := client.HTTPClient
	client.HTTPClient = provider.Client()
	t.Cleanup(func() { client.HTTPClient = previousClient })

	local := model.ModelConfigLocal{Ratio: 4, CompletionRatio: 5, CacheWrite5mRatio: 5}
	c, _, _ := protocolContext(t, channeltype.OpenRouter, name, "/v1/chat/completions", "{}", provider.URL, balance, 1, false, &local)
	m := metalib.GetByContext(c)
	m.OriginModelName, m.ActualModelName = name, name
	request := &relaymodel.GeneralOpenAIRequest{Model: name, MaxTokens: 1,
		Messages: []relaymodel.Message{{Role: "user", Content: "hi"}},
		Tools: []relaymodel.Tool{{Type: "function", Function: &relaymodel.Function{Name: "probe", Description: largeToolDescription,
			Parameters: map[string]any{"type": "object", "properties": map[string]any{}}}}}}
	_, usage, _, _, apiErr := executeChatMCPToolLoop(c, m, request, registry, 0)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.EqualValues(t, 2, providerCalls.Load(), "the real loop must dispatch the tool continuation")
	for round := 1; round <= 2; round++ {
		got := <-observed
		require.NoError(t, got.err)
		require.Greater(t, got.tools, 10_000, "round %d must send the injected schema", round)
		reserved := balance - got.userQuota
		require.GreaterOrEqual(t, reserved, int64(got.tools)*5,
			"round %d hold must fund the tool schema it sends as written prompt input", round)
	}
}
