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
	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/mcp"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

const clientCompatibilityClaudePayload = `{"model":"claude-sonnet-4","max_tokens":32,"stream":false,"context_management":{"edits":[{"type":"clear_tool_uses_20250919","keep":{"type":"tool_uses","value":2}}]},"vendor_control":{"exact":9007199254740993},"system":[{"type":"text","text":"synthetic policy","cache_control":{"type":"ephemeral","ttl":"1h"}}],"tools":[{"name":"probe_a","description":"First local fixture","input_schema":{"type":"object"},"defer_loading":true,"cache_control":{"type":"ephemeral"},"vendor_tool":{"exact":9007199254740993}},{"name":"probe_b","input_schema":{"type":"object"}}],"messages":[{"role":"user","content":[{"type":"text","text":"hello","cache_control":{"type":"ephemeral"}}]}]}`

const clientCompatibilityClaudeOpaque = `[{"type":"thinking","thinking":"synthetic thinking","signature":"sig-fixture","vendor_thinking":{"exact":9007199254740993}},{"type":"redacted_thinking","data":"opaque-redacted-fixture","vendor_redacted":"retain"},{"type":"compaction","content":"opaque-compacted-state","signature":"sig-compaction","vendor_compaction":{"exact":9007199254740993}},{"type":"tool_use","id":"call_a","name":"probe_a","input":{},"vendor_tool_use":{"exact":9007199254740993}}]`

// clientCompatibilityClaudeLoopResult holds bounded local loop observations.
type clientCompatibilityClaudeLoopResult struct {
	response      *anthropic.Response
	usage         *relaymodel.Usage
	err           *relaymodel.ErrorWithStatusCode
	providerCalls int32
	toolNames     []string
	toolArguments []json.RawMessage
	cost          int64
	bodies        [][]byte
}

