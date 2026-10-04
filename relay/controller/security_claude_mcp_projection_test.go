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

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/mcp"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/stretchr/testify/require"
)

// TestSecurityNativeClaudeMCPProjection runs real MCP initialization/tool HTTP
// calls and two native Claude rounds, then inspects the actual second request.
func TestSecurityNativeClaudeMCPProjection(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(fmt.Sprintf("legacy_aliases=%v", legacy), func(t *testing.T) {
			xaiVideoSetup(t, 1_000_000, false)
			const private = "synthetic-private-mcp-envelope"
			result := map[string]any{"content": []any{map[string]any{"type": "text", "text": "visible tool answer"}}, "_meta": map[string]any{"private": private}, "privateExtension": private}
			if legacy {
				result["structured_content"] = map[string]any{"answer": 42}
				result["is_error"] = true
			} else {
				result["structuredContent"] = map[string]any{"answer": 42}
				result["isError"] = true
			}
			var toolCalls atomic.Int32
			mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var rpc struct {
					ID     any    `json:"id"`
					Method string `json:"method"`
				}
				if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&rpc); err != nil {
					http.Error(w, "bad fixture request", 400)
					return
				}
				if strings.HasPrefix(rpc.Method, "notifications/") {
					w.WriteHeader(http.StatusAccepted)
					return
				}
				var payload any
				switch rpc.Method {
				case "initialize":
					payload = map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]any{"name": "synthetic", "version": "1"}}
				case "tools/call":
					toolCalls.Add(1)
					payload = result
				default:
					http.Error(w, "unexpected fixture method", 400)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": rpc.ID, "result": payload})
			}))
			t.Cleanup(mcpServer.Close)
			stored := &model.MCPServer{Name: fmt.Sprintf("projection-fixture-%v", legacy), Status: model.MCPServerStatusEnabled, BaseURL: mcpServer.URL}
			require.NoError(t, model.DB.Create(stored).Error)
			t.Cleanup(func() { require.NoError(t, model.DB.Delete(stored).Error) })
			tool := &model.MCPTool{ServerId: stored.Id, Name: "probe", InputSchema: `{"type":"object","properties":{}}`}
			require.NoError(t, model.DB.Create(tool).Error)
			t.Cleanup(func() { require.NoError(t, model.DB.Delete(tool).Error) })
			registry := &claudeToolSearchMCPRegistry{candidatesByName: map[string][]mcp.ToolCandidate{"probe": {{ResolvedTool: mcp.ResolvedTool{Tool: tool, ServerID: stored.Id, ServerLabel: stored.Name, ServerURL: mcpServer.URL, Policy: mcp.ToolPolicySnapshot{Allowed: true}}}}}, requestHeaders: map[string]map[string]string{}, selectedIndex: map[string]int{}}
			var upstreamCalls atomic.Int32
			observed := make(chan []byte, 2)
			provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
				if err != nil {
					http.Error(w, "bad fixture body", 400)
					return
				}
				observed <- body
				w.Header().Set("Content-Type", "application/json")
				if upstreamCalls.Add(1) == 1 {
					_, _ = io.WriteString(w, `{"id":"synthetic-turn-1","type":"message","role":"assistant","content":[{"type":"tool_use","id":"call_probe","name":"probe","input":{}}],"stop_reason":"tool_use","usage":{"input_tokens":1,"output_tokens":1}}`)
				} else {
					_, _ = io.WriteString(w, `{"id":"synthetic-turn-2","type":"message","role":"assistant","content":[{"type":"text","text":"done"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
				}
			}))
			t.Cleanup(provider.Close)
			old := client.HTTPClient
			client.HTTPClient = provider.Client()
			t.Cleanup(func() { client.HTTPClient = old })
			c, _, _ := protocolContext(t, channeltype.Anthropic, "claude-sonnet-4", "/v1/messages", `{"model":"claude-sonnet-4","max_tokens":1,"messages":[{"role":"user","content":"hello"}]}`, provider.URL, 1_000_000, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
			meta := metalib.GetByContext(c)
			providerAdaptor := &anthropic.Adaptor{}
			providerAdaptor.Init(meta)
			req := &ClaudeMessagesRequest{Model: "claude-sonnet-4", MaxTokens: 1, Messages: []relaymodel.ClaudeMessage{{Role: "user", Content: "hello"}}}
			final, usage, _, _, apiErr := executeClaudeToolSearchMCPLoop(c, meta, req, registry, providerAdaptor, 0)
			require.Nil(t, apiErr)
			require.NotNil(t, final)
			require.NotNil(t, usage)
			require.EqualValues(t, 2, upstreamCalls.Load())
			require.EqualValues(t, 1, toolCalls.Load())
			<-observed
			wire := <-observed
			require.Contains(t, string(wire), "tool_result")
			require.Contains(t, string(wire), "call_probe")
			require.Contains(t, string(wire), "visible tool answer")
			var request struct {
				Messages []struct {
					Content json.RawMessage `json:"content"`
				} `json:"messages"`
			}
			require.NoError(t, json.Unmarshal(wire, &request))
			require.Len(t, request.Messages, 3)
			var blocks []struct {
				Content string `json:"content"`
			}
			require.NoError(t, json.Unmarshal(request.Messages[2].Content, &blocks))
			require.Len(t, blocks, 1)
			var projected map[string]any
			require.NoError(t, json.Unmarshal([]byte(blocks[0].Content), &projected))
			if strings.Contains(string(wire), private) {
				t.Log("REPRODUCED_NATIVE_CLAUDE_PRIVATE_MCP_HISTORY")
			}
			require.NotContains(t, string(wire), private)
			require.Len(t, projected, 3)
			require.Equal(t, true, projected["isError"])
			require.Equal(t, float64(42), projected["structuredContent"].(map[string]any)["answer"])
			// Projection must not mutate the direct client transport contract.
			raw, err := json.Marshal(result)
			require.NoError(t, err)
			require.Contains(t, string(raw), private)
		})
	}
}
