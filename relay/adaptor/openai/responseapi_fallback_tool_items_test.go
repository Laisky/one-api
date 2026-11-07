package openai

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

// fallbackCodexRequest is a synthetic Codex-shaped Responses request with a
// function, a grammar custom tool, a namespace, client tool search, a hosted
// tool and history covering every lowered item kind.
const fallbackCodexRequest = `{"model":"fixture","instructions":"You are a coding agent.","reasoning":{"effort":"medium"},
"tools":[
	{"type":"function","name":"exec_command","description":"Run a shell command","strict":false,"parameters":{"type":"object","properties":{"cmd":{"type":"string"}},"required":["cmd"]}},
	{"type":"custom","name":"apply_patch","description":"Use apply_patch to edit files. This is a FREEFORM tool.","format":{"type":"grammar","syntax":"lark","definition":"start: begin_patch hunk+ end_patch"}},
	{"type":"namespace","name":"collaboration","description":"Multi-agent collaboration","tools":[
		{"type":"function","name":"spawn_agent","description":"Spawn a worker","parameters":{"type":"object","properties":{"task":{"type":"string"}}}},
		{"type":"function","name":"wait_agent","description":"Wait for a worker","parameters":{"type":"object","properties":{"id":{"type":"string"}}}}
	]},
	{"type":"tool_search","execution":"client","description":"# Tool discovery","parameters":{"type":"object","properties":{"query":{"type":"string"},"limit":{"type":"number"}},"required":["query"],"additionalProperties":false}},
	{"type":"web_search","external_web_access":true}
],
"input":[
	{"type":"message","role":"user","content":[{"type":"input_text","text":"Read the docs and patch the file."}]},
	{"type":"tool_search_call","id":"tsc_1","call_id":"call_s1","execution":"client","status":"completed","arguments":{"query": "docs", "limit": 8, "large": 9007199254740993}},
	{"type":"tool_search_output","id":"tso_1","call_id":"call_s1","status":"completed","execution":"client","tools":[{"type":"namespace","name":"mcp__docs","description":"Docs server","tools":[{"type":"function","name":"read_doc","description":"Read a document\nSecond line","defer_loading":true,"parameters":{"type":"object","properties":{"path":{"type":"string"}}}}]}]},
	{"type":"function_call","id":"call_x","call_id":"call_x","namespace":"mcp__docs","name":"read_doc","arguments":"{\"path\":\"a\"}"},
	{"type":"function_call_output","id":"fco_1","call_id":"call_x","output":"text"},
	{"type":"custom_tool_call","id":"call_p","call_id":"call_p","name":"apply_patch","input":"*** Begin Patch\n*** End Patch\n"},
	{"type":"custom_tool_call_output","call_id":"call_p","output":"Success"},
	{"type":"agent_message","author":"/root","recipient":"/root/worker","content":[{"type":"input_text","text":"hi"}]},
	{"type":"configuration_update","reasoning":{"effort":"xhigh"}}
]}`

// fallbackLower decodes raw, builds its catalog and lowers its input.
// Parameters: t owns assertions and raw is the request JSON. Returns the
// lowered input, the effort override and the lowering error.
func fallbackLower(t *testing.T, raw string) (ResponseAPIInput, string, error) {
	t.Helper()
	request := fallbackToolRequest(t, raw)
	catalog, err := BuildResponseFallbackToolCatalog(request)
	require.NoError(t, err)
	return lowerResponseFallbackToolItems(request.Input, catalog)
}

// fallbackLowerItem lowers a request whose input holds exactly one item and
// returns the single lowered item as a map. Parameters: t owns assertions,
// tools is the root tools JSON array and item is the input item JSON.
func fallbackLowerItem(t *testing.T, tools, item string) map[string]any {
	t.Helper()
	lowered, effort, err := fallbackLower(t, `{"model":"fixture","tools":`+tools+`,"input":[`+item+`]}`)
	require.NoError(t, err)
	require.Empty(t, effort)
	require.Len(t, lowered, 1)
	got, ok := lowered[0].(map[string]any)
	require.True(t, ok, "lowered item must be a JSON object map, got %T", lowered[0])
	return got
}