// clientCompatibilityClaudeLoop runs actual native rounds and MCP transport with fake tools only.
func clientCompatibilityClaudeLoop(t *testing.T, turns []string, controllerEntry ...bool) clientCompatibilityClaudeLoopResult {
	t.Helper()
	const balance = int64(1_000_000)
	xaiVideoSetup(t, balance, false)
	toolNames := make(chan string, 16)
	toolArguments := make(chan json.RawMessage, 16)
	mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var rpc struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
			Params struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			} `json:"params"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&rpc); err != nil {
			http.Error(w, "invalid local MCP fixture", http.StatusBadRequest)
			return
		}
		if strings.HasPrefix(rpc.Method, "notifications/") {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		var result any
		switch rpc.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]any{"name": "client-compatibility-fixture", "version": "1"}}
		case "tools/call":
			select {
			case toolNames <- rpc.Params.Name:
			default:
			}
			select {
			case toolArguments <- append(json.RawMessage(nil), rpc.Params.Arguments...):
			default:
			}
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": "result:" + rpc.Params.Name}}}
		default:
			http.Error(w, "unexpected local MCP method", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": rpc.ID, "result": result})
	}))
	t.Cleanup(mcpServer.Close)
	stored := &model.MCPServer{Name: "client-compatibility-loop", Status: model.MCPServerStatusEnabled, BaseURL: mcpServer.URL}
	require.NoError(t, model.DB.Create(stored).Error)
	t.Cleanup(func() { require.NoError(t, model.DB.Delete(stored).Error) })
	registry := &claudeToolSearchMCPRegistry{candidatesByName: map[string][]mcp.ToolCandidate{}, requestHeaders: map[string]map[string]string{}, selectedIndex: map[string]int{}}
	for _, name := range []string{"probe_a", "probe_b"} {
		tool := &model.MCPTool{ServerId: stored.Id, Name: name, InputSchema: `{"type":"object","properties":{}}`}
		require.NoError(t, model.DB.Create(tool).Error)
		t.Cleanup(func() { require.NoError(t, model.DB.Delete(tool).Error) })
		registry.candidatesByName[name] = []mcp.ToolCandidate{{ResolvedTool: mcp.ResolvedTool{Tool: tool, ServerID: stored.Id, ServerLabel: stored.Name, ServerURL: mcpServer.URL, Policy: mcp.ToolPolicySnapshot{Allowed: true}}}}
	}
	var providerCalls atomic.Int32
	bodies := make(chan []byte, 16)
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			http.Error(w, "invalid local provider body", http.StatusBadRequest)
			return
		}
		select {
		case bodies <- body:
		default:
		}
		round := int(providerCalls.Add(1)) - 1
		w.Header().Set("Content-Type", "application/json")
		if round >= len(turns) {
			_, _ = io.WriteString(w, `{"id":"unexpected-extra-round","type":"message","role":"assistant","content":[{"type":"text","text":"unexpected extra round"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
			return
		}
		_, _ = io.WriteString(w, turns[round])
	}))
	t.Cleanup(provider.Close)
	previous := client.HTTPClient
	client.HTTPClient = provider.Client()
	t.Cleanup(func() { client.HTTPClient = previous })
	payload := clientCompatibilityClaudePayload
	fullController := len(controllerEntry) > 0 && controllerEntry[0]
	if fullController {
		// Caller-owned names deliberately do not collide with the gateway
		// catalog: injection must own probe_a/probe_b for this ledger fixture.
		var root map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(payload), &root))
		root["tools"] = json.RawMessage(`[{"type":"tool_search_tool_regex_20251119","name":"tool_search_tool_regex"}]`)
		encoded, encodeErr := json.Marshal(root)
		require.NoError(t, encodeErr)
		payload = string(encoded)
	}
	c, recorder, requestID := protocolContext(t, channeltype.Anthropic, "claude-sonnet-4", "/v1/messages", payload, provider.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
	meta := metalib.GetByContext(c)
	adapter := &anthropic.Adaptor{}
	adapter.Init(meta)
	var request ClaudeMessagesRequest
	require.NoError(t, json.Unmarshal([]byte(payload), &request))
	_, err := adapter.ConvertClaudeRequest(c, &request)
	require.NoError(t, err)
	var response *anthropic.Response
	var usage *relaymodel.Usage
	var apiErr *relaymodel.ErrorWithStatusCode
	if fullController {
		responseNamespaceCleanLedger(t, requestID)
		apiErr = RelayClaudeMessagesHelper(c)
		if apiErr == nil {
			response = &anthropic.Response{}
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), response))
		}
	} else {
		response, usage, _, _, apiErr = executeClaudeToolSearchMCPLoop(c, meta, &request, registry, adapter, 0)
	}
	drainCriticalTasks(t)
	result := clientCompatibilityClaudeLoopResult{response: response, usage: usage, err: apiErr, providerCalls: providerCalls.Load()}
	for len(toolNames) > 0 {
		result.toolNames = append(result.toolNames, <-toolNames)
	}
	for len(toolArguments) > 0 {
		result.toolArguments = append(result.toolArguments, <-toolArguments)
	}
	if fullController && apiErr == nil {
		result.cost = requestCostQuota(t, requestID)
	}
	for len(bodies) > 0 {
		result.bodies = append(result.bodies, <-bodies)
	}
	return result
}

// clientCompatibilityClaudeTurn wraps raw opaque content in a complete local Claude response.
func clientCompatibilityClaudeTurn(content, stop string) string {
	return fmt.Sprintf(`{"id":"msg-fixture","type":"message","role":"assistant","model":"claude-sonnet-4","content":%s,"stop_reason":%q,"usage":{"input_tokens":11,"output_tokens":7}}`, content, stop)
}

