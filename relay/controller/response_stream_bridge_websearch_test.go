package controller

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/model"
)

// bridgeDeltaChunk wraps one Chat delta in a stream chunk.
func bridgeDeltaChunk(delta model.Message) *openai_compatible.ChatCompletionsStreamResponse {
	return &openai_compatible.ChatCompletionsStreamResponse{
		Model:   "gpt-4o-test",
		Created: 1700000000,
		Choices: []openai_compatible.ChatCompletionsStreamResponseChoice{{Delta: delta}},
	}
}

// TestBridgeWebSearchOrderingWithReasoningAndToolCalls proves web_search_call
// items take output indexes in arrival order between reasoning and caller
// tool calls, close exactly once, and that the non-streaming renderer emits
// the same item order for the equivalent message.
func TestBridgeWebSearchOrderingWithReasoningAndToolCalls(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	reasoning := "thinking"
	toolIndex := 0
	bridge.HandleChunk(c, bridgeDeltaChunk(model.Message{Reasoning: &reasoning}))
	bridge.HandleChunk(c, bridgeDeltaChunk(model.Message{WebSearchCalls: []model.WebSearchCall{{ID: "srvtoolu_x", Query: "q", Status: model.WebSearchCallInProgress}}}))
	bridge.HandleChunk(c, bridgeDeltaChunk(model.Message{WebSearchCalls: []model.WebSearchCall{{ID: "srvtoolu_x", Query: "q", Status: model.WebSearchCallCompleted}}}))
	// A duplicate terminal update must not emit a second done event.
	bridge.HandleChunk(c, bridgeDeltaChunk(model.Message{WebSearchCalls: []model.WebSearchCall{{ID: "srvtoolu_x", Status: model.WebSearchCallCompleted}}}))
	bridge.HandleChunk(c, bridgeDeltaChunk(model.Message{WebSearchCalls: []model.WebSearchCall{{ID: "srvtoolu_y", Query: "never answered", Status: model.WebSearchCallInProgress}}}))
	bridge.HandleChunk(c, bridgeTextChunk("answer"))
	bridge.HandleChunk(c, bridgeDeltaChunk(model.Message{ToolCalls: []model.Tool{{Id: "call_1", Type: "function", Index: &toolIndex,
		Function: &model.Function{Name: "shell", Arguments: `{"cmd":"ls"}`}}}}))
	bridge.HandleDone(c)
	require.NoError(t, bridge.StreamWriteError())

	events := parseBridgeSSE(w.Body.String())
	doneByID := map[string]openai.ResponseAPIStreamEvent{}
	for _, event := range bridgeFindEvents(events, "response.output_item.done") {
		var decoded openai.ResponseAPIStreamEvent
		bridgeUnmarshal(t, event, &decoded)
		require.NotContains(t, doneByID, decoded.Item.Id, "each item closes once")
		doneByID[decoded.Item.Id] = decoded
	}
	require.Equal(t, "completed", doneByID["ws_x"].Item.Status)
	require.Equal(t, "failed", doneByID["ws_y"].Item.Status, "an unanswered search closes as failed")
	require.Len(t, bridgeFindEvents(events, "response.web_search_call.completed"), 1)

	var completed openai.ResponseAPIStreamEvent
	terminal := bridgeFindEvents(events, "response.completed")
	require.Len(t, terminal, 1)
	bridgeUnmarshal(t, terminal[0], &completed)
	types := make([]string, 0, len(completed.Response.Output))
	for _, item := range completed.Response.Output {
		types = append(types, item.Type+":"+item.Id)
	}
	require.Equal(t, []string{"reasoning:" + completed.Response.Output[0].Id, "web_search_call:ws_x", "web_search_call:ws_y",
		"message:" + completed.Response.Output[3].Id, "function_call:call_1"}, types, "items keep the order the model produced them")
	require.Equal(t, 1, doneByID["ws_x"].OutputIndex)
	require.Equal(t, 2, doneByID["ws_y"].OutputIndex)

	jsonOutput := buildResponseOutput(logger.Logger, []openai_compatible.TextResponseChoice{{Message: model.Message{
		Role: "assistant", Content: "answer", Reasoning: &reasoning,
		WebSearchCalls: []model.WebSearchCall{{ID: "srvtoolu_x", Query: "q", Status: model.WebSearchCallCompleted}, {ID: "srvtoolu_y", Query: "never answered", Status: model.WebSearchCallInProgress}},
		ToolCalls:      []model.Tool{{Id: "call_1", Type: "function", Function: &model.Function{Name: "shell", Arguments: `{"cmd":"ls"}`}}},
	}}}, nil)
	jsonTypes := make([]string, 0, len(jsonOutput))
	for _, item := range jsonOutput {
		jsonTypes = append(jsonTypes, item.Type+":"+item.Status)
	}
	require.Equal(t, []string{"reasoning:completed", "web_search_call:completed", "web_search_call:failed", "message:completed", "function_call:completed"}, jsonTypes,
		"JSON output uses the streaming order")
	require.Equal(t, "ws_x", jsonOutput[1].Id)
}
