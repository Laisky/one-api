package openai

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// fallbackCustomInputSchema is the exact Chat parameter schema every custom
// tool must expose on fallback: one required raw string argument "input".
const fallbackCustomInputSchema = `{"type":"object","properties":{"input":{"type":"string","description":"The complete raw input for the tool, in the format the tool description requires."}},"required":["input"]}`

// fallbackApplyPatchGrammar is a synthetic Lark grammar shaped like Codex's
// apply_patch definition.
const fallbackApplyPatchGrammar = "start: begin_patch hunk+ end_patch\nbegin_patch: \"*** Begin Patch\" LF\nend_patch: \"*** End Patch\" LF?\n"

// fallbackDecodeTool decodes one Responses tool definition through the real
// wire decoder. Parameters: t owns assertions and raw is the tool JSON.
// Returns the decoded tool.
func fallbackDecodeTool(t *testing.T, raw string) ResponseAPITool {
	t.Helper()
	var tool ResponseAPITool
	require.NoError(t, json.Unmarshal([]byte(raw), &tool))
	return tool
}

// fallbackJSON encodes value for JSON equality assertions. Parameters: t owns
// assertions and value is any JSON-encodable value. Returns the JSON text.
func fallbackJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return string(encoded)
}

// TestResponseCustomToolToChatGrammarSchema proves a Codex-shaped grammar
// custom tool becomes a function tool with the single required string argument
// "input" and a description carrying the original text, the gateway note and
// the full grammar definition.
func TestResponseCustomToolToChatGrammarSchema(t *testing.T) {
	t.Parallel()
	definition, err := json.Marshal(fallbackApplyPatchGrammar)
	require.NoError(t, err)
	tool := fallbackDecodeTool(t, `{"type":"custom","name":"apply_patch","description":"Use the apply_patch tool to edit files. This is a FREEFORM tool.","format":{"type":"grammar","syntax":"lark","definition":`+string(definition)+`}}`)
	chat := responseCustomToolToChat(tool)
	require.NotNil(t, chat)
	require.Equal(t, "function", chat.Type)
	require.Equal(t, "apply_patch", chat.Function.Name)
	require.JSONEq(t, fallbackCustomInputSchema, fallbackJSON(t, chat.Function.Parameters))
	require.Nil(t, chat.Function.Strict)
	want := "Use the apply_patch tool to edit files. This is a FREEFORM tool.\n\n" + responseCustomToolGatewayNote +
		"\n\nThe input must match this lark grammar exactly:\n" + strings.TrimSpace(fallbackApplyPatchGrammar)
	require.Equal(t, want, chat.Function.Description)
	require.Contains(t, chat.Function.Description, "start: begin_patch hunk+ end_patch\n")
}

// TestResponseCustomToolToChatFormats proves each custom format renders the
// expected description: text and missing formats add no grammar sentence, a
// grammar without syntax is called "formal", and malformed formats are ignored.
func TestResponseCustomToolToChatFormats(t *testing.T) {
	t.Parallel()
	base := "Original description.\n\n" + responseCustomToolGatewayNote
	cases := map[string]struct {
		tool, description string
	}{
		"text":              {tool: `{"type":"custom","name":"note","description":"Original description.","format":{"type":"text"}}`, description: base},
		"missing":           {tool: `{"type":"custom","name":"note","description":"Original description."}`, description: base},
		"grammar_no_syntax": {tool: `{"type":"custom","name":"note","description":"Original description.","format":{"type":"grammar","definition":"start: WORD"}}`, description: base + "\n\nThe input must match this formal grammar exactly:\nstart: WORD"},
		"regex":             {tool: `{"type":"custom","name":"note","description":"Original description.","format":{"type":"grammar","syntax":"regex","definition":"^[a-z]+$"}}`, description: base + "\n\nThe input must match this regex grammar exactly:\n^[a-z]+$"},
		"grammar_empty":     {tool: `{"type":"custom","name":"note","description":"Original description.","format":{"type":"grammar","syntax":"lark","definition":"  "}}`, description: base},
		"malformed_format":  {tool: `{"type":"custom","name":"note","description":"Original description.","format":"grammar"}`, description: base},
		"no_description":    {tool: `{"type":"custom","name":"note","format":{"type":"text"}}`, description: responseCustomToolGatewayNote},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			chat := responseCustomToolToChat(fallbackDecodeTool(t, tc.tool))
			require.NotNil(t, chat)
			require.Equal(t, "note", chat.Function.Name)
			require.Equal(t, tc.description, chat.Function.Description)
			require.JSONEq(t, fallbackCustomInputSchema, fallbackJSON(t, chat.Function.Parameters))
		})
	}
	require.Nil(t, responseCustomToolToChat(ResponseAPITool{Type: "custom"}), "unnamed custom tools are skipped")
}