// TestClientCompatibilityClaudeMixedOwnership yields the entire parallel turn before any gateway execution.
func TestClientCompatibilityClaudeMixedOwnership(t *testing.T) {
	for _, content := range []string{
		`[{"type":"tool_use","id":"call_a","name":"probe_a","input":{}},{"type":"tool_use","id":"call_client","name":"client_editor","input":{}}]`,
		`[{"type":"tool_use","id":"call_client","name":"client_editor","input":{}},{"type":"tool_use","id":"call_a","name":"probe_a","input":{}}]`,
	} {
		t.Run(fmt.Sprint(strings.Index(content, "client_editor")), func(t *testing.T) {
			result := clientCompatibilityClaudeLoop(t, []string{clientCompatibilityClaudeTurn(content, "tool_use")})
			require.Nil(t, result.err)
			require.Empty(t, result.toolNames, "mixed ownership must not execute even the MCP prefix")
			require.EqualValues(t, 1, result.providerCalls, "mixed ownership must yield without a follow-up provider round")
			require.NotNil(t, result.response)
			require.NotNil(t, result.response.StopReason)
			require.Equal(t, "tool_use", *result.response.StopReason)
			encoded, err := json.Marshal(result.response.Content)
			require.NoError(t, err)
			require.JSONEq(t, content, string(encoded), "yield all original client and MCP calls in original order")
		})
	}
}

// TestClientCompatibilityClaudeAllMCPOwnership is the positive two-round parallel execution control.
func TestClientCompatibilityClaudeAllMCPOwnership(t *testing.T) {
	content := `[{"type":"tool_use","id":"call_b","name":"probe_b","input":{}},{"type":"tool_use","id":"call_a","name":"probe_a","input":{}}]`
	result := clientCompatibilityClaudeLoop(t, []string{clientCompatibilityClaudeTurn(content, "tool_use"), clientCompatibilityClaudeTurn(`[{"type":"text","text":"done"}]`, "end_turn")})
	require.Nil(t, result.err)
	require.NotNil(t, result.response)
	require.EqualValues(t, 2, result.providerCalls)
	require.Equal(t, []string{"probe_b", "probe_a"}, result.toolNames, "execute each distinct call once in original parallel order")
	require.Len(t, result.bodies, 2)
	var second struct {
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(result.bodies[1], &second))
	require.Len(t, second.Messages, 3)
	require.JSONEq(t, content, string(second.Messages[1].Content))
	var results []struct {
		Type    string `json:"type"`
		ID      string `json:"tool_use_id"`
		Content string `json:"content"`
	}
	require.NoError(t, json.Unmarshal(second.Messages[2].Content, &results))
	require.Len(t, results, 2)
	require.Equal(t, "call_b", results[0].ID)
	require.Equal(t, "call_a", results[1].ID)
	require.Contains(t, results[0].Content, "result:probe_b")
	require.Contains(t, results[1].Content, "result:probe_a")
}

// TestClientCompatibilityClaudeOpaqueContinuation preserves provider-owned blocks through an actual next round.
func TestClientCompatibilityClaudeOpaqueContinuation(t *testing.T) {
	result := clientCompatibilityClaudeLoop(t, []string{clientCompatibilityClaudeTurn(clientCompatibilityClaudeOpaque, "tool_use"), clientCompatibilityClaudeTurn(`[{"type":"text","text":"done","vendor_text":{"exact":9007199254740993}}]`, "end_turn")})
	require.Nil(t, result.err)
	require.EqualValues(t, 2, result.providerCalls)
	require.Equal(t, []string{"probe_a"}, result.toolNames)
	require.Len(t, result.bodies, 2)
	var second struct {
		Messages []struct {
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(result.bodies[1], &second))
	require.Len(t, second.Messages, 3)
	require.JSONEq(t, clientCompatibilityClaudeOpaque, string(second.Messages[1].Content))
	require.Contains(t, string(second.Messages[1].Content), "9007199254740993", "opaque numeric tokens must not round")
	wire, err := json.Marshal(result.response.Content)
	require.NoError(t, err)
	require.Contains(t, string(wire), `"vendor_text"`, "final native content must retain unknown metadata")
	require.Contains(t, string(wire), "9007199254740993")
}

// TestClientCompatibilityClaudeOriginalControls preserves raw root/tool/cache fields in every native loop round.
func TestClientCompatibilityClaudeOriginalControls(t *testing.T) {
	first := clientCompatibilityClaudeTurn(`[{"type":"tool_use","id":"call_a","name":"probe_a","input":{}}]`, "tool_use")
	result := clientCompatibilityClaudeLoop(t, []string{first, clientCompatibilityClaudeTurn(`[{"type":"text","text":"done"}]`, "end_turn")})
	require.Nil(t, result.err)
	require.EqualValues(t, 2, result.providerCalls)
	var before map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(clientCompatibilityClaudePayload), &before))
	for round, body := range result.bodies {
		var after map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(body, &after))
		for _, key := range []string{"context_management", "vendor_control", "system", "tools"} {
			require.Contains(t, after, key, "round %d must preserve raw field %s", round, key)
			require.JSONEq(t, string(before[key]), string(after[key]), "round %d field %s", round, key)
		}
		require.Contains(t, string(body), "9007199254740993")
		var transcript struct {
			Messages []json.RawMessage `json:"messages"`
		}
		require.NoError(t, json.Unmarshal(body, &transcript))
		require.JSONEq(t, `{"role":"user","content":[{"type":"text","text":"hello","cache_control":{"type":"ephemeral"}}]}`, string(transcript.Messages[0]))
	}
}

