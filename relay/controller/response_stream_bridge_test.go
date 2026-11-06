package controller

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

// --- bridge test helpers (prefixed to avoid collisions with response_fallback_test.go) ---

func newBridgeTestContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("POST", "/v1/responses", nil)
	c.Request = req
	c.Set(ctxkey.RequestId, "test-req-001")
	common.SetEventStreamHeaders(c)
	return c, w
}

func newTestBridge(t *testing.T, c *gin.Context) *chatToResponseStreamBridge {
	t.Helper()
	meta := &metalib.Meta{ActualModelName: "gpt-4o-test"}
	request := &openai.ResponseAPIRequest{Model: "gpt-4o"}
	handler := newChatToResponseStreamBridge(c, meta, request)
	return handler.(*chatToResponseStreamBridge)
}

func bridgeTextChunk(content string) *openai_compatible.ChatCompletionsStreamResponse {
	return &openai_compatible.ChatCompletionsStreamResponse{
		Model:   "gpt-4o-test",
		Created: 1700000000,
		Choices: []openai_compatible.ChatCompletionsStreamResponseChoice{
			{Delta: model.Message{Content: content}},
		},
	}
}

func bridgeReasoningChunk(reasoning string) *openai_compatible.ChatCompletionsStreamResponse {
	return &openai_compatible.ChatCompletionsStreamResponse{
		Model:   "gpt-4o-test",
		Created: 1700000000,
		Choices: []openai_compatible.ChatCompletionsStreamResponseChoice{
			{Delta: model.Message{Reasoning: &reasoning}},
		},
	}
}

func bridgeReasoningContentChunk(content string) *openai_compatible.ChatCompletionsStreamResponse {
	return &openai_compatible.ChatCompletionsStreamResponse{
		Choices: []openai_compatible.ChatCompletionsStreamResponseChoice{
			{Delta: model.Message{ReasoningContent: &content}},
		},
	}
}

func bridgeThinkingChunk(thinking string) *openai_compatible.ChatCompletionsStreamResponse {
	return &openai_compatible.ChatCompletionsStreamResponse{
		Choices: []openai_compatible.ChatCompletionsStreamResponseChoice{
			{Delta: model.Message{Thinking: &thinking}},
		},
	}
}

func bridgeFinishChunk(reason string) *openai_compatible.ChatCompletionsStreamResponse {
	return &openai_compatible.ChatCompletionsStreamResponse{
		Choices: []openai_compatible.ChatCompletionsStreamResponseChoice{
			{FinishReason: &reason},
		},
	}
}

func bridgeToolCallChunk(id string, index int, name string, args string) *openai_compatible.ChatCompletionsStreamResponse {
	idx := index
	var fn *model.Function
	if name != "" || args != "" {
		fn = &model.Function{}
		if name != "" {
			fn.Name = name
		}
		if args != "" {
			fn.Arguments = args
		}
	}
	return &openai_compatible.ChatCompletionsStreamResponse{
		Choices: []openai_compatible.ChatCompletionsStreamResponseChoice{
			{
				Delta: model.Message{
					ToolCalls: []model.Tool{
						{Id: id, Index: &idx, Function: fn},
					},
				},
			},
		},
	}
}

type bridgeSSE struct {
	event string
	data  json.RawMessage
}

func parseBridgeSSE(body string) []bridgeSSE {
	var events []bridgeSSE
	blocks := strings.Split(body, "\n\n")
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		var eventType, data string
		for _, line := range strings.Split(block, "\n") {
			if strings.HasPrefix(line, "event: ") {
				eventType = strings.TrimPrefix(line, "event: ")
			} else if strings.HasPrefix(line, "data: ") {
				data = strings.TrimPrefix(line, "data: ")
			}
		}
		if data != "" {
			events = append(events, bridgeSSE{event: eventType, data: json.RawMessage(data)})
		}
	}
	return events
}

func bridgeFindEvents(events []bridgeSSE, eventType string) []bridgeSSE {
	var result []bridgeSSE
	for _, e := range events {
		if e.event == eventType {
			result = append(result, e)
		}
	}
	return result
}

func bridgeUnmarshal(t *testing.T, e bridgeSSE, target any) {
	t.Helper()
	require.NoError(t, json.Unmarshal(e.data, target))
}

// --- Tests ---

