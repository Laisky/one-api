package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// loweredMetadataRequest returns a public Responses request for each replayed client-call family.
func loweredMetadataRequest(t *testing.T, kind, extra string) *ResponseAPIRequest {
	t.Helper()
	tools, call, result := "", "", ""
	switch kind {
	case "function":
		tools = `[{"type":"function","name":"save","parameters":{"type":"object"}}]`
		call = `{"type":"function_call","call_id":"call_x","name":"save","arguments":"{}"` + extra + `}`
		result = `{"type":"function_call_output","call_id":"call_x","output":"done"}`
	case "custom":
		tools = `[{"type":"custom","name":"apply_patch"}]`
		call = `{"type":"custom_tool_call","call_id":"call_x","name":"apply_patch","input":"patch"` + extra + `}`
		result = `{"type":"custom_tool_call_output","call_id":"call_x","output":"done"}`
	case "search":
		tools = `[{"type":"tool_search","execution":"client"}]`
		call = `{"type":"tool_search_call","call_id":"call_x","execution":"client","arguments":{"query":"docs"}` + extra + `}`
		result = `{"type":"tool_search_output","call_id":"call_x","execution":"client","tools":[]}`
	}
	var request ResponseAPIRequest
	require.NoError(t, json.Unmarshal([]byte(`{"model":"fixture","tools":`+tools+`,"input":[`+call+`,`+result+`]}`), &request))
	return &request
}

// TestResponseLoweredCallMetadataPreservation verifies rewritten custom/search calls retain the same typed native metadata as ordinary function calls.
func TestResponseLoweredCallMetadataPreservation(t *testing.T) {
	for _, kind := range []string{"function", "custom", "search"} {
		for _, native := range []bool{false, true} {
			t.Run(kind+map[bool]string{true: "/native", false: "/compatibility"}[native], func(t *testing.T) {
				for _, signed := range []bool{false, true} {
					extra := ""
					if signed {
						extra = `,"extra_content":{"google":{"thought_signature":"opaque-sig","unreviewed":"drop"},"oneapi":{"gemini_part_index":0},"unreviewed":"drop"}`
					}
					request := loweredMetadataRequest(t, kind, extra)
					before, err := json.Marshal(request)
					require.NoError(t, err)
					chat, err := ConvertResponseAPIToChatCompletionRequestWithOptions(request, nil, ResponseFallbackConversionOptions{NativeMedia: native})
					require.NoError(t, err)
					require.Len(t, chat.Messages, 2)
					require.Len(t, chat.Messages[0].ToolCalls, 1)
					call := chat.Messages[0].ToolCalls[0]
					if native && signed {
						require.Equal(t, "opaque-sig", model.ToolThoughtSignature(call))
						index, ok := model.GeminiPartIndex(call.ExtraContent)
						require.True(t, ok)
						require.Zero(t, index)
						encoded, err := json.Marshal(call.ExtraContent)
						require.NoError(t, err)
						require.JSONEq(t, `{"google":{"thought_signature":"opaque-sig"},"oneapi":{"gemini_part_index":0}}`, string(encoded))
					} else {
						require.Nil(t, call.ExtraContent)
					}
					after, err := json.Marshal(request)
					require.NoError(t, err)
					require.JSONEq(t, string(before), string(after))
				}
			})
		}
	}
}

// TestResponseLoweredCallMetadataValidation verifies rewrites do not bypass native typed validation while compatibility providers remain unchanged.
func TestResponseLoweredCallMetadataValidation(t *testing.T) {
	for _, kind := range []string{"function", "custom", "search"} {
		t.Run(kind, func(t *testing.T) {
			request := loweredMetadataRequest(t, kind, `,"extra_content":{"google":{"thought_signature":7}}`)
			_, err := ConvertResponseAPIToChatCompletionRequestWithOptions(request, nil, ResponseFallbackConversionOptions{NativeMedia: true})
			require.ErrorContains(t, err, "Google function call metadata")
			chat, err := ConvertResponseAPIToChatCompletionRequest(request)
			require.NoError(t, err)
			require.Nil(t, chat.Messages[0].ToolCalls[0].ExtraContent)
		})
	}
}

// TestResponseLoweredCallPositionOnly retains unsigned source positions without inventing a Google signature.
func TestResponseLoweredCallPositionOnly(t *testing.T) {
	for _, kind := range []string{"function", "custom", "search"} {
		t.Run(kind, func(t *testing.T) {
			request := loweredMetadataRequest(t, kind, `,"extra_content":{"oneapi":{"gemini_part_index":0}}`)
			chat, err := ConvertResponseAPIToChatCompletionRequest(request, true)
			require.NoError(t, err)
			call := chat.Messages[0].ToolCalls[0]
			require.Empty(t, model.ToolThoughtSignature(call))
			index, ok := model.GeminiPartIndex(call.ExtraContent)
			require.True(t, ok)
			require.Zero(t, index)
		})
	}
}