// TestLowerResponseFallbackToolItemsFunctionCalls proves namespaced function
// calls get their flattened name, default-namespace calls keep their plain
// name, and outputs drop namespace and name.
func TestLowerResponseFallbackToolItemsFunctionCalls(t *testing.T) {
	t.Parallel()
	tools := `[{"type":"namespace","name":"mcp__docs","tools":[{"type":"function","name":"read_doc"}]}]`
	got := fallbackLowerItem(t, tools, `{"type":"function_call","id":"call_x","call_id":"call_x","namespace":"mcp__docs","name":"read_doc","arguments":"{\"path\":\"a\"}","status":"completed"}`)
	require.Equal(t, map[string]any{"type": "function_call", "id": "call_x", "call_id": "call_x", "name": "mcp__docs__read_doc", "arguments": `{"path":"a"}`, "status": "completed"}, got)

	got = fallbackLowerItem(t, `[]`, `{"type":"function_call","call_id":"call_d","namespace":"functions","name":"exec_command","arguments":"{}"}`)
	require.Equal(t, map[string]any{"type": "function_call", "call_id": "call_d", "name": "exec_command", "arguments": "{}"}, got)

	got = fallbackLowerItem(t, `[]`, `{"type":"function_call","call_id":"call_u","namespace":"undeclared","name":"lookup","arguments":"{}"}`)
	require.Equal(t, "undeclared__lookup", got["name"], "history-only namespaces flatten deterministically")
	require.NotContains(t, got, "namespace")

	got = fallbackLowerItem(t, `[]`, `{"type":"function_call","call_id":"call_plain","name":"lookup","arguments":"{}"}`)
	require.Equal(t, map[string]any{"type": "function_call", "call_id": "call_plain", "name": "lookup", "arguments": "{}"}, got)

	got = fallbackLowerItem(t, `[]`, `{"type":"function_call_output","id":"fco_1","call_id":"call_x","namespace":"mcp__docs","name":"read_doc","output":"text"}`)
	require.Equal(t, map[string]any{"type": "function_call_output", "id": "fco_1", "call_id": "call_x", "output": "text"}, got)

	got = fallbackLowerItem(t, `[]`, `{"type":"custom_tool_call_output","call_id":"call_p","name":"apply_patch","output":"Success"}`)
	require.Equal(t, map[string]any{"type": "function_call_output", "call_id": "call_p", "output": "Success"}, got)
}

// TestLowerResponseFallbackToolItemsCustomToolCall proves custom_tool_call
// history becomes a function_call whose arguments wrap the raw input as
// {"input": ...}, keeping identifiers and flattening namespaced custom tools.
func TestLowerResponseFallbackToolItemsCustomToolCall(t *testing.T) {
	t.Parallel()
	patch := "*** Begin Patch\n*** Add File: a.txt\n+hello \"quoted\" <tag>\n*** End Patch\n"
	encodedPatch, err := json.Marshal(patch)
	require.NoError(t, err)
	got := fallbackLowerItem(t, `[{"type":"custom","name":"apply_patch"}]`, `{"type":"custom_tool_call","id":"call_p","call_id":"call_p","status":"completed","name":"apply_patch","input":`+string(encodedPatch)+`}`)
	require.Equal(t, "function_call", got["type"])
	require.Equal(t, "apply_patch", got["name"])
	require.Equal(t, "call_p", got["id"])
	require.Equal(t, "call_p", got["call_id"])
	require.Equal(t, "completed", got["status"])
	require.NotContains(t, got, "input")
	arguments, ok := got["arguments"].(string)
	require.True(t, ok)
	var envelope map[string]string
	require.NoError(t, json.Unmarshal([]byte(arguments), &envelope))
	require.Equal(t, map[string]string{"input": patch}, envelope)
	require.Equal(t, patch, ResponseCustomToolInput(arguments), "render reverses the lowering")

	got = fallbackLowerItem(t, `[{"type":"namespace","name":"mcp__tools","tools":[{"type":"custom","name":"patch"}]}]`, `{"type":"custom_tool_call","call_id":"call_q","namespace":"mcp__tools","name":"patch","input":"raw"}`)
	require.Equal(t, "mcp__tools__patch", got["name"])
	require.Equal(t, `{"input":"raw"}`, got["arguments"])
	require.NotContains(t, got, "namespace")
}

