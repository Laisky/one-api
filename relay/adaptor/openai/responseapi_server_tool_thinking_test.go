package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestConvertResponseAPIDropsThinkingForServerToolResponses verifies that
// Claude thinking is not replayed for a model response that also ran provider
// web searches: fallback cannot replay the server_tool_use and
// web_search_tool_result blocks, and Claude rejects replayed thinking whose
// assistant message was modified. Responses without searches keep thinking.
func TestConvertResponseAPIDropsThinkingForServerToolResponses(t *testing.T) {
	t.Parallel()
	searched, err := model.EncodeClaudeThinkingEnvelope(model.ClaudeThinkingBlock{Type: "thinking", Signature: "sig_search"})
	require.NoError(t, err)
	plain, err := model.EncodeClaudeThinkingEnvelope(model.ClaudeThinkingBlock{Type: "thinking", Signature: "sig_plain"})
	require.NoError(t, err)
	input := []any{
		map[string]any{"type": "message", "role": "user", "content": "search it"},
		map[string]any{"type": "reasoning", "id": "rs_1", "summary": []any{map[string]any{"type": "summary_text", "text": "planning a search"}}, "encrypted_content": searched},
		map[string]any{"type": "web_search_call", "id": "ws_1", "status": "completed", "action": map[string]any{"type": "search", "query": "go release"}},
		map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "go1.27.2"}}},
		map[string]any{"type": "message", "role": "user", "content": "now list files"},
		map[string]any{"type": "reasoning", "id": "rs_2", "summary": []any{}, "encrypted_content": plain},
		map[string]any{"type": "function_call", "id": "call_ls", "call_id": "call_ls", "name": "exec_command", "arguments": `{"cmd":"ls"}`},
		map[string]any{"type": "function_call_output", "call_id": "call_ls", "output": "a.txt"},
	}
	raw, err := json.Marshal(map[string]any{"model": "claude-opus-5-5", "input": input, "tools": []any{map[string]any{"type": "function", "name": "exec_command", "parameters": map[string]any{"type": "object"}}}})
	require.NoError(t, err)
	var request ResponseAPIRequest
	require.NoError(t, json.Unmarshal(raw, &request))

	converted, err := ConvertResponseAPIToChatCompletionRequest(&request)
	require.NoError(t, err)
	var assistants []model.Message
	for _, message := range converted.Messages {
		if message.Role == "assistant" {
			assistants = append(assistants, message)
		}
	}
	require.Len(t, assistants, 2)
	require.Contains(t, assistants[0].StringContent(), "[Web search: go release]", "the model must still know this answer came from a web search")
	require.Contains(t, assistants[0].StringContent(), "go1.27.2")
	require.NotContains(t, assistants[1].StringContent(), "Web search", "responses without searches are not annotated")
	require.Empty(t, assistants[0].ClaudeThinkingBlocks, "thinking from a response with provider searches must not be replayed")
	require.Nil(t, assistants[0].ReasoningContent, "its summary must not be replayed either, or the signature cache could re-sign it")
	require.Len(t, assistants[1].ClaudeThinkingBlocks, 1, "thinking from a response without searches is still replayed")
	require.Equal(t, "sig_plain", assistants[1].ClaudeThinkingBlocks[0].Signature)
}

// TestDescribeResponseWebSearchCall verifies the bounded search note phrasing
// for each web_search_call action kind.
func TestDescribeResponseWebSearchCall(t *testing.T) {
	t.Parallel()
	cases := []struct {
		action map[string]any
		want   string
	}{
		{map[string]any{"type": "search", "query": "go release"}, "go release"},
		{map[string]any{"type": "search", "queries": []any{"a", "b"}}, "a; b"},
		{map[string]any{"type": "open_page", "url": "https://go.dev"}, "opened https://go.dev"},
		{map[string]any{"type": "find_in_page", "url": "https://go.dev", "pattern": "1.27"}, "searched https://go.dev for 1.27"},
		{map[string]any{"type": "search"}, "query not recorded"},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, describeResponseWebSearchCall(map[string]any{"type": "web_search_call", "action": tc.action}))
	}
	require.Equal(t, "query not recorded", describeResponseWebSearchCall(map[string]any{"type": "web_search_call"}))
}
