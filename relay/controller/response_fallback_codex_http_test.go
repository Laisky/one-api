package controller

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
)

// codexFallbackCustomInputSchema is the single-string schema that carries a
// custom tool's raw input through function-only upstreams.
const codexFallbackCustomInputSchema = `{"type":"object","properties":{"input":{"type":"string","description":"The complete raw input for the tool, in the format the tool description requires."}},"required":["input"]}`

// codexFallbackExpectedTurns is the lowered transcript both Chat and Claude
// upstreams must receive for codexFallbackInput: discovery as a search call and
// result, flattened namespaced and custom calls with paired results, ONE
// assistant turn holding the multi-agent text and both collaboration calls,
// and the attributed agent message.
func codexFallbackExpectedTurns() []fallbackWireTurn {
	return []fallbackWireTurn{
		{Role: "user", Text: "Read the docs and patch the README."},
		{Role: "assistant", Calls: []fallbackWireCall{{ID: "s1", Name: "tool_search", Arguments: `{"query":"docs","limit":8,"cursor":9007199254740993}`}}},
		{Role: "tool", Results: []fallbackWireResult{{ID: "s1", Content: "Tool search loaded 1 tool(s); they are now available to call:\n- mcp__docs__read_doc: Read one document"}}},
		{Role: "assistant", Calls: []fallbackWireCall{{ID: "x", Name: "mcp__docs__read_doc", Arguments: `{"path":"a"}`}}},
		{Role: "tool", Results: []fallbackWireResult{{ID: "x", Content: "doc text"}}},
		{Role: "assistant", Calls: []fallbackWireCall{{ID: "p", Name: "apply_patch", Arguments: mustCodexJSON(map[string]string{"input": "*** Begin Patch\n*** Add File: a.txt\n+a\n*** End Patch\n"})}}},
		{Role: "tool", Results: []fallbackWireResult{{ID: "p", Content: "Success"}}},
		{Role: "assistant", Text: "Spawning a reviewer.", Calls: []fallbackWireCall{
			{ID: "a", Name: "collaboration__spawn_agent", Arguments: `{"task":"review"}`},
			{ID: "b", Name: "collaboration__wait_agent", Arguments: `{"id":"w1"}`},
		}},
		{Role: "tool", Results: []fallbackWireResult{{ID: "a", Content: "spawned w1"}}},
		{Role: "tool", Results: []fallbackWireResult{{ID: "b", Content: "w1 finished"}}},
		{Role: "user", Text: "[Agent message from /root to /root/worker]\nhi"},
	}
}

// requireFallbackWireTurns compares normalized upstream turns, matching tool
// arguments as JSON. Parameters: t owns assertions, want and got are turns.
func requireFallbackWireTurns(t *testing.T, want, got []fallbackWireTurn) {
	t.Helper()
	require.Len(t, got, len(want), "upstream turns: %+v", got)
	for i := range want {
		require.Equal(t, want[i].Role, got[i].Role, "turn %d", i)
		require.Equal(t, want[i].Text, got[i].Text, "turn %d", i)
		require.Equal(t, want[i].Results, got[i].Results, "turn %d", i)
		require.Len(t, got[i].Calls, len(want[i].Calls), "turn %d", i)
		for j, call := range want[i].Calls {
			require.Equal(t, call.ID, got[i].Calls[j].ID, "turn %d call %d", i, j)
			require.Equal(t, call.Name, got[i].Calls[j].Name, "turn %d call %d", i, j)
			require.JSONEq(t, call.Arguments, got[i].Calls[j].Arguments, "turn %d call %d", i, j)
		}
	}
}

