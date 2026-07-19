package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestResponseStateConversionBehaviorConversationIsNotRepresented verifies that
// the typed request used by fallback conversion cannot retain either supported
// Responses conversation selector shape.
func TestResponseStateConversionBehaviorConversationIsNotRepresented(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		conversation string
	}{
		{name: "conversation id", conversation: `"conv_123"`},
		{name: "conversation object", conversation: `{"id":"conv_123"}`},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			raw := []byte(`{"model":"gpt-5","conversation":` + tt.conversation + `,"input":"continue"}`)
			var request ResponseAPIRequest
			require.NoError(t, json.Unmarshal(raw, &request))

			typedJSON, err := json.Marshal(&request)
			require.NoError(t, err)

			var typed map[string]any
			require.NoError(t, json.Unmarshal(typedJSON, &typed))
			require.NotContains(t, typed, "conversation")
		})
	}
}

// TestResponseStateConversionBehaviorPreviousResponseContextIsNotResolved
// verifies that Chat fallback converts only the current incremental input and
// does not materialize the transcript referenced by previous_response_id.
func TestResponseStateConversionBehaviorPreviousResponseContextIsNotResolved(t *testing.T) {
	t.Parallel()

	previousResponseID := "resp_prior"
	request := &ResponseAPIRequest{
		Model:              "gpt-5",
		PreviousResponseId: &previousResponseID,
		Input: ResponseAPIInput{
			map[string]any{
				"type":    "message",
				"role":    "user",
				"content": "What did I ask before?",
			},
		},
	}

	converted, err := ConvertResponseAPIToChatCompletionRequest(request)
	require.NoError(t, err)
	require.Len(t, converted.Messages, 1)
	require.Equal(t, "user", converted.Messages[0].Role)
	require.Equal(t, "What did I ask before?", converted.Messages[0].StringContent())
}

// TestResponseStateConversionBehaviorInstructionsRemainRequestLocal verifies
// that fallback conversion does not invent prior instructions when a chained
// request omits them, matching the Responses API request-local instruction rule.
func TestResponseStateConversionBehaviorInstructionsRemainRequestLocal(t *testing.T) {
	t.Parallel()

	previousResponseID := "resp_prior"
	request := &ResponseAPIRequest{
		Model:              "gpt-5",
		PreviousResponseId: &previousResponseID,
		Input:              ResponseAPIInput{"continue"},
	}

	converted, err := ConvertResponseAPIToChatCompletionRequest(request)
	require.NoError(t, err)
	require.Len(t, converted.Messages, 1)
	require.Equal(t, "user", converted.Messages[0].Role)
	require.Equal(t, "continue", converted.Messages[0].StringContent())
}

// TestResponseStateConversionBehaviorPriorToolOutputLosesCallLink verifies that
// a tool result linked to a function call in a prior stored response is treated
// as an orphan when fallback conversion cannot resolve that prior response.
func TestResponseStateConversionBehaviorPriorToolOutputLosesCallLink(t *testing.T) {
	t.Parallel()

	previousResponseID := "resp_tool_call"
	request := &ResponseAPIRequest{
		Model:              "gpt-5",
		PreviousResponseId: &previousResponseID,
		Input: ResponseAPIInput{
			map[string]any{
				"type":    "function_call_output",
				"call_id": "call_weather",
				"output":  `{"temperature":21}`,
			},
		},
	}

	converted, err := ConvertResponseAPIToChatCompletionRequest(request)
	require.NoError(t, err)
	require.Len(t, converted.Messages, 1)
	require.Equal(t, "user", converted.Messages[0].Role)
	require.Empty(t, converted.Messages[0].ToolCallId)
	require.JSONEq(t, `{"temperature":21}`, converted.Messages[0].StringContent())
}

// TestResponseStateConversionBehaviorTypedStateItemsDegrade verifies how
// state-bearing Responses items without a Chat Completions equivalent are
// currently converted into empty user messages.
func TestResponseStateConversionBehaviorTypedStateItemsDegrade(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		item map[string]any
	}{
		{
			name: "encrypted reasoning item",
			item: map[string]any{
				"type":              "reasoning",
				"id":                "rs_123",
				"encrypted_content": "opaque-state",
				"summary": []any{
					map[string]any{"type": "summary_text", "text": "private plan"},
				},
			},
		},
		{
			name: "conversation item reference",
			item: map[string]any{
				"type": "item_reference",
				"id":   "msg_123",
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			request := &ResponseAPIRequest{
				Model: "gpt-5",
				Input: ResponseAPIInput{tt.item},
			}
			converted, err := ConvertResponseAPIToChatCompletionRequest(request)
			require.NoError(t, err)
			require.Len(t, converted.Messages, 1)
			require.Equal(t, "user", converted.Messages[0].Role)
			require.Empty(t, converted.Messages[0].StringContent())
		})
	}
}

// TestResponseStateConversionBehaviorResponseRoundTripDropsOpaqueState verifies
// that decoding a native response into the conversion DTO and encoding it again
// loses fields required for faithful manual replay.
func TestResponseStateConversionBehaviorResponseRoundTripDropsOpaqueState(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"id":"resp_123",
		"object":"response",
		"created_at":1,
		"status":"completed",
		"model":"gpt-5",
		"store":false,
		"conversation":{"id":"conv_123"},
		"output":[
			{"id":"rs_123","type":"reasoning","status":"completed","encrypted_content":"opaque-state","summary":[]},
			{"id":"msg_123","type":"message","role":"assistant","phase":"final_answer","status":"completed","content":[{"type":"output_text","text":"done"}]}
		]
	}`)

	var response ResponseAPIResponse
	require.NoError(t, json.Unmarshal(raw, &response))
	roundTrip, err := json.Marshal(&response)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(roundTrip, &decoded))
	require.NotContains(t, decoded, "store")
	require.NotContains(t, decoded, "conversation")

	output, ok := decoded["output"].([]any)
	require.True(t, ok)
	require.Len(t, output, 2)
	reasoning, ok := output[0].(map[string]any)
	require.True(t, ok)
	require.NotContains(t, reasoning, "encrypted_content")
	message, ok := output[1].(map[string]any)
	require.True(t, ok)
	require.NotContains(t, message, "phase")
}