// TestClientCompatibilityClaudeAssistantOpaqueReconstruction isolates native block fidelity from execution/billing.
func TestClientCompatibilityClaudeAssistantOpaqueReconstruction(t *testing.T) {
	var response anthropic.Response
	require.NoError(t, json.Unmarshal([]byte(clientCompatibilityClaudeTurn(clientCompatibilityClaudeOpaque, "tool_use")), &response))
	message := buildClaudeAssistantMessage(&response)
	encoded, err := json.Marshal(message.Content)
	require.NoError(t, err)
	require.JSONEq(t, clientCompatibilityClaudeOpaque, string(encoded))
	require.Contains(t, string(encoded), "9007199254740993")
}

// TestClientCompatibilityClaudeMCPExecutionPrecision checks actual MCP argument bytes, not only replay bytes.
func TestClientCompatibilityClaudeMCPExecutionPrecision(t *testing.T) {
	turn := clientCompatibilityClaudeTurn(`[{"type":"tool_use","id":"call_a","name":"probe_a","input":{"exact":9007199254740993}}]`, "tool_use")
	result := clientCompatibilityClaudeLoop(t, []string{turn, clientCompatibilityClaudeTurn(`[{"type":"text","text":"done"}]`, "end_turn")})
	require.Nil(t, result.err)
	require.EqualValues(t, 2, result.providerCalls)
	require.Len(t, result.toolArguments, 1)
	require.Contains(t, string(result.toolArguments[0]), "9007199254740993", "execution must receive the exact native numeric token")

	// A public typed mutation must remain authoritative over retained original input.
	var response anthropic.Response
	require.NoError(t, json.Unmarshal([]byte(turn), &response))
	response.Content[0].Input = map[string]any{"exact": json.Number("9007199254740995")}
	calls := extractMCPToolCalls(&response, &claudeToolSearchMCPRegistry{candidatesByName: map[string][]mcp.ToolCandidate{"probe_a": {{}}}})
	require.Len(t, calls, 1)
	encoded, err := json.Marshal(calls[0].Input)
	require.NoError(t, err)
	require.Contains(t, string(encoded), "9007199254740995")
	require.NotContains(t, string(encoded), "9007199254740993")
}

// TestClientCompatibilityClaudeMCPMissingInputPreservesObject keeps the prior native tool-input contract.
func TestClientCompatibilityClaudeMCPMissingInputPreservesObject(t *testing.T) {
	for _, input := range []string{"", `,"input":null`} {
		t.Run(fmt.Sprint(len(input)), func(t *testing.T) {
			turn := clientCompatibilityClaudeTurn(`[{"type":"tool_use","id":"call_a","name":"probe_a"`+input+`}]`, "tool_use")
			result := clientCompatibilityClaudeLoop(t, []string{turn, clientCompatibilityClaudeTurn(`[{"type":"text","text":"done"}]`, "end_turn")})
			require.Nil(t, result.err)
			require.EqualValues(t, 2, result.providerCalls)
			require.Len(t, result.toolArguments, 1)
			require.JSONEq(t, `{}`, string(result.toolArguments[0]))
			var body struct {
				Messages []struct {
					Content []struct {
						Type  string          `json:"type"`
						Input json.RawMessage `json:"input"`
					} `json:"content"`
				} `json:"messages"`
			}
			require.NoError(t, json.Unmarshal(result.bodies[1], &body))
			require.Len(t, body.Messages, 3)
			require.Len(t, body.Messages[1].Content, 1)
			require.JSONEq(t, `{}`, string(body.Messages[1].Content[0].Input), "assistant replay tool_use.input must remain an object")
		})
	}
}