// requireCodexFallbackUpstream asserts the upstream request for the synthetic
// Codex request: flattened tool names in catalog order, namespace description
// prefixes, the custom tool's input schema and grammar, the search tool's own
// schema, the lowered transcript and exact discovery number literals.
// Parameters: t owns assertions, channel picks the wire format, got is the
// provider observation.
func requireCodexFallbackUpstream(t *testing.T, channel int, got codexFallbackObservation) {
	t.Helper()
	require.Equal(t, fallbackUpstreamPath(channel), got.Path)
	require.Contains(t, string(got.Body), "9007199254740993", "tool_search_call arguments keep exact integer literals")
	wire := decodeFallbackWire(t, channel, got.Body)
	require.Equal(t, codexFallbackFlatTools, fallbackWireToolNames(wire))

	require.Equal(t, "Run a shell command", fallbackWireToolByName(t, wire, "exec_command").Description)
	require.Equal(t, "[Multi-agent collaboration] Spawn a worker agent", fallbackWireToolByName(t, wire, "collaboration__spawn_agent").Description)
	require.Equal(t, "[Multi-agent collaboration] Wait for a worker agent", fallbackWireToolByName(t, wire, "collaboration__wait_agent").Description)
	require.Equal(t, "[GitHub connector] Create an issue", fallbackWireToolByName(t, wire, "mcp__codex_apps__github_create_issue").Description)
	require.Equal(t, "[Docs server] Read one document", fallbackWireToolByName(t, wire, "mcp__docs__read_doc").Description)
	require.JSONEq(t, `{"type":"object","properties":{"task":{"type":"string"}},"required":["task"]}`, string(fallbackWireToolByName(t, wire, "collaboration__spawn_agent").Schema))

	patch := fallbackWireToolByName(t, wire, "apply_patch")
	require.JSONEq(t, codexFallbackCustomInputSchema, string(patch.Schema))
	require.Contains(t, patch.Description, "Use apply_patch to edit files. This is a FREEFORM tool.")
	require.Contains(t, patch.Description, "as the string argument `input`")
	require.Contains(t, patch.Description, "The input must match this lark grammar exactly:\nstart: begin_patch hunk+ end_patch\nbegin_patch: \"*** Begin Patch\" LF")

	search := fallbackWireToolByName(t, wire, "tool_search")
	require.Equal(t, "# Tool discovery\nSearch deferred tools.", search.Description)
	var searchSchema struct {
		Type       string                     `json:"type"`
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	require.NoError(t, json.Unmarshal(search.Schema, &searchSchema))
	require.Equal(t, "object", searchSchema.Type)
	require.JSONEq(t, `{"type":"string"}`, string(searchSchema.Properties["query"]))
	require.JSONEq(t, `{"type":"number"}`, string(searchSchema.Properties["limit"]))
	require.Equal(t, []string{"query"}, searchSchema.Required)

	requireFallbackWireTurns(t, codexFallbackExpectedTurns(), wire.Turns)
	if channel == channeltype.OpenAICompatible {
		require.Equal(t, "high", wire.Reasoning, "configuration_update overrides the reasoning effort")
	}
}

// codexFallbackExpectedItems returns the Responses items the upstream tool
// calls of channel must render as, in upstream order: namespaced function
// calls split into namespace and name with an empty encrypted_function_args
// list, a root function call without either field, a custom_tool_call with
// raw input, and a client tool_search_call whose integral limit is an integer.
func codexFallbackExpectedItems(channel int) []string {
	upstream := codexFallbackUpstreamCalls(channel)
	id := func(i int) string { return codexFallbackRenderedCallID(upstream[i].id) }
	return []string{
		fmt.Sprintf(`{"type":"function_call","id":%q,"status":"completed","call_id":%q,"name":"read_doc","namespace":"mcp__docs","arguments":"{\"path\":\"b\"}","encrypted_function_args":[]}`, id(0), id(0)),
		fmt.Sprintf(`{"type":"function_call","id":%q,"status":"completed","call_id":%q,"name":"spawn_agent","namespace":"collaboration","arguments":"{\"task\":\"fix\"}","encrypted_function_args":[]}`, id(1), id(1)),
		fmt.Sprintf(`{"type":"function_call","id":%q,"status":"completed","call_id":%q,"name":"_create_issue","namespace":"mcp__codex_apps__github","arguments":"{\"title\":\"bug\"}","encrypted_function_args":[]}`, id(2), id(2)),
		fmt.Sprintf(`{"type":"function_call","id":%q,"status":"completed","call_id":%q,"name":"exec_command","arguments":"{\"cmd\":\"ls\"}"}`, id(3), id(3)),
		fmt.Sprintf(`{"type":"custom_tool_call","id":%q,"status":"completed","call_id":%q,"name":"apply_patch","input":%s}`, id(4), id(4), mustCodexJSON(codexFallbackPatch)),
		fmt.Sprintf(`{"type":"tool_search_call","id":%q,"status":"completed","call_id":%q,"execution":"client","arguments":{"query":"calendar","limit":8}}`, id(5), id(5)),
	}
}

// requireCodexFallbackOutput asserts a rendered Responses output: the text
// message first, then exactly the expected tool items. Parameters: t owns
// assertions, channel selects the expected call IDs and output is the
// response output array.
func requireCodexFallbackOutput(t *testing.T, channel int, output []json.RawMessage) {
	t.Helper()
	expected := codexFallbackExpectedItems(channel)
	require.Len(t, output, len(expected)+1)
	var message struct {
		Type    string `json:"type"`
		Role    string `json:"role"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	require.NoError(t, json.Unmarshal(output[0], &message))
	require.Equal(t, "message", message.Type)
	require.Equal(t, "assistant", message.Role)
	require.Len(t, message.Content, 1)
	require.Equal(t, "Working.", message.Content[0].Text)
	for i, want := range expected {
		require.JSONEq(t, want, string(output[i+1]), "output item %d", i+1)
	}
	search := string(output[len(output)-1])
	require.Contains(t, search, `"limit":8`)
	require.NotContains(t, search, `8.0`, "Codex parses limit as an integer")
}

// TestResponseFallbackCodexToolsHTTPNonStream relays a synthetic Codex request
// carrying namespaces, a custom grammar tool, client tool search and discovery
// history through the OpenAI-compatible Chat and the Anthropic Claude fallback
// routes over loopback TLS. It proves the upstream receives flattened tools
// and lowered history, the upstream's flattened tool calls render back as the
// original Responses item kinds, and ordinary token billing settles.
func TestResponseFallbackCodexToolsHTTPNonStream(t *testing.T) {
	const balance = int64(1_000_000)
	for _, channel := range []int{channeltype.OpenAICompatible, channeltype.Anthropic} {
		t.Run(channeltype.IdToName(channel), func(t *testing.T) {
			c, recorder, requestID, got, _ := codexFallbackRun(t, channel, false, balance)
			requireCodexFallbackUpstream(t, channel, got)

			require.Contains(t, recorder.Header().Get("Content-Type"), "application/json")
			var reply struct {
				Status string            `json:"status"`
				Output []json.RawMessage `json:"output"`
				Usage  struct {
					Input  int `json:"input_tokens"`
					Output int `json:"output_tokens"`
					Total  int `json:"total_tokens"`
				} `json:"usage"`
			}
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &reply))
			require.Equal(t, "completed", reply.Status)
			requireCodexFallbackOutput(t, channel, reply.Output)
			require.Equal(t, 11, reply.Usage.Input)
			require.Equal(t, 7, reply.Usage.Output)
			require.Equal(t, 18, reply.Usage.Total)
			requireFallbackTokenSettlement(t, c, requestID, balance, got.User, got.Token)
		})
	}
}
