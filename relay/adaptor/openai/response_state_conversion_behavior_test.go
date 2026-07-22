package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestResponseStateConversionBehaviorConversationIsRepresented verifies that the
// typed request retains both supported Responses conversation selector shapes and
// canonicalizes them to the same conversation ID. Closes B01.
func TestResponseStateConversionBehaviorConversationIsRepresented(t *testing.T) {
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

			// Both selector shapes canonicalize to the same conversation ID (A02).
			require.NotNil(t, request.Conversation)
			require.Equal(t, "conv_123", request.Conversation.ConversationID())

			typedJSON, err := json.Marshal(&request)
			require.NoError(t, err)

			var typed map[string]any
			require.NoError(t, json.Unmarshal(typedJSON, &typed))
			require.Contains(t, typed, "conversation")
		})
	}
}

// TestResponseAPIToChatConverterIsPureLowering documents that
// ConvertResponseAPIToChatCompletionRequest is a pure lowering primitive: it
// lowers exactly the items it is given and does not itself resolve state
// selectors. State resolution is the hydrator's responsibility, which runs before
// this converter (see relay/controller TestHydratePreviousResponseResolvesPriorContext,
// closing B02). This keeps the converter a single-turn lowering step.
func TestResponseAPIToChatConverterIsPureLowering(t *testing.T) {
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

// TestResponseAPIToChatConverterOrphanToolOutputDowngrades documents intentional
// primitive behavior: a function_call_output with no matching preceding
// function_call is an orphan (e.g. trimmed history), and is downgraded to a user
// message to avoid emitting an invalid `tool`-after-nothing sequence upstream.
// When the matching call IS hydrated the link is preserved end-to-end (see
// relay/controller TestHydratePriorToolCallLink, closing B04).
func TestResponseAPIToChatConverterOrphanToolOutputDowngrades(t *testing.T) {
	t.Parallel()

	request := &ResponseAPIRequest{
		Model: "gpt-5",
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

// Note: the former TestResponseStateConversionBehaviorTypedStateItemsDegrade
// (B05) is replaced by relay/controller TestHydrateResolvesItemReferenceAndDropsReasoning,
// which proves reasoning items are dropped and item_reference items are resolved
// by the hydrator before they ever reach this converter, so no empty message is
// produced.

// TestResponseStateConversionBehaviorResponseRoundTripPreservesOpaqueState
// verifies that decoding a native response into the DTO and encoding it again
// preserves the fields required for faithful manual replay: store, conversation,
// reasoning encrypted_content, and message phase. Closes B06.
func TestResponseStateConversionBehaviorResponseRoundTripPreservesOpaqueState(t *testing.T) {
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
	require.NotNil(t, response.Store)
	require.False(t, *response.Store)
	require.NotNil(t, response.Conversation)
	require.Equal(t, "conv_123", response.Conversation.ConversationID())

	roundTrip, err := json.Marshal(&response)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(roundTrip, &decoded))
	require.Contains(t, decoded, "store")
	require.Equal(t, false, decoded["store"])
	require.Contains(t, decoded, "conversation")

	output, ok := decoded["output"].([]any)
	require.True(t, ok)
	require.Len(t, output, 2)
	reasoning, ok := output[0].(map[string]any)
	require.True(t, ok)
	require.Contains(t, reasoning, "encrypted_content")
	require.Equal(t, "opaque-state", reasoning["encrypted_content"])
	message, ok := output[1].(map[string]any)
	require.True(t, ok)
	require.Contains(t, message, "phase")
	require.Equal(t, "final_answer", message["phase"])
}