func TestChatToResponseStreamBridge_TextStreaming(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	ok, done := bridge.HandleChunk(c, bridgeTextChunk("Hello"))
	assert.True(t, ok)
	assert.False(t, done)

	ok, done = bridge.HandleChunk(c, bridgeTextChunk(" world"))
	assert.True(t, ok)
	assert.False(t, done)

	bridge.HandleDone(c)

	events := parseBridgeSSE(w.Body.String())
	require.NotEmpty(t, events)

	assert.Len(t, bridgeFindEvents(events, "response.created"), 1)
	assert.GreaterOrEqual(t, len(bridgeFindEvents(events, "response.output_item.added")), 1)
	assert.Len(t, bridgeFindEvents(events, "response.content_part.added"), 1)

	deltas := bridgeFindEvents(events, "response.output_text.delta")
	assert.Len(t, deltas, 2)

	var ev openai.ResponseAPIStreamEvent
	bridgeUnmarshal(t, deltas[0], &ev)
	var s string
	require.NoError(t, json.Unmarshal(ev.Delta, &s))
	assert.Equal(t, "Hello", s)

	bridgeUnmarshal(t, deltas[1], &ev)
	require.NoError(t, json.Unmarshal(ev.Delta, &s))
	assert.Equal(t, " world", s)

	textDone := bridgeFindEvents(events, "response.output_text.done")
	assert.Len(t, textDone, 1)
	bridgeUnmarshal(t, textDone[0], &ev)
	assert.Equal(t, "Hello world", ev.Text)

	assert.Len(t, bridgeFindEvents(events, "response.completed"), 1)
}

func TestChatToResponseStreamBridge_ReasoningStreaming(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	bridge.HandleChunk(c, bridgeReasoningChunk("Let me think"))
	bridge.HandleChunk(c, bridgeReasoningChunk(" about this"))
	bridge.HandleDone(c)

	events := parseBridgeSSE(w.Body.String())

	assert.Len(t, bridgeFindEvents(events, "response.reasoning_summary_part.added"), 1)

	deltas := bridgeFindEvents(events, "response.reasoning_summary_text.delta")
	assert.Len(t, deltas, 2)

	var ev openai.ResponseAPIStreamEvent
	bridgeUnmarshal(t, deltas[0], &ev)
	var s string
	require.NoError(t, json.Unmarshal(ev.Delta, &s))
	assert.Equal(t, "Let me think", s)

	assert.Len(t, bridgeFindEvents(events, "response.reasoning_summary_text.done"), 1)
}

func TestChatToResponseStreamBridge_ReasoningContentField(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	bridge.HandleChunk(c, bridgeReasoningContentChunk("Deep thought"))
	bridge.HandleDone(c)

	events := parseBridgeSSE(w.Body.String())
	assert.Len(t, bridgeFindEvents(events, "response.reasoning_summary_part.added"), 1)

	deltas := bridgeFindEvents(events, "response.reasoning_summary_text.delta")
	assert.Len(t, deltas, 1)

	var ev openai.ResponseAPIStreamEvent
	bridgeUnmarshal(t, deltas[0], &ev)
	var s string
	require.NoError(t, json.Unmarshal(ev.Delta, &s))
	assert.Equal(t, "Deep thought", s)

	completed := bridgeFindEvents(events, "response.completed")
	require.Len(t, completed, 1)
	bridgeUnmarshal(t, completed[0], &ev)
	require.NotNil(t, ev.Response)
	var reasoningItem *openai.OutputItem
	for idx := range ev.Response.Output {
		if ev.Response.Output[idx].Type == "reasoning" {
			reasoningItem = &ev.Response.Output[idx]
			break
		}
	}
	require.NotNil(t, reasoningItem)
	require.Equal(t, []openai.OutputContent{{Type: "text", Text: "Deep thought"}}, reasoningItem.Content,
		"fallback output must retain replayable reasoning content for the next tool turn")
}

func TestChatToResponseStreamBridge_ThinkingField(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	bridge.HandleChunk(c, bridgeThinkingChunk("Thinking hard"))
	bridge.HandleDone(c)

	events := parseBridgeSSE(w.Body.String())
	assert.Len(t, bridgeFindEvents(events, "response.reasoning_summary_part.added"), 1)
	assert.Len(t, bridgeFindEvents(events, "response.reasoning_summary_text.delta"), 1)
}