// TestLowerResponseFallbackToolItemsDiscovery proves raw-preserved discovery
// items lower to a tool_search function call that keeps exact numeric
// literals, a tool result naming the loaded flattened tools, and nothing for
// additional_tools.
func TestLowerResponseFallbackToolItemsDiscovery(t *testing.T) {
	t.Parallel()
	request := fallbackToolRequest(t, fallbackCodexRequest)
	for _, index := range []int{1, 2} {
		require.IsType(t, json.RawMessage{}, request.Input[index], "discovery items stay raw JSON")
	}
	catalog, err := BuildResponseFallbackToolCatalog(request)
	require.NoError(t, err)
	lowered, effort, err := lowerResponseFallbackToolItems(request.Input, catalog)
	require.NoError(t, err)
	require.Equal(t, "xhigh", effort)
	require.Len(t, lowered, len(request.Input)-1, "configuration_update is consumed")

	call, ok := lowered[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "function_call", call["type"])
	require.Equal(t, "tool_search", call["name"])
	require.Equal(t, "tsc_1", call["id"])
	require.Equal(t, "call_s1", call["call_id"])
	require.Equal(t, "completed", call["status"])
	arguments, ok := call["arguments"].(string)
	require.True(t, ok)
	require.JSONEq(t, `{"query":"docs","limit":8,"large":9007199254740993}`, arguments)
	require.Contains(t, arguments, `"large":9007199254740993`, "integer literals beyond float64 precision survive lowering")
	require.Contains(t, arguments, `"limit":8`)

	output, ok := lowered[2].(map[string]any)
	require.True(t, ok)
	require.Equal(t, map[string]any{
		"type": "function_call_output", "id": "tso_1", "call_id": "call_s1",
		"output": "Tool search loaded 1 tool(s); they are now available to call:\n- mcp__docs__read_doc: Read a document",
	}, output)

	lowered, _, err = fallbackLower(t, `{"model":"fixture","input":[
		{"type":"additional_tools","role":"developer","tools":[{"type":"function","name":"lookup"}]},
		{"type":"tool_search_call","call_id":"call_s2","execution":"client"},
		{"type":"tool_search_output","call_id":"call_s2","execution":"client","tools":[]}
	]}`)
	require.NoError(t, err)
	require.Len(t, lowered, 2, "additional_tools is carried by the catalog, not the transcript")
	call, ok = lowered[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, map[string]any{"type": "function_call", "name": "tool_search", "call_id": "call_s2", "arguments": "{}"}, call)
	output, ok = lowered[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "Tool search found no matching tools.", output["output"])
}

// TestSummarizeResponseLoadedTools proves the lowered tool_search_output text
// lists every loaded flattened tool with a bounded first description line.
func TestSummarizeResponseLoadedTools(t *testing.T) {
	t.Parallel()
	long := "a" + strings.Repeat("é", 100)
	tools := []ResponseAPITool{
		fallbackDecodeTool(t, `{"type":"namespace","name":"mcp__codex_apps__github","tools":[{"type":"function","name":"_create_issue","description":"\n\n  Create an issue  \nmore"}]}`),
		fallbackDecodeTool(t, `{"type":"custom","name":"patch"}`),
		{Type: "function", Name: "long", Description: long},
		{Type: "function"},
		{Type: "web_search"},
	}
	summary := summarizeResponseLoadedTools(tools, nil)
	lines := strings.Split(summary, "\n")
	require.Len(t, lines, 4)
	require.Equal(t, "Tool search loaded 3 tool(s); they are now available to call:", lines[0])
	require.Equal(t, "- mcp__codex_apps__github_create_issue: Create an issue", lines[1])
	require.Equal(t, "- patch", lines[2])
	require.True(t, strings.HasPrefix(lines[3], "- long: a"))
	require.True(t, strings.HasSuffix(lines[3], "…"))
	require.True(t, utf8.ValidString(lines[3]), "truncation must not split a UTF-8 sequence")
	require.LessOrEqual(t, len(strings.TrimSuffix(strings.TrimPrefix(lines[3], "- long: "), "…")), responseFallbackToolSummaryLimit)
	require.Equal(t, "Tool search found no matching tools.", summarizeResponseLoadedTools(nil, nil))
}

