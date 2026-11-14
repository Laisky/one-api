package vertexai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestVertexResponsesContinuationIngress verifies Responses call/output history reaches native v1 response parts.
func TestVertexResponsesContinuationIngress(t *testing.T) {
	request := &openai.ResponseAPIRequest{Model: "gemini-3.8-flash"}
	require.NoError(t, json.Unmarshal([]byte(`[
 {"role":"user","content":"Look it up."},
 {"type":"function_call","call_id":"call_one","name":"lookup","arguments":"{\"key\":\"value\"}"},
 {"type":"function_call_output","call_id":"call_one","output":"{\"answer\":42}"}
 ]`), &request.Input))
	before, err := json.Marshal(request)
	require.NoError(t, err)
	chat, err := openai.ConvertResponseAPIToChatCompletionRequest(request, true)
	require.NoError(t, err)
	converted, err := (&Adaptor{}).ConvertRequest(vertexIngressContext("/v1/responses"), relaymode.ChatCompletions, chat)
	require.NoError(t, err)
	wire := vertexIngressWire(t, converted)
	contents := wire["contents"].([]any)
	require.Len(t, contents, 3)
	resultParts := contents[2].(map[string]any)["parts"].([]any)
	result := resultParts[0].(map[string]any)["functionResponse"].(map[string]any)
	require.Equal(t, "lookup", result["name"])
	require.Equal(t, map[string]any{"answer": float64(42)}, result["response"])
	after, err := json.Marshal(request)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after))
}

// TestVertexClaudeContinuationIngress verifies tool-result identity and explicit execution errors survive lowering.
func TestVertexClaudeContinuationIngress(t *testing.T) {
	request := &model.ClaudeRequest{Model: "gemini-3.8-flash", MaxTokens: 64, Messages: []model.ClaudeMessage{
		{Role: "user", Content: "Look it up."},
		{Role: "assistant", Content: []any{map[string]any{"type": "tool_use", "id": "call_one", "name": "lookup", "input": map[string]any{"key": "value"}}}},
		{Role: "user", Content: []any{map[string]any{"type": "tool_result", "tool_use_id": "call_one", "content": "database unavailable", "is_error": true}}},
	}}
	before, err := json.Marshal(request)
	require.NoError(t, err)
	converted, err := (&Adaptor{}).ConvertClaudeRequest(vertexIngressContext("/v1/messages"), request)
	require.NoError(t, err)
	wire := vertexIngressWire(t, converted)
	contents := wire["contents"].([]any)
	require.Len(t, contents, 3)
	resultParts := contents[2].(map[string]any)["parts"].([]any)
	result := resultParts[0].(map[string]any)["functionResponse"].(map[string]any)
	require.Equal(t, "lookup", result["name"])
	require.Equal(t, map[string]any{"error": "database unavailable"}, result["response"])
	after, err := json.Marshal(request)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after))
}