func TestChatToResponseStreamBridge_ToolCallStreaming(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	bridge.HandleChunk(c, bridgeToolCallChunk("call_abc123", 0, "get_weather", `{"loc`))
	bridge.HandleChunk(c, bridgeToolCallChunk("call_abc123", 0, "", `ation":"NYC"}`))
	bridge.HandleChunk(c, bridgeFinishChunk("stop"))
	bridge.HandleDone(c)

	events := parseBridgeSSE(w.Body.String())

	assert.GreaterOrEqual(t, len(bridgeFindEvents(events, "response.output_item.added")), 2)
	assert.Len(t, bridgeFindEvents(events, "response.function_call_arguments.delta"), 2)

	argDone := bridgeFindEvents(events, "response.function_call_arguments.done")
	assert.Len(t, argDone, 1)
	var ev openai.ResponseAPIStreamEvent
	bridgeUnmarshal(t, argDone[0], &ev)
	assert.Equal(t, `{"location":"NYC"}`, ev.Arguments)

	assert.GreaterOrEqual(t, len(bridgeFindEvents(events, "response.required_action.added")), 1)
	assert.Len(t, bridgeFindEvents(events, "response.required_action.done"), 1)
}

func TestChatToResponseStreamBridge_MultipleToolCalls(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	bridge.HandleChunk(c, bridgeToolCallChunk("call_first", 0, "get_weather", `{"city":"NYC"}`))
	bridge.HandleChunk(c, bridgeToolCallChunk("call_second", 1, "get_time", `{"tz":"EST"}`))
	bridge.HandleDone(c)

	events := parseBridgeSSE(w.Body.String())

	argDone := bridgeFindEvents(events, "response.function_call_arguments.done")
	assert.Len(t, argDone, 2)

	var ev1, ev2 openai.ResponseAPIStreamEvent
	bridgeUnmarshal(t, argDone[0], &ev1)
	bridgeUnmarshal(t, argDone[1], &ev2)
	assert.Equal(t, "call_first", ev1.ItemId)
	assert.Equal(t, "call_second", ev2.ItemId)
	assert.Equal(t, `{"city":"NYC"}`, ev1.Arguments)
	assert.Equal(t, `{"tz":"EST"}`, ev2.Arguments)
}

