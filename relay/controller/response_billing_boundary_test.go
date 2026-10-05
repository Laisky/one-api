package controller

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestNativeResponseWireBoundaryRejectsUnmeteredTools verifies that hosted
// tools cannot escape billing validation simply because their type is unknown.
func TestNativeResponseWireBoundaryRejectsUnmeteredTools(t *testing.T) {
	t.Parallel()
	for _, toolType := range []string{"image_generation", "code_interpreter", "file_search", "computer", "computer_use_preview", "mcp", "future_paid_tool"} {
		t.Run(toolType, func(t *testing.T) {
			t.Parallel()
			raw, err := json.Marshal(map[string]any{
				"model": "gpt-4o", "input": "hello",
				"tools": []map[string]any{{"type": toolType, "container": map[string]any{"type": "auto"}}},
			})
			require.NoError(t, err)
			var request openai.ResponseAPIRequest
			require.NoError(t, json.Unmarshal(raw, &request))
			for _, body := range [][]byte{raw, nil} {
				wire, _, _, err := normalizeResponseAPIRawBody(body, &request, channeltype.OpenAI)
				require.Error(t, err, "raw and typed-fallback paths must reject unmetered upstream execution")
				require.Empty(t, wire)
				require.Contains(t, err.Error(), "billing")
			}
		})
	}
}

// TestNativeResponseWireBoundaryFiltersUnknownRoots verifies that raw extension
// fields cannot bypass the controlled passthrough policy and that a conversation
// selector, which only the owner-scoped gateway store can resolve, never reaches
// the provider.
func TestNativeResponseWireBoundaryFiltersUnknownRoots(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"model":"gpt-4o","input":"hello","conversation":"conv_test","max_tool_calls":100,"vendor_extra":true}`)
	var request openai.ResponseAPIRequest
	require.NoError(t, json.Unmarshal(raw, &request))
	wire, _, changed, err := normalizeResponseAPIRawBody(raw, &request, channeltype.OpenAI)
	require.NoError(t, err)
	require.True(t, changed)
	var root map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(wire, &root))
	require.NotContains(t, root, "max_tool_calls")
	require.NotContains(t, root, "vendor_extra")
	require.NotContains(t, root, "conversation")
	require.JSONEq(t, `"hello"`, string(root["input"]))
}

// TestNativeResponseWireBoundaryDropsUnresolvedPreviousResponse verifies that a
// raw previous_response_id the typed request does not carry, and which
// resolveNativePreviousResponse therefore never owner-resolved, is removed
// instead of forwarded, while a typed parent still reaches the provider.
func TestNativeResponseWireBoundaryDropsUnresolvedPreviousResponse(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"model":"gpt-4o","input":"hello","previous_response_id":"resp_foreign_provider_handle"}`)

	var unresolved openai.ResponseAPIRequest
	require.NoError(t, json.Unmarshal([]byte(`{"model":"gpt-4o","input":"hello"}`), &unresolved))
	wire, _, changed, err := normalizeResponseAPIRawBody(raw, &unresolved, channeltype.OpenAI)
	require.NoError(t, err)
	require.True(t, changed)
	var root map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(wire, &root))
	require.NotContains(t, root, "previous_response_id")

	var resolved openai.ResponseAPIRequest
	require.NoError(t, json.Unmarshal(raw, &resolved))
	wire, _, _, err = normalizeResponseAPIRawBody(raw, &resolved, channeltype.OpenAI)
	require.NoError(t, err)
	root = nil
	require.NoError(t, json.Unmarshal(wire, &root))
	require.JSONEq(t, `"resp_foreign_provider_handle"`, string(root["previous_response_id"]))
}

// TestNativeResponseWireBoundaryPreservesLocalToolGrammar verifies that
// deny-by-default hosted-tool admission does not break locally executed custom tools.
func TestNativeResponseWireBoundaryPreservesLocalToolGrammar(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"model":"gpt-4o","input":"hello","tools":[{"type":"web_search"},{"type":"custom","name":"apply_patch","format":{"type":"grammar","syntax":"lark","definition":"start: \"ok\""}}]}`)
	var request openai.ResponseAPIRequest
	require.NoError(t, json.Unmarshal(raw, &request))
	// Simulate an earlier policy stage pruning another tool. Matching must not
	// attach its raw configuration to the surviving custom tool by array index.
	request.Tools = request.Tools[1:]
	wire, _, _, err := normalizeResponseAPIRawBody(raw, &request, channeltype.OpenAI)
	require.NoError(t, err)
	var root struct {
		Tools []map[string]json.RawMessage `json:"tools"`
	}
	require.NoError(t, json.Unmarshal(wire, &root))
	require.Len(t, root.Tools, 1)
	require.JSONEq(t, `"custom"`, string(root.Tools[0]["type"]))
	require.JSONEq(t, `{"type":"grammar","syntax":"lark","definition":"start: \"ok\""}`, string(root.Tools[0]["format"]))
}