// TestLowerResponseFallbackToolItemsAgentMessage proves agent_message items
// become attributed user messages, and encrypted or non-text content fails.
func TestLowerResponseFallbackToolItemsAgentMessage(t *testing.T) {
	t.Parallel()
	got := fallbackLowerItem(t, `[]`, `{"type":"agent_message","author":"/root","recipient":"/root/worker","content":[{"type":"input_text","text":"hi"}]}`)
	require.Equal(t, map[string]any{
		"type": "message", "role": "user",
		"content": []any{map[string]any{"type": "input_text", "text": "[Agent message from /root to /root/worker]\nhi"}},
	}, got)

	got = fallbackLowerItem(t, `[]`, `{"type":"agent_message","content":[{"type":"output_text","text":"one"},{"type":"text","text":"two"}]}`)
	require.Equal(t, "[Agent message]\none\ntwo", got["content"].([]any)[0].(map[string]any)["text"])

	for name, content := range map[string]string{
		"encrypted": `[{"type":"encrypted_content","encrypted_content":"opaque-agent-canary"}]`,
		"image":     `[{"type":"input_image","image_url":"https://example.invalid/a.png"}]`,
		"scalar":    `["bare string"]`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			lowered, _, err := fallbackLower(t, `{"model":"fixture","input":[{"type":"agent_message","author":"/root","content":`+content+`}]}`)
			require.ErrorIs(t, err, ErrResponseFallbackNotRepresentable)
			require.Nil(t, lowered)
			require.NotContains(t, err.Error(), "opaque-agent-canary", "errors must not echo opaque content")
		})
	}
}

// TestLowerResponseFallbackToolItemsConfigurationUpdate proves the last valid
// configuration_update effort wins, the items are consumed, and an unknown
// effort fails explicitly.
func TestLowerResponseFallbackToolItemsConfigurationUpdate(t *testing.T) {
	t.Parallel()
	lowered, effort, err := fallbackLower(t, `{"model":"fixture","input":[
		{"type":"configuration_update","reasoning":{"effort":"low"}},
		{"type":"message","role":"user","content":"hi"},
		{"type":"configuration_update","reasoning":{"effort":"  XHIGH "}},
		{"type":"configuration_update","reasoning":{"effort":""}},
		{"type":"configuration_update","model":"other"}
	]}`)
	require.NoError(t, err)
	require.Equal(t, "xhigh", effort)
	require.Len(t, lowered, 1)

	for _, value := range []string{"none", "default", "minimal", "low", "medium", "high", "xhigh", "max", "ultra"} {
		require.True(t, validResponseReasoningEffort(value), value)
	}
	lowered, effort, err = fallbackLower(t, `{"model":"fixture","input":[{"type":"configuration_update","reasoning":{"effort":"turbo"}}]}`)
	require.ErrorIs(t, err, ErrResponseFallbackNotRepresentable)
	require.Nil(t, lowered)
	require.Empty(t, effort)
}

// TestLowerResponseFallbackToolItemsDoesNotMutateInput proves lowering never
// rewrites the caller's input items in place and passes other items through.
func TestLowerResponseFallbackToolItemsDoesNotMutateInput(t *testing.T) {
	t.Parallel()
	request := fallbackToolRequest(t, fallbackCodexRequest)
	before, err := json.Marshal(request.Input)
	require.NoError(t, err)
	catalog, err := BuildResponseFallbackToolCatalog(request)
	require.NoError(t, err)
	lowered, _, err := lowerResponseFallbackToolItems(request.Input, catalog)
	require.NoError(t, err)
	after, err := json.Marshal(request.Input)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after))
	require.Equal(t, request.Input[0], lowered[0], "ordinary messages pass through")

	empty, effort, err := lowerResponseFallbackToolItems(nil, catalog)
	require.NoError(t, err)
	require.Nil(t, empty)
	require.Empty(t, effort)
	text := ResponseAPIInput{"hello"}
	lowered, _, err = lowerResponseFallbackToolItems(text, catalog)
	require.NoError(t, err)
	require.Equal(t, text, lowered)
}