// TestChatToResponseStreamBridge_ToolCallArgsDoneBeforeOutputItemDone is the
// regression test for the OneAPI agent-loop event ordering bug. Downstream
// consumers (e.g. go-ramjet's agentx oneAPI adapter) rely on the Responses
// API spec contract:
//
//	response.output_item.added         (function_call, arguments empty)
//	response.function_call_arguments.delta * N
//	response.function_call_arguments.done   (final arguments)
//	response.output_item.done               (final arguments, identical)
//
// In particular, output_item.done for a function_call MUST be emitted AFTER
// the matching function_call_arguments.done and MUST carry the complete
// buffered arguments JSON. Failing this invariant ships empty/partial args
// to downstream tool dispatchers ("args: {" instead of "args: {\"q\":...}").
//
// The test feeds a realistic chunked tool-call stream and asserts (1) the
// per-call ordering, (2) that no function_call output_item.done arrives
// before its matching args.done in the global wire order, and (3) that the
// tool-call output_item.done carries the full buffered arguments.
func TestChatToResponseStreamBridge_ToolCallArgsDoneBeforeOutputItemDone(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	bridge.HandleChunk(c, bridgeToolCallChunk("call_b", 0, "web_search", `{"q":`))
	bridge.HandleChunk(c, bridgeToolCallChunk("call_b", 0, "", `"weather"}`))
	bridge.HandleChunk(c, bridgeFinishChunk("tool_calls"))
	bridge.HandleDone(c)

	events := parseBridgeSSE(w.Body.String())

	// 1. Locate the function_call_arguments.added/delta/done and the matching
	//    output_item.done for call_b. The added event corresponds to the
	//    function_call output_item.added (with empty arguments), the delta
	//    events stream the JSON in fragments, and the done event carries the
	//    full arguments.
	var (
		toolAddedIdx     = -1
		toolDeltaIndices []int
		argsDoneIdx      = -1
		toolDoneIdx      = -1
	)
	for i, e := range events {
		switch e.event {
		case "response.output_item.added":
			var ev openai.ResponseAPIStreamEvent
			bridgeUnmarshal(t, e, &ev)
			if ev.Item != nil && ev.Item.Type == "function_call" {
				toolAddedIdx = i
			}
		case "response.function_call_arguments.delta":
			toolDeltaIndices = append(toolDeltaIndices, i)
		case "response.function_call_arguments.done":
			argsDoneIdx = i
		case "response.output_item.done":
			var ev openai.ResponseAPIStreamEvent
			bridgeUnmarshal(t, e, &ev)
			if ev.Item != nil && ev.Item.Type == "function_call" {
				toolDoneIdx = i
			}
		}
	}

	require.NotEqual(t, -1, toolAddedIdx, "expected a function_call output_item.added event")
	require.NotEmpty(t, toolDeltaIndices, "expected at least one function_call_arguments.delta")
	require.NotEqual(t, -1, argsDoneIdx, "expected a function_call_arguments.done event")
	require.NotEqual(t, -1, toolDoneIdx, "expected a function_call output_item.done event")

	// 2. Per-call spec ordering: added < delta(s) < args.done < output_item.done.
	assert.Less(t, toolAddedIdx, toolDeltaIndices[0], "added must precede delta events")
	for _, di := range toolDeltaIndices {
		assert.Less(t, di, argsDoneIdx, "delta events must precede args.done")
	}
	assert.Less(t, argsDoneIdx, toolDoneIdx,
		"function_call_arguments.done must be emitted BEFORE output_item.done for the same call_id; "+
			"downstream agent loops fire tool dispatch on output_item.done with the args buffered "+
			"to that point — emitting output_item.done first ships empty/partial args")

	// 3. output_item.done for the function_call carries the complete arguments JSON.
	var doneEv openai.ResponseAPIStreamEvent
	bridgeUnmarshal(t, events[toolDoneIdx], &doneEv)
	require.NotNil(t, doneEv.Item)
	assert.Equal(t, "function_call", doneEv.Item.Type)
	assert.Equal(t, `{"q":"weather"}`, doneEv.Item.Arguments,
		"output_item.done.Item.Arguments must hold the complete buffered JSON, not a partial state")

	// 4. The matching args.done event also carries the complete arguments JSON.
	var argsDoneEv openai.ResponseAPIStreamEvent
	bridgeUnmarshal(t, events[argsDoneIdx], &argsDoneEv)
	assert.Equal(t, `{"q":"weather"}`, argsDoneEv.Arguments)
	assert.Equal(t, "call_b", argsDoneEv.ItemId)
}

// TestChatToResponseStreamBridge_ToolCallAddedCarriesCallID verifies the
// function_call output_item.added event already carries call_id. Responses
// clients such as pi build the tool call identity from call_id on the added
// event; without it every replayed call_id is "undefined" and DeepSeek rejects
// the duplicate tool_call_id on the next turn.
func TestChatToResponseStreamBridge_ToolCallAddedCarriesCallID(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	bridge.HandleChunk(c, bridgeToolCallChunk("call_a", 0, "read_file", `{"path":"a"}`))
	bridge.HandleChunk(c, bridgeToolCallChunk("call_b", 1, "read_file", `{"path":"b"}`))
	bridge.HandleChunk(c, bridgeFinishChunk("tool_calls"))
	bridge.HandleDone(c)

	var callIDs []string
	for _, e := range bridgeFindEvents(parseBridgeSSE(w.Body.String()), "response.output_item.added") {
		var ev openai.ResponseAPIStreamEvent
		bridgeUnmarshal(t, e, &ev)
		if ev.Item != nil && ev.Item.Type == "function_call" {
			callIDs = append(callIDs, ev.Item.CallId)
		}
	}
	require.Equal(t, []string{"call_a", "call_b"}, callIDs)
}

