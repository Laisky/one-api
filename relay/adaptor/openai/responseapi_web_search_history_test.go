package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// codexWebSearchReplayInput is a Codex-shaped history that replays hosted
// web_search_call items verbatim between assistant items of the same turns.
const codexWebSearchReplayInput = `[
	{"type":"message","role":"user","content":[{"type":"input_text","text":"latest go release?"}]},
	{"type":"web_search_call","id":"ws_1","status":"completed","action":{"type":"search","query":"go release"}},
	{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Go 1.27 is out."}]},
	{"type":"web_search_call","id":"ws_2","status":"completed","action":{"type":"open_page","url":"https://go.dev"}},
	{"type":"function_call","call_id":"call_a","name":"shell","arguments":"{\"cmd\":\"go version\"}"},
	{"type":"web_search_call","id":"ws_3","status":"failed","action":{"type":"find_in_page","url":"https://go.dev","pattern":"1.27"}},
	{"type":"function_call_output","call_id":"call_a","output":"go1.27.2"},
	{"type":"message","role":"user","content":[{"type":"input_text","text":"thanks"}]}
]`

// TestResponseFallbackAcceptsWebSearchCallHistory proves fallback admission
// accepts replayed web_search_call items and the converter drops them without
// splitting the assistant turn that surrounds them.
func TestResponseFallbackAcceptsWebSearchCallHistory(t *testing.T) {
	var input []any
	require.NoError(t, json.Unmarshal([]byte(codexWebSearchReplayInput), &input))
	require.NoError(t, ValidateResponseAPIFallbackFidelity(input, false))
	require.NoError(t, ValidateResponseAPIFallbackFidelity(input, true))

	request := &ResponseAPIRequest{
		Model: "claude-opus-4-6",
		Input: input,
		Tools: []ResponseAPITool{
			{Type: "web_search"},
			{Type: "function", Name: "shell", Parameters: map[string]any{"type": "object"}},
		},
	}
	converted, err := ConvertResponseAPIToChatCompletionRequest(request)
	require.NoError(t, err)

	roles := make([]string, 0, len(converted.Messages))
	for _, message := range converted.Messages {
		roles = append(roles, message.Role)
	}
	require.Equal(t, []string{"user", "assistant", "tool", "user"}, roles)
	// The searches themselves cannot be replayed, so the response that ran them
	// carries a deterministic note naming each action ahead of its answer.
	require.Equal(t, "[Web search: go release; opened https://go.dev; searched https://go.dev for 1.27]\nGo 1.27 is out.", converted.Messages[1].StringContent())
	require.Len(t, converted.Messages[1].ToolCalls, 1)
	require.Equal(t, "shell", converted.Messages[1].ToolCalls[0].Function.Name)
	require.Equal(t, converted.Messages[1].ToolCalls[0].Id, converted.Messages[2].ToolCallId)

	// The hosted declaration never becomes a Chat function tool: Claude server
	// search is attached by the Claude converter, other upstreams drop it.
	require.Len(t, converted.Tools, 1)
	require.Equal(t, "shell", converted.Tools[0].Function.Name)
}
