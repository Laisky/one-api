package controller

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/model"
)

// TestChatToResponseStreamBridge_SingleChunkSearchOrder verifies that a whole
// Claude message rendered as one chunk (a non-streaming upstream bridged to
// SSE) emits its items in the JSON path's order: the leading thinking, then the
// provider-executed web searches, then the thinking and call that followed.
func TestChatToResponseStreamBridge_SingleChunkSearchOrder(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	bridge.HandleChunk(c, &openai_compatible.ChatCompletionsStreamResponse{
		Choices: []openai_compatible.ChatCompletionsStreamResponseChoice{{Delta: model.Message{
			ClaudeThinkingBlocks: []model.ClaudeThinkingBlock{
				{Type: "thinking", Signature: "sig_lead"},
				{Type: "thinking", Signature: "sig_after", ToolCallsBefore: 1},
			},
			WebSearchCalls: []model.WebSearchCall{{ID: "srvtoolu_1", Query: "go release", Status: model.WebSearchCallCompleted}},
			ToolCalls: []model.Tool{
				{Id: "call_a", Type: "function", Function: &model.Function{Name: "shell", Arguments: `{"cmd":"ls"}`}},
			},
		}}},
	})
	bridge.HandleDone(c)

	var order []string
	for _, raw := range bridgeFindEvents(parseBridgeSSE(w.Body.String()), "response.output_item.added") {
		var ev openai.ResponseAPIStreamEvent
		bridgeUnmarshal(t, raw, &ev)
		require.NotNil(t, ev.Item)
		if ev.Item.Type != "message" {
			order = append(order, ev.Item.Type)
		}
	}
	require.Equal(t, []string{"reasoning", "web_search_call", "function_call", "reasoning"}, order)
}