// TestChatToResponseStreamBridge_MultiCallEventOrderingInterleaved exercises two
// interleaved tool calls in a single response and asserts that for every
// call_id, the per-call spec ordering (args.done before output_item.done) holds
// and that the calls are not interleaved such that output_item.done for call A
// fires before args.done for call A.
func TestChatToResponseStreamBridge_MultiCallEventOrderingInterleaved(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	// Two interleaved tool calls — fragments delivered in alternating chunks.
	bridge.HandleChunk(c, bridgeToolCallChunk("call_a", 0, "tool_a", `{"x":`))
	bridge.HandleChunk(c, bridgeToolCallChunk("call_b", 1, "tool_b", `{"y":`))
	bridge.HandleChunk(c, bridgeToolCallChunk("call_a", 0, "", `1}`))
	bridge.HandleChunk(c, bridgeToolCallChunk("call_b", 1, "", `2}`))
	bridge.HandleChunk(c, bridgeFinishChunk("tool_calls"))
	bridge.HandleDone(c)

	events := parseBridgeSSE(w.Body.String())

	// Track per-call event positions.
	argsDoneByCall := make(map[string]int)
	toolDoneByCall := make(map[string]int)
	for i, e := range events {
		switch e.event {
		case "response.function_call_arguments.done":
			var ev openai.ResponseAPIStreamEvent
			bridgeUnmarshal(t, e, &ev)
			argsDoneByCall[ev.ItemId] = i
		case "response.output_item.done":
			var ev openai.ResponseAPIStreamEvent
			bridgeUnmarshal(t, e, &ev)
			if ev.Item != nil && ev.Item.Type == "function_call" {
				toolDoneByCall[ev.Item.Id] = i
			}
		}
	}

	require.Contains(t, argsDoneByCall, "call_a")
	require.Contains(t, argsDoneByCall, "call_b")
	require.Contains(t, toolDoneByCall, "call_a")
	require.Contains(t, toolDoneByCall, "call_b")

	// Per-call invariant: args.done strictly precedes output_item.done.
	assert.Less(t, argsDoneByCall["call_a"], toolDoneByCall["call_a"],
		"call_a: args.done must precede output_item.done")
	assert.Less(t, argsDoneByCall["call_b"], toolDoneByCall["call_b"],
		"call_b: args.done must precede output_item.done")

	// Cross-call invariant: neither call's output_item.done may precede the
	// other call's args.done such that a stream consumer using a single
	// arg-buffer-per-call accumulator would fire with the wrong call's args.
	// In the bridge's HandleDone the tool-call terminals are emitted
	// per-call, so we expect: call_a.args.done, call_a.output_item.done,
	// call_b.args.done, call_b.output_item.done.
	assert.Less(t, toolDoneByCall["call_a"], argsDoneByCall["call_b"],
		"call_a's output_item.done should be emitted before call_b's args.done (interleave-safety)")

	// Sanity check on arg content per call.
	var aDone, bDone openai.ResponseAPIStreamEvent
	bridgeUnmarshal(t, events[argsDoneByCall["call_a"]], &aDone)
	bridgeUnmarshal(t, events[argsDoneByCall["call_b"]], &bDone)
	assert.Equal(t, `{"x":1}`, aDone.Arguments)
	assert.Equal(t, `{"y":2}`, bDone.Arguments)

	var aItem, bItem openai.ResponseAPIStreamEvent
	bridgeUnmarshal(t, events[toolDoneByCall["call_a"]], &aItem)
	bridgeUnmarshal(t, events[toolDoneByCall["call_b"]], &bItem)
	require.NotNil(t, aItem.Item)
	require.NotNil(t, bItem.Item)
	assert.Equal(t, `{"x":1}`, aItem.Item.Arguments)
	assert.Equal(t, `{"y":2}`, bItem.Item.Arguments)
}

