package controller

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestResponseNamespaceWirePreservesLocalTools checks native and typed-fallback
// wire bodies for both response modes without dispatching provider work.
func TestResponseNamespaceWirePreservesLocalTools(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, fallback := range []bool{false, true} {
			raw := []byte(`{"model":"gpt-4o","input":"hello","tools":[{"type":"namespace","name":"alpha","description":"Local tools","tools":[{"type":"function","name":"same","description":"Function","strict":true,"defer_loading":false,"parameters":{"type":"object","properties":{"namespace":{"type":"string"}}}},{"type":"custom","name":"patch","format":{"type":"grammar","syntax":"lark","definition":"start: WORD"}}]},{"type":"namespace","name":"beta","tools":[{"type":"function","name":"same","parameters":{"type":"object"}}]}],"tool_choice":{"type":"function","namespace":"alpha","name":"same"}}`)
			var request openai.ResponseAPIRequest
			require.NoError(t, json.Unmarshal(raw, &request))
			request.Stream = &stream
			body := raw
			if fallback {
				body = nil
			}
			wire, _, _, err := normalizeResponseAPIRawBody(body, &request, channeltype.OpenAI)
			require.NoError(t, err, "stream=%v fallback=%v", stream, fallback)
			var before, after map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(raw, &before))
			require.NoError(t, json.Unmarshal(wire, &after))
			// Function marshalling retains its historical compatibility function block;
			// compare the protocol fields rather than requiring that duplicate block.
			var tools []map[string]any
			require.NoError(t, json.Unmarshal(after["tools"], &tools))
			require.Len(t, tools, 2)
			require.Equal(t, "alpha", tools[0]["name"])
			require.Equal(t, "beta", tools[1]["name"])
			children := tools[0]["tools"].([]any)
			require.Len(t, children, 2)
			function := children[0].(map[string]any)
			require.Equal(t, true, function["strict"])
			require.Equal(t, false, function["defer_loading"])
			require.Equal(t, "same", function["name"])
			custom := children[1].(map[string]any)
			require.Equal(t, "grammar", custom["format"].(map[string]any)["type"])
			require.Equal(t, "start: WORD", custom["format"].(map[string]any)["definition"])
			require.JSONEq(t, string(before["tool_choice"]), string(after["tool_choice"]))
		}
	}
}

// TestResponseNamespaceRejectsUnsupportedChildren keeps paid and unknown tools
// behind the existing billing boundary in both wire preparation paths.
func TestResponseNamespaceRejectsUnsupportedChildren(t *testing.T) {
	for _, child := range []string{
		`{"type":"web_search"}`, `{"type":"image_generation"}`,
		`{"type":"code_interpreter"}`, `{"type":"file_search"}`,
		`{"type":"mcp"}`, `{"type":"tool_search"}`,
		`{"type":"future_paid_tool"}`, `{"type":"namespace","name":"nested","tools":[{"type":"function","name":"f"}]}`,
		`{"type":"function"}`, `{"type":"custom"}`, `{}`,
	} {
		raw := []byte(`{"model":"gpt-4o","input":"hello","tools":[{"type":"namespace","name":"local","tools":[` + child + `]}]}`)
		var request openai.ResponseAPIRequest
		if err := json.Unmarshal(raw, &request); err != nil {
			continue
		}
		for _, body := range [][]byte{raw, nil} {
			wire, _, _, err := normalizeResponseAPIRawBody(body, &request, channeltype.OpenAI)
			require.Error(t, err, child)
			require.Empty(t, wire)
		}
	}
}

// TestResponseNamespaceRejectsMalformedGroups rejects unnamed or empty wrappers
// instead of sending a partially represented native request upstream.
func TestResponseNamespaceRejectsMalformedGroups(t *testing.T) {
	for _, group := range []string{`{"type":"namespace"}`, `{"type":"namespace","name":"x","tools":[]}`, `{"type":"namespace","tools":[{"type":"function","name":"f"}]}`} {
		var request openai.ResponseAPIRequest
		require.NoError(t, json.Unmarshal([]byte(`{"tools":[`+group+`]}`), &request))
		require.Error(t, validateNativeResponseToolBilling(request.Tools))
	}
}

// TestResponseNamespaceMergeRetainsScopedExtensions verifies identity-scoped
// preservation after policy changes without resurrecting removed children.
func TestResponseNamespaceMergeRetainsScopedExtensions(t *testing.T) {
	raw := []byte(`{"model":"gpt-4o","input":"hello","tools":[{"type":"namespace","name":"alpha","tools":[{"type":"function","name":"same","vendor_scope":"alpha"},{"type":"custom","name":"patch","vendor_scope":"keep","format":{"type":"text"}},{"type":"function","name":"removed"}]},{"type":"namespace","name":"beta","tools":[{"type":"function","name":"same","vendor_scope":"beta"}]}]}`)
	var request openai.ResponseAPIRequest
	require.NoError(t, json.Unmarshal(raw, &request))
	request.Tools[0].Tools = []openai.ResponseAPITool{request.Tools[0].Tools[1], request.Tools[0].Tools[0]}
	request.Tools = []openai.ResponseAPITool{request.Tools[1], request.Tools[0]}
	wire, _, _, err := normalizeResponseAPIRawBody(raw, &request, channeltype.OpenAI)
	require.NoError(t, err)
	var out struct {
		Tools []struct {
			Name  string
			Tools []map[string]any
		}
	}
	require.NoError(t, json.Unmarshal(wire, &out))
	require.Equal(t, "beta", out.Tools[0].Name)
	require.Equal(t, "beta", out.Tools[0].Tools[0]["vendor_scope"])
	require.Equal(t, "alpha", out.Tools[1].Name)
	require.Len(t, out.Tools[1].Tools, 2)
	require.Equal(t, "keep", out.Tools[1].Tools[0]["vendor_scope"])
	require.Equal(t, "alpha", out.Tools[1].Tools[1]["vendor_scope"])
	require.NotContains(t, string(wire), "removed")
}

// TestResponseNamespaceRejectsAmbiguousToolKeys prevents a provider reading a
// different paid-tool type or child array from the values admitted by Go.
func TestResponseNamespaceRejectsAmbiguousToolKeys(t *testing.T) {
	for _, group := range []string{
		`{"type":"namespace","name":"local","tools":[{"type":"code_interpreter","Type":"function","name":"run"}]}`,
		`{"type":"namespace","name":"local","tools":[{"type":"code_interpreter","type":"function","name":"run"}]}`,
		`{"type":"namespace","name":"local","tools":[{"type":"code_interpreter","t\u0079pe":"function","name":"run"}]}`,
		`{"type":"namespace","name":"local","tools":[{"type":"code_interpreter"}],"Tools":[{"type":"function","name":"run"}]}`,
		`{"type":"namespace","name":"local","tools":[{"type":"code_interpreter"}],"tool\u017f":[{"type":"function","name":"run"}]}`,
	} {
		var request openai.ResponseAPIRequest
		require.Error(t, json.Unmarshal([]byte(`{"tools":[`+group+`]}`), &request), group)
	}
}
