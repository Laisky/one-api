package openai

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// fallbackRenderCatalog builds the catalog of the synthetic Codex request plus
// a namespaced custom tool and a long hashed identity. Parameters: t owns
// assertions. Returns the catalog and the hashed long flat name.
func fallbackRenderCatalog(t *testing.T) (*ResponseFallbackToolCatalog, string) {
	t.Helper()
	request := fallbackToolRequest(t, fallbackCodexRequest)
	extra := fallbackToolRequest(t, fmt.Sprintf(`{"model":"fixture","input":"hi","tools":[
		{"type":"namespace","name":"mcp__tools","tools":[{"type":"custom","name":"patch","format":{"type":"text"}}]},
		{"type":"namespace","name":%q,"tools":[{"type":"function","name":%q}]}
	]}`, "mcp__"+strings.Repeat("n", 40), strings.Repeat("c", 40)))
	request.Tools = append(request.Tools, extra.Tools...)
	catalog, err := BuildResponseFallbackToolCatalog(request)
	require.NoError(t, err)
	long := catalog.FlatName("mcp__"+strings.Repeat("n", 40), strings.Repeat("c", 40))
	require.Regexp(t, fallbackHashedSuffixPattern, long)
	return catalog, long
}

// TestResponseFallbackToolCallItemRendersEachKind proves upstream calls to
// flattened names render back as the Responses item their definition expects:
// function_call with separate namespace and name (namespaced calls marked with
// an empty encrypted_function_args list), custom_tool_call with raw input,
// client tool_search_call with object arguments, and plain function_call for
// undeclared names or a nil catalog.
func TestResponseFallbackToolCallItemRendersEachKind(t *testing.T) {
	t.Parallel()
	catalog, long := fallbackRenderCatalog(t)
	cases := map[string]struct {
		flat, arguments string
		want            OutputItem
	}{
		"discovered_namespace_function": {flat: "mcp__docs__read_doc", arguments: `{"path":"a"}`,
			want: OutputItem{Type: "function_call", Status: "completed", Id: "call_1", CallId: "call_1", Name: "read_doc", Namespace: "mcp__docs", Arguments: `{"path":"a"}`, EncryptedFunctionArgs: &[]string{}}},
		"root_namespace_function": {flat: "collaboration__spawn_agent", arguments: `{"task":"t"}`,
			want: OutputItem{Type: "function_call", Status: "completed", Id: "call_1", CallId: "call_1", Name: "spawn_agent", Namespace: "collaboration", Arguments: `{"task":"t"}`, EncryptedFunctionArgs: &[]string{}}},
		"root_function": {flat: "exec_command", arguments: `{"cmd":"ls"}`,
			want: OutputItem{Type: "function_call", Status: "completed", Id: "call_1", CallId: "call_1", Name: "exec_command", Arguments: `{"cmd":"ls"}`}},
		"hashed_function": {flat: long, arguments: `{}`,
			want: OutputItem{Type: "function_call", Status: "completed", Id: "call_1", CallId: "call_1", Name: strings.Repeat("c", 40), Namespace: "mcp__" + strings.Repeat("n", 40), Arguments: `{}`, EncryptedFunctionArgs: &[]string{}}},
		"root_custom": {flat: "apply_patch", arguments: `{"input":"*** Begin Patch\n*** End Patch\n"}`,
			want: OutputItem{Type: "custom_tool_call", Status: "completed", Id: "call_1", CallId: "call_1", Name: "apply_patch", Input: "*** Begin Patch\n*** End Patch\n"}},
		"namespaced_custom": {flat: "mcp__tools__patch", arguments: `{"input":"raw text"}`,
			want: OutputItem{Type: "custom_tool_call", Status: "completed", Id: "call_1", CallId: "call_1", Name: "patch", Namespace: "mcp__tools", Input: "raw text"}},
		"client_tool_search": {flat: "tool_search", arguments: `{"query":"docs","limit":8.0}`,
			want: OutputItem{Type: "tool_search_call", Status: "completed", Id: "call_1", CallId: "call_1", Execution: "client", ToolSearchArguments: json.RawMessage(`{"limit":8,"query":"docs"}`)}},
		"unknown_name": {flat: "ghost_tool", arguments: `{"x":1}`,
			want: OutputItem{Type: "function_call", Status: "completed", Id: "call_1", CallId: "call_1", Name: "ghost_tool", Arguments: `{"x":1}`}},
		"unflattened_leaf": {flat: "read_doc", arguments: `{}`,
			want: OutputItem{Type: "function_call", Status: "completed", Id: "call_1", CallId: "call_1", Name: "read_doc", Arguments: `{}`}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := catalog.ResponseFallbackToolCallItem("call_1", tc.flat, tc.arguments)
			require.Equal(t, tc.want, got)
		})
	}

	var empty *ResponseFallbackToolCatalog
	require.Equal(t,
		OutputItem{Type: "function_call", Status: "completed", Id: "call_9", CallId: "call_9", Name: "apply_patch", Arguments: `{"input":"x"}`},
		empty.ResponseFallbackToolCallItem("call_9", "apply_patch", `{"input":"x"}`),
		"a nil catalog renders plain function calls")
}