// TestChatToResponseStreamBridge_ToolCallWithEmptyArguments handles the legit
// arg-less tool case: a function_call whose arguments are empty must still
// produce well-ordered terminals (args.done with arguments="" then
// output_item.done with item.arguments=""). The bridge must not panic.
func TestChatToResponseStreamBridge_ToolCallWithEmptyArguments(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	// Tool call where the function takes no arguments — only the name+id arrive.
	idx := 0
	bridge.HandleChunk(c, &openai_compatible.ChatCompletionsStreamResponse{
		Choices: []openai_compatible.ChatCompletionsStreamResponseChoice{
			{
				Delta: model.Message{
					ToolCalls: []model.Tool{
						{Id: "call_noop", Index: &idx, Function: &model.Function{Name: "noop"}},
					},
				},
			},
		},
	})
	bridge.HandleChunk(c, bridgeFinishChunk("tool_calls"))
	bridge.HandleDone(c)

	events := parseBridgeSSE(w.Body.String())

	// args.done event must exist even for empty arguments.
	argsDone := bridgeFindEvents(events, "response.function_call_arguments.done")
	require.Len(t, argsDone, 1)
	var argsDoneEv openai.ResponseAPIStreamEvent
	bridgeUnmarshal(t, argsDone[0], &argsDoneEv)
	assert.Equal(t, "call_noop", argsDoneEv.ItemId)
	assert.Equal(t, "", argsDoneEv.Arguments)

	// function_call output_item.done must follow and carry the same arguments.
	argsDoneIdx, toolDoneIdx := -1, -1
	for i, e := range events {
		switch e.event {
		case "response.function_call_arguments.done":
			argsDoneIdx = i
		case "response.output_item.done":
			var ev openai.ResponseAPIStreamEvent
			bridgeUnmarshal(t, e, &ev)
			if ev.Item != nil && ev.Item.Type == "function_call" {
				toolDoneIdx = i
			}
		}
	}
	require.NotEqual(t, -1, argsDoneIdx)
	require.NotEqual(t, -1, toolDoneIdx)
	assert.Less(t, argsDoneIdx, toolDoneIdx)

	var doneEv openai.ResponseAPIStreamEvent
	bridgeUnmarshal(t, events[toolDoneIdx], &doneEv)
	require.NotNil(t, doneEv.Item)
	assert.Equal(t, "function_call", doneEv.Item.Type)
	assert.Equal(t, "", doneEv.Item.Arguments)
	assert.Equal(t, "noop", doneEv.Item.Name)
}

// TestChatToResponseStreamBridge_ToolCallsPrecedeMessageOutputItemDone asserts
// that all function_call terminals (args.done + output_item.done) are flushed
// BEFORE the message item's output_item.done. This protects defensive
// downstream agent parsers that watch *any* output_item.done event as a tool
// dispatch trigger — previously, the message item.done fired first while tool
// calls were still pending args.done, shipping empty args downstream.
func TestChatToResponseStreamBridge_ToolCallsPrecedeMessageOutputItemDone(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	// Mix text + tool call so we exercise both message and function_call items.
	bridge.HandleChunk(c, bridgeTextChunk("hi"))
	bridge.HandleChunk(c, bridgeToolCallChunk("call_mix", 0, "do_thing", `{"k":"v"}`))
	bridge.HandleChunk(c, bridgeFinishChunk("tool_calls"))
	bridge.HandleDone(c)

	events := parseBridgeSSE(w.Body.String())

	toolDoneIdx, msgDoneIdx, argsDoneIdx := -1, -1, -1
	for i, e := range events {
		switch e.event {
		case "response.function_call_arguments.done":
			argsDoneIdx = i
		case "response.output_item.done":
			var ev openai.ResponseAPIStreamEvent
			bridgeUnmarshal(t, e, &ev)
			if ev.Item == nil {
				continue
			}
			switch ev.Item.Type {
			case "function_call":
				toolDoneIdx = i
			case "message":
				msgDoneIdx = i
			}
		}
	}

	require.NotEqual(t, -1, argsDoneIdx, "args.done must be emitted")
	require.NotEqual(t, -1, toolDoneIdx, "function_call output_item.done must be emitted")
	require.NotEqual(t, -1, msgDoneIdx, "message output_item.done must be emitted")

	assert.Less(t, argsDoneIdx, toolDoneIdx, "args.done must precede function_call output_item.done")
	assert.Less(t, toolDoneIdx, msgDoneIdx,
		"function_call terminals must be flushed BEFORE the message item's output_item.done so "+
			"naive downstream parsers that trigger on any output_item.done observe complete args")
}

func TestChatToResponseStreamBridge_HandleDoneTerminalEvents(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	bridge.HandleChunk(c, bridgeTextChunk("result"))
	bridge.HandleDone(c)

	events := parseBridgeSSE(w.Body.String())

	var terminalTypes []string
	for _, e := range events {
		switch e.event {
		case "response.output_text.done",
			"response.content_part.done",
			"response.output_item.done",
			"response.completed":
			terminalTypes = append(terminalTypes, e.event)
		case "":
			if strings.Contains(string(e.data), "[DONE]") {
				terminalTypes = append(terminalTypes, "[DONE]")
			}
		}
	}

	assert.Contains(t, terminalTypes, "response.output_text.done")
	assert.Contains(t, terminalTypes, "response.content_part.done")
	assert.Contains(t, terminalTypes, "response.output_item.done")
	assert.Contains(t, terminalTypes, "response.completed")

	completed := bridgeFindEvents(events, "response.completed")
	require.Len(t, completed, 1)
	var ev openai.ResponseAPIStreamEvent
	bridgeUnmarshal(t, completed[0], &ev)
	require.NotNil(t, ev.Response)
	assert.Equal(t, "completed", ev.Response.Status)
	assert.NotEmpty(t, ev.Response.Output)
}