// TestClientCompatibilityClaudeMCPIterationUsage verifies loop accumulation and final ledger use the shared usage contract.
func TestClientCompatibilityClaudeMCPIterationUsage(t *testing.T) {
	for _, tc := range []struct {
		name, usage   string
		input, output int
	}{
		{"compaction", `{"input_tokens":0,"output_tokens":0,"iterations":[{"type":"compaction","input_tokens":144,"output_tokens":276}]}`, 144, 276},
		{"canonical", `{"input_tokens":0,"output_tokens":0,"iterations":[{"type":"message","input_tokens":200000,"output_tokens":4000},{"type":"compaction","input_tokens":3000,"output_tokens":500}]}`, 203000, 4500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			first := clientCompatibilityClaudeTurn(`[{"type":"tool_use","id":"call_a","name":"probe_a","input":{}}]`, "tool_use")
			first = strings.Replace(first, `{"input_tokens":11,"output_tokens":7}`, tc.usage, 1)
			final := clientCompatibilityClaudeTurn(`[{"type":"text","text":"done"}]`, "end_turn")
			final = strings.Replace(final, `{"input_tokens":11,"output_tokens":7}`, `{"input_tokens":0,"output_tokens":0}`, 1)
			result := clientCompatibilityClaudeLoop(t, []string{first, final})
			require.Nil(t, result.err)
			require.NotNil(t, result.usage)
			require.Equal(t, tc.input, result.usage.PromptTokens)
			require.Equal(t, tc.output, result.usage.CompletionTokens)
			require.Equal(t, tc.input+tc.output, result.usage.TotalTokens)
		})
		t.Run(tc.name+"_controller_ledger", func(t *testing.T) {
			first := clientCompatibilityClaudeTurn(`[{"type":"tool_use","id":"call_a","name":"probe_a","input":{}}]`, "tool_use")
			first = strings.Replace(first, `{"input_tokens":11,"output_tokens":7}`, tc.usage, 1)
			final := clientCompatibilityClaudeTurn(`[{"type":"text","text":"done"}]`, "end_turn")
			final = strings.Replace(final, `{"input_tokens":11,"output_tokens":7}`, `{"input_tokens":0,"output_tokens":0}`, 1)
			result := clientCompatibilityClaudeLoop(t, []string{first, final}, true)
			require.Nil(t, result.err)
			require.EqualValues(t, 2, result.providerCalls, "ledger control must enter the actual native MCP loop")
			require.Equal(t, []string{"probe_a"}, result.toolNames)
			require.EqualValues(t, tc.input+tc.output, result.cost)
		})
	}
}

// TestClientCompatibilityClaudeMCPResponseBound rejects oversized provider content before retaining raw snapshots.
func TestClientCompatibilityClaudeMCPResponseBound(t *testing.T) {
	// This is one bounded local response, not a provider call or disk artifact.
	text, err := json.Marshal(strings.Repeat("x", maxClaudeMCPLoopBody))
	require.NoError(t, err)
	response := clientCompatibilityClaudeTurn(`[{"type":"text","text":`+string(text)+`}]`, "end_turn")
	result := clientCompatibilityClaudeLoop(t, []string{response})
	require.NotNil(t, result.err)
	require.Equal(t, "claude_mcp_response_too_large", result.err.Code)
	require.Equal(t, http.StatusBadGateway, result.err.StatusCode)
	require.EqualValues(t, 1, result.providerCalls)
	require.Empty(t, result.toolNames)
}