// TestResponseFallbackCatalogCustomToolBinding proves custom tools, at the
// root and inside a namespace, register the custom kind with the input schema.
func TestResponseFallbackCatalogCustomToolBinding(t *testing.T) {
	t.Parallel()
	catalog := fallbackToolCatalog(t, `{"model":"fixture","input":"hi","tools":[
		{"type":"custom","name":"apply_patch","description":"Patch files","format":{"type":"grammar","syntax":"lark","definition":"start: WORD"}},
		{"type":"namespace","name":"mcp__tools","tools":[{"type":"custom","name":"patch","format":{"type":"text"}}]}
	]}`)
	require.Equal(t, []string{"apply_patch", "mcp__tools__patch"}, fallbackToolNames(catalog))
	for _, tool := range catalog.Tools() {
		binding, ok := catalog.Binding(tool.Function.Name)
		require.True(t, ok)
		require.Equal(t, ResponseFallbackToolCustom, binding.Kind)
		require.JSONEq(t, fallbackCustomInputSchema, fallbackJSON(t, tool.Function.Parameters))
	}
	binding, _ := catalog.Binding("mcp__tools__patch")
	require.Equal(t, ResponseFallbackToolBinding{FlatName: "mcp__tools__patch", Namespace: "mcp__tools", Name: "patch", Kind: ResponseFallbackToolCustom}, binding)
}

// TestResponseFunctionToolToChatSanitizes proves function definitions keep the
// legacy fallback sanitation: strict dropped, schema keywords upstreams reject
// removed, and a missing or blank type defaulted to "object".
func TestResponseFunctionToolToChatSanitizes(t *testing.T) {
	t.Parallel()
	chat := responseFunctionToolToChat(fallbackDecodeTool(t, `{"type":"function","name":"exec_command","description":"Run a command","strict":true,"parameters":{"$schema":"http://json-schema.org/draft-07/schema#","properties":{"cmd":{"type":"string"}},"required":["cmd"],"additionalProperties":false}}`))
	require.NotNil(t, chat)
	require.Equal(t, "exec_command", chat.Function.Name)
	require.Equal(t, "Run a command", chat.Function.Description)
	require.Nil(t, chat.Function.Strict)
	require.JSONEq(t, `{"type":"object","properties":{"cmd":{"type":"string"}},"required":["cmd"]}`, fallbackJSON(t, chat.Function.Parameters))

	chat = responseFunctionToolToChat(fallbackDecodeTool(t, `{"type":"function","name":"noop"}`))
	require.NotNil(t, chat)
	require.JSONEq(t, `{"type":"object"}`, fallbackJSON(t, chat.Function.Parameters))

	chat = responseFunctionToolToChat(fallbackDecodeTool(t, `{"type":"function","name":"blank","parameters":{"type":" ","properties":{}}}`))
	require.NotNil(t, chat)
	require.JSONEq(t, `{"type":"object","properties":{}}`, fallbackJSON(t, chat.Function.Parameters))

	require.Nil(t, responseFunctionToolToChat(ResponseAPITool{Type: "function"}), "empty definitions are skipped")
}

// TestResponseSearchToolToChat proves a client tool_search definition becomes
// a function tool named tool_search that keeps its description and schema, and
// that an empty definition gets a default query schema and description.
func TestResponseSearchToolToChat(t *testing.T) {
	t.Parallel()
	chat := responseSearchToolToChat(fallbackDecodeTool(t, `{"type":"tool_search","execution":"client","description":"# Tool discovery\nSearch deferred tools.","parameters":{"type":"object","properties":{"query":{"type":"string"},"limit":{"type":"number"}},"required":["query"],"additionalProperties":false}}`))
	require.NotNil(t, chat)
	require.Equal(t, "function", chat.Type)
	require.Equal(t, "tool_search", chat.Function.Name)
	require.Equal(t, "# Tool discovery\nSearch deferred tools.", chat.Function.Description)
	require.JSONEq(t, `{"type":"object","properties":{"query":{"type":"string"},"limit":{"type":"number"}},"required":["query"]}`, fallbackJSON(t, chat.Function.Parameters))

	chat = responseSearchToolToChat(ResponseAPITool{Type: "tool_search", Execution: "client"})
	require.NotNil(t, chat)
	require.Equal(t, "tool_search", chat.Function.Name)
	require.Equal(t, "Search the client's deferred tool catalog. Matching tools become callable on the next turn.", chat.Function.Description)
	require.JSONEq(t, `{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`, fallbackJSON(t, chat.Function.Parameters))

	chat = responseSearchToolToChat(ResponseAPITool{Type: "tool_search", Execution: "client", Parameters: map[string]any{"properties": map[string]any{"q": map[string]any{"type": "string"}}}})
	require.NotNil(t, chat)
	require.JSONEq(t, `{"type":"object","properties":{"q":{"type":"string"}}}`, fallbackJSON(t, chat.Function.Parameters), "missing type defaults to object")
}

// TestDescribeResponseCustomToolFormat proves the grammar sentence is emitted
// only for grammar formats with a nonblank definition.
func TestDescribeResponseCustomToolFormat(t *testing.T) {
	t.Parallel()
	require.Empty(t, describeResponseCustomToolFormat(nil))
	require.Empty(t, describeResponseCustomToolFormat(json.RawMessage(`{"type":"text"}`)))
	require.Empty(t, describeResponseCustomToolFormat(json.RawMessage(`not json`)))
	require.Equal(t, "The input must match this lark grammar exactly:\nstart: A", describeResponseCustomToolFormat(json.RawMessage(`{"type":"GRAMMAR","syntax":" lark ","definition":"start: A"}`)))
}