func TestChatToResponseStreamBridge_FinalStatus_Stop(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	bridge.HandleChunk(c, bridgeTextChunk("done"))
	bridge.HandleChunk(c, bridgeFinishChunk("stop"))
	bridge.HandleDone(c)

	events := parseBridgeSSE(w.Body.String())
	completed := bridgeFindEvents(events, "response.completed")
	require.Len(t, completed, 1)
	var ev openai.ResponseAPIStreamEvent
	bridgeUnmarshal(t, completed[0], &ev)
	assert.Equal(t, "completed", ev.Response.Status)
	assert.Nil(t, ev.Response.IncompleteDetails)
}

func TestChatToResponseStreamBridge_FinalStatus_Length(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	bridge.HandleChunk(c, bridgeTextChunk("truncated"))
	bridge.HandleChunk(c, bridgeFinishChunk("length"))
	bridge.HandleDone(c)

	events := parseBridgeSSE(w.Body.String())
	completed := bridgeFindEvents(events, "response.completed")
	require.Len(t, completed, 1)
	var ev openai.ResponseAPIStreamEvent
	bridgeUnmarshal(t, completed[0], &ev)
	assert.Equal(t, "incomplete", ev.Response.Status)
	require.NotNil(t, ev.Response.IncompleteDetails)
	assert.Equal(t, "max_output_tokens", ev.Response.IncompleteDetails.Reason)
}

func TestChatToResponseStreamBridge_FinalStatus_Cancelled(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	bridge.HandleChunk(c, bridgeTextChunk("partial"))
	bridge.HandleChunk(c, bridgeFinishChunk("cancelled"))
	bridge.HandleDone(c)

	events := parseBridgeSSE(w.Body.String())
	completed := bridgeFindEvents(events, "response.completed")
	require.Len(t, completed, 1)
	var ev openai.ResponseAPIStreamEvent
	bridgeUnmarshal(t, completed[0], &ev)
	assert.Equal(t, "cancelled", ev.Response.Status)
	assert.Nil(t, ev.Response.IncompleteDetails)
}

func TestChatToResponseStreamBridge_FinalStatus_Empty(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	bridge.HandleChunk(c, bridgeTextChunk("ok"))
	bridge.HandleDone(c)

	events := parseBridgeSSE(w.Body.String())
	completed := bridgeFindEvents(events, "response.completed")
	require.Len(t, completed, 1)
	var ev openai.ResponseAPIStreamEvent
	bridgeUnmarshal(t, completed[0], &ev)
	assert.Equal(t, "completed", ev.Response.Status)
	assert.Nil(t, ev.Response.IncompleteDetails)
}

func TestChatToResponseStreamBridge_UsageForwarded(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	usageChunk := &openai_compatible.ChatCompletionsStreamResponse{
		Choices: []openai_compatible.ChatCompletionsStreamResponseChoice{
			{Delta: model.Message{Content: "hi"}},
		},
		Usage: &model.Usage{
			PromptTokens:     10,
			CompletionTokens: 20,
			TotalTokens:      30,
		},
	}
	bridge.HandleChunk(c, usageChunk)
	bridge.HandleDone(c)

	events := parseBridgeSSE(w.Body.String())
	completed := bridgeFindEvents(events, "response.completed")
	require.Len(t, completed, 1)
	var ev openai.ResponseAPIStreamEvent
	bridgeUnmarshal(t, completed[0], &ev)
	require.NotNil(t, ev.Response)
	require.NotNil(t, ev.Response.Usage)
	assert.Equal(t, 10, ev.Response.Usage.InputTokens)
	assert.Equal(t, 20, ev.Response.Usage.OutputTokens)
	assert.Equal(t, 30, ev.Response.Usage.TotalTokens)
}