// TestConvertResponseAPIToChatCompletionRequestCodexFallback proves the full
// converter emits the flattened tool list and a valid Chat transcript for a
// Codex-shaped request: discovery, namespaced and custom calls with paired
// tool results, the attributed agent message and the effort override.
func TestConvertResponseAPIToChatCompletionRequestCodexFallback(t *testing.T) {
	t.Parallel()
	request := fallbackToolRequest(t, fallbackCodexRequest)
	converted, err := ConvertResponseAPIToChatCompletionRequest(request)
	require.NoError(t, err)

	names := make([]string, 0, len(converted.Tools))
	for _, tool := range converted.Tools {
		require.Equal(t, "function", tool.Type)
		names = append(names, tool.Function.Name)
	}
	require.Equal(t, []string{"exec_command", "apply_patch", "collaboration__spawn_agent", "collaboration__wait_agent", "tool_search", "mcp__docs__read_doc"}, names,
		"hosted web_search stays out of the local catalog")
	require.Equal(t, "[Multi-agent collaboration] Spawn a worker", converted.Tools[2].Function.Description)
	require.Equal(t, "[Docs server] Read a document\nSecond line", converted.Tools[5].Function.Description)
	require.JSONEq(t, fallbackCustomInputSchema, fallbackJSON(t, converted.Tools[1].Function.Parameters))

	type wantCall struct{ id, name, arguments string }
	type wantMessage struct {
		role, content, toolCallID string
		calls                     []wantCall
	}
	want := []wantMessage{
		{role: "system", content: "You are a coding agent."},
		{role: "user", content: "Read the docs and patch the file."},
		{role: "assistant", calls: []wantCall{{id: "s1", name: "tool_search", arguments: `{"query":"docs","limit":8,"large":9007199254740993}`}}},
		{role: "tool", toolCallID: "s1", content: "Tool search loaded 1 tool(s); they are now available to call:\n- mcp__docs__read_doc: Read a document"},
		{role: "assistant", calls: []wantCall{{id: "x", name: "mcp__docs__read_doc", arguments: `{"path":"a"}`}}},
		{role: "tool", toolCallID: "x", content: "text"},
		{role: "assistant", calls: []wantCall{{id: "p", name: "apply_patch", arguments: `{"input":"*** Begin Patch\n*** End Patch\n"}`}}},
		{role: "tool", toolCallID: "p", content: "Success"},
		{role: "user", content: "[Agent message from /root to /root/worker]\nhi"},
	}
	require.Len(t, converted.Messages, len(want))
	for i, expected := range want {
		message := converted.Messages[i]
		require.Equal(t, expected.role, message.Role, "message %d", i)
		require.Equal(t, expected.toolCallID, message.ToolCallId, "message %d", i)
		require.Len(t, message.ToolCalls, len(expected.calls), "message %d", i)
		for j, call := range expected.calls {
			got := message.ToolCalls[j]
			require.Equal(t, call.id, got.Id, "message %d call %d", i, j)
			require.Equal(t, call.name, got.Function.Name, "message %d call %d", i, j)
			arguments, ok := got.Function.Arguments.(string)
			require.True(t, ok, "message %d call %d arguments type %T", i, j, got.Function.Arguments)
			require.JSONEq(t, call.arguments, arguments)
			if call.name == "tool_search" {
				require.Contains(t, arguments, "9007199254740993")
			}
		}
		if len(expected.calls) == 0 {
			require.Equal(t, expected.content, message.StringContent(), "message %d", i)
		}
	}

	require.NotNil(t, converted.Reasoning)
	require.NotNil(t, converted.Reasoning.Effort)
	require.Equal(t, "xhigh", *converted.Reasoning.Effort, "configuration_update overrides the request effort")
	require.Equal(t, "medium", *request.Reasoning.Effort, "the caller's reasoning is not mutated")
}

// TestConvertResponseAPIToChatCompletionRequestFallbackErrors proves the
// converter surfaces unrepresentable catalogs and history as wrapped
// ErrResponseFallbackNotRepresentable errors.
func TestConvertResponseAPIToChatCompletionRequestFallbackErrors(t *testing.T) {
	t.Parallel()
	for name, raw := range map[string]string{
		"server_search":   `{"model":"fixture","input":"hi","tools":[{"type":"tool_search","execution":"server"}]}`,
		"hosted_child":    `{"model":"fixture","input":"hi","tools":[{"type":"namespace","name":"ns","tools":[{"type":"web_search"}]}]}`,
		"encrypted_agent": `{"model":"fixture","input":[{"type":"agent_message","content":[{"type":"encrypted_content","encrypted_content":"opaque"}]}]}`,
		"invalid_effort":  `{"model":"fixture","input":[{"type":"configuration_update","reasoning":{"effort":"turbo"}},{"type":"message","role":"user","content":"hi"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			converted, err := ConvertResponseAPIToChatCompletionRequest(fallbackToolRequest(t, raw))
			require.ErrorIs(t, err, ErrResponseFallbackNotRepresentable)
			require.Nil(t, converted)
		})
	}
}
