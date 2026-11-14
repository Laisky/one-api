package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestResponseFallbackToolPairingIsIndependent verifies history validation is a separate trusted provider capability from schema fidelity.
func TestResponseFallbackToolPairingIsIndependent(t *testing.T) {
	var request ResponseAPIRequest
	require.NoError(t, json.Unmarshal([]byte(`{"model":"fixture","input":[{"type":"function_call_output","call_id":"orphan","output":"result"}]}`), &request))
	_, err := ConvertResponseAPIToChatCompletionRequestWithOptions(&request, nil, ResponseFallbackConversionOptions{RequireToolResultPairing: true})
	require.ErrorContains(t, err, "no matching pending function call")
	converted, err := ConvertResponseAPIToChatCompletionRequestWithOptions(&request, nil, ResponseFallbackConversionOptions{PreserveJSONSchema: true})
	require.NoError(t, err)
	require.Len(t, converted.Messages, 1)
	require.Equal(t, "user", converted.Messages[0].Role)
	require.Equal(t, "result", converted.Messages[0].Content)
}

// TestResponseFallbackToolPairingRetainsFIFO verifies strict pairing does not alter the established duplicate-ID disambiguation contract.
func TestResponseFallbackToolPairingRetainsFIFO(t *testing.T) {
	var request ResponseAPIRequest
	require.NoError(t, json.Unmarshal([]byte(`{"model":"fixture","input":[{"type":"function_call","call_id":"duplicate","name":"one","arguments":"{}"},{"type":"function_call","call_id":"duplicate","name":"two","arguments":"{}"},{"type":"function_call_output","call_id":"duplicate","output":"first"},{"type":"function_call_output","call_id":"duplicate","output":"second"}]}`), &request))
	converted, err := ConvertResponseAPIToChatCompletionRequestWithOptions(&request, nil, ResponseFallbackConversionOptions{RequireToolResultPairing: true})
	require.NoError(t, err)
	require.Len(t, converted.Messages, 3)
	require.Len(t, converted.Messages[0].ToolCalls, 2)
	require.NotEqual(t, converted.Messages[0].ToolCalls[0].Id, converted.Messages[0].ToolCalls[1].Id)
	require.Equal(t, converted.Messages[0].ToolCalls[0].Id, converted.Messages[1].ToolCallId)
	require.Equal(t, converted.Messages[0].ToolCalls[1].Id, converted.Messages[2].ToolCallId)
}