// TestResponseFallbackToolCallItemJSON proves the rendered items serialize to
// the Responses wire shapes Codex parses: namespaced calls carry an explicit
// empty encrypted_function_args list while root calls omit it, and tool_search
// arguments keep integer limits and integers beyond float64 precision.
func TestResponseFallbackToolCallItemJSON(t *testing.T) {
	t.Parallel()
	catalog, _ := fallbackRenderCatalog(t)
	cases := map[string]struct {
		flat, arguments, want string
	}{
		"function": {flat: "mcp__docs__read_doc", arguments: `{"path":"a"}`,
			want: `{"type":"function_call","id":"call_1","status":"completed","call_id":"call_1","name":"read_doc","namespace":"mcp__docs","arguments":"{\"path\":\"a\"}","encrypted_function_args":[]}`},
		"root_function": {flat: "exec_command", arguments: `{"cmd":"ls"}`,
			want: `{"type":"function_call","id":"call_1","status":"completed","call_id":"call_1","name":"exec_command","arguments":"{\"cmd\":\"ls\"}"}`},
		"custom": {flat: "apply_patch", arguments: `{"input":"*** Begin Patch\n*** End Patch\n"}`,
			want: `{"type":"custom_tool_call","id":"call_1","status":"completed","call_id":"call_1","name":"apply_patch","input":"*** Begin Patch\n*** End Patch\n"}`},
		"tool_search": {flat: "tool_search", arguments: `{"query":"docs","limit":8.0,"large":9007199254740993}`,
			want: `{"type":"tool_search_call","id":"call_1","status":"completed","call_id":"call_1","execution":"client","arguments":{"query":"docs","limit":8,"large":9007199254740993}}`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			item := catalog.ResponseFallbackToolCallItem("call_1", tc.flat, tc.arguments)
			encoded, err := json.Marshal(item)
			require.NoError(t, err)
			require.JSONEq(t, tc.want, string(encoded))
			if name == "tool_search" {
				require.Contains(t, string(encoded), `"limit":8`)
				require.NotContains(t, string(encoded), `"limit":8.0`)
				require.Contains(t, string(encoded), `"large":9007199254740993`)
				require.NotContains(t, string(encoded), `"name"`, "tool_search_call carries no tool name")
				var decoded OutputItem
				require.NoError(t, json.Unmarshal(encoded, &decoded))
				require.Equal(t, "client", decoded.Execution)
				require.JSONEq(t, `{"query":"docs","limit":8,"large":9007199254740993}`, string(decoded.ToolSearchArguments))
			}
		})
	}
}

// TestNormalizeResponseToolSearchArguments proves tool_search arguments are
// always a JSON object with a string query, integral positive limits become
// integer literals while every other limit is dropped, and every other value
// keeps its exact bytes.
func TestNormalizeResponseToolSearchArguments(t *testing.T) {
	t.Parallel()
	cases := map[string]struct{ in, want string }{
		"float_limit":        {in: `{"query":"docs","limit":8.0}`, want: `{"limit":8,"query":"docs"}`},
		"integer_limit":      {in: `{"query":"docs","limit":8}`, want: `{"limit":8,"query":"docs"}`},
		"exponent_limit":     {in: `{"limit":1e2}`, want: `{"limit":100,"query":""}`},
		"negative_limit":     {in: `{"limit":-3.0}`, want: `{"query":""}`},
		"fractional_limit":   {in: `{"limit":2.5}`, want: `{"query":""}`},
		"string_limit":       {in: `{"limit":"8"}`, want: `{"limit":8,"query":""}`},
		"huge_limit":         {in: `{"limit":9007199254740993}`, want: `{"limit":9007199254740993,"query":""}`},
		"overflow_limit":     {in: `{"limit":1e300}`, want: `{"query":""}`},
		"no_limit_canonical": {in: `{"query": "x", "n": 1.50}`, want: `{"n":1.50,"query":"x"}`},
		"trimmed":            {in: "  {\"query\":\"x\"}\n", want: `{"query":"x"}`},
		"invalid_json":       {in: `not json`, want: `{"query":""}`},
		"empty":              {in: ``, want: `{"query":""}`},
		"null":               {in: `null`, want: `{"query":""}`},
		"array":              {in: `[1,2]`, want: `{"query":""}`},
		"string":             {in: `"docs"`, want: `{"query":""}`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, string(normalizeResponseToolSearchArguments(tc.in)))
		})
	}
	withLarge := string(normalizeResponseToolSearchArguments(`{"limit":4.0,"large":9007199254740993}`))
	require.JSONEq(t, `{"limit":4,"large":9007199254740993,"query":""}`, withLarge)
	require.Contains(t, withLarge, "9007199254740993", "sibling literals survive the limit rewrite")
}

// TestResponseCustomToolInput proves the raw custom input is unwrapped from
// the {"input": ...} envelope, and that malformed envelopes still deliver the
// model's text instead of dropping it.
func TestResponseCustomToolInput(t *testing.T) {
	t.Parallel()
	cases := map[string]struct{ in, want string }{
		"string_input": {in: `{"input":"x"}`, want: "x"},
		"escaped":      {in: `{"input":"line1\nline2 \"q\" <tag>"}`, want: "line1\nline2 \"q\" <tag>"},
		"extra_keys":   {in: `{"input":"x","note":"ignored"}`, want: "x"},
		"not_json":     {in: `*** Begin Patch`, want: `*** Begin Patch`},
		"missing_key":  {in: `{"other":"x"}`, want: `{"other":"x"}`},
		"number":       {in: `{"input":42}`, want: "42"},
		"object":       {in: `{"input":{"a":1}}`, want: `{"a":1}`},
		"null":         {in: `{"input":null}`, want: ""},
		"array":        {in: `[1]`, want: `[1]`},
		"empty":        {in: ``, want: ``},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, ResponseCustomToolInput(tc.in))
		})
	}
}
