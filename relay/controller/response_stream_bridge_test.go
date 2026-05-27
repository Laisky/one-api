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

func TestChatToResponseStreamBridge_HandleDoneWithoutChunks(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	ok, done := bridge.HandleDone(c)
	assert.True(t, ok)
	assert.True(t, done)

	events := parseBridgeSSE(w.Body.String())

	assert.Len(t, bridgeFindEvents(events, "response.created"), 1)

	textDone := bridgeFindEvents(events, "response.output_text.done")
	assert.Len(t, textDone, 1)
	var ev openai.ResponseAPIStreamEvent
	bridgeUnmarshal(t, textDone[0], &ev)
	assert.Equal(t, "", ev.Text)

	assert.Len(t, bridgeFindEvents(events, "response.completed"), 1)
}

func TestChatToResponseStreamBridge_SSEFormat(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	bridge.HandleChunk(c, bridgeTextChunk("x"))
	bridge.HandleDone(c)

	body := w.Body.String()
	lines := strings.Split(body, "\n")
	foundEventDataPair := false
	for i, line := range lines {
		if strings.HasPrefix(line, "event: response.created") {
			require.Greater(t, len(lines), i+1)
			assert.True(t, strings.HasPrefix(lines[i+1], "data: "), "expected data: line after event: line")
			dataStr := strings.TrimPrefix(lines[i+1], "data: ")
			assert.True(t, json.Valid([]byte(dataStr)), "data payload should be valid JSON")
			foundEventDataPair = true
			break
		}
	}
	assert.True(t, foundEventDataPair, "should find at least one event/data pair")
}

func TestChatToResponseStreamBridge_HandleUpstreamDone(t *testing.T) {
	t.Parallel()
	c, _ := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	assert.False(t, bridge.upstreamDone)

	ok, done := bridge.HandleUpstreamDone(c)
	assert.True(t, ok)
	assert.False(t, done)
	assert.True(t, bridge.upstreamDone)
}

func TestChatToResponseStreamBridge_HandleChunkAfterDone(t *testing.T) {
	t.Parallel()
	c, _ := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	bridge.HandleDone(c)

	ok, done := bridge.HandleChunk(c, bridgeTextChunk("late"))
	assert.True(t, ok)
	assert.True(t, done)

	ok, done = bridge.HandleDone(c)
	assert.True(t, ok)
	assert.True(t, done)
}

func TestChatToResponseStreamBridge_ModelOverride(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	chunk := bridgeTextChunk("hi")
	chunk.Model = "gpt-4o-mini-override"
	bridge.HandleChunk(c, chunk)
	bridge.HandleDone(c)

	events := parseBridgeSSE(w.Body.String())
	completed := bridgeFindEvents(events, "response.completed")
	require.Len(t, completed, 1)
	var ev openai.ResponseAPIStreamEvent
	bridgeUnmarshal(t, completed[0], &ev)
	assert.Equal(t, "gpt-4o-mini-override", ev.Response.Model)
}

func TestChatToResponseStreamBridge_FinalizeUsage(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	bridge.HandleChunk(c, bridgeTextChunk("hi"))
	bridge.FinalizeUsage(&model.Usage{
		PromptTokens:     5,
		CompletionTokens: 15,
		TotalTokens:      20,
	})
	bridge.HandleDone(c)

	events := parseBridgeSSE(w.Body.String())
	completed := bridgeFindEvents(events, "response.completed")
	require.Len(t, completed, 1)
	var ev openai.ResponseAPIStreamEvent
	bridgeUnmarshal(t, completed[0], &ev)
	require.NotNil(t, ev.Response.Usage)
	assert.Equal(t, 5, ev.Response.Usage.InputTokens)
	assert.Equal(t, 15, ev.Response.Usage.OutputTokens)
}

func TestChatToResponseStreamBridge_FinalizeUsageNil(t *testing.T) {
	t.Parallel()
	c, _ := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	bridge.FinalizeUsage(nil)
	assert.Nil(t, bridge.usage)
}

func TestChatToResponseStreamBridge_ReasoningNotEmittedForWhitespace(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	bridge.HandleChunk(c, bridgeReasoningChunk("   "))
	bridge.HandleChunk(c, bridgeReasoningChunk(""))
	bridge.HandleDone(c)

	events := parseBridgeSSE(w.Body.String())
	assert.Len(t, bridgeFindEvents(events, "response.reasoning_summary_part.added"), 0)
	assert.Len(t, bridgeFindEvents(events, "response.reasoning_summary_text.delta"), 0)
	assert.Len(t, bridgeFindEvents(events, "response.reasoning_summary_text.done"), 0)
}

func TestChatToResponseStreamBridge_MixedTextAndReasoning(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	bridge.HandleChunk(c, bridgeReasoningChunk("thinking"))
	bridge.HandleChunk(c, bridgeTextChunk("answer"))
	bridge.HandleDone(c)

	events := parseBridgeSSE(w.Body.String())
	assert.Len(t, bridgeFindEvents(events, "response.reasoning_summary_text.delta"), 1)
	assert.Len(t, bridgeFindEvents(events, "response.output_text.delta"), 1)

	completed := bridgeFindEvents(events, "response.completed")
	require.Len(t, completed, 1)
	var ev openai.ResponseAPIStreamEvent
	bridgeUnmarshal(t, completed[0], &ev)
	assert.GreaterOrEqual(t, len(ev.Response.Output), 2)
}

func TestChatToResponseStreamBridge_CreatedTimestampFromChunk(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	chunk := bridgeTextChunk("hi")
	chunk.Created = 1234567890
	bridge.HandleChunk(c, chunk)
	bridge.HandleDone(c)

	events := parseBridgeSSE(w.Body.String())
	completed := bridgeFindEvents(events, "response.completed")
	require.Len(t, completed, 1)
	var ev openai.ResponseAPIStreamEvent
	bridgeUnmarshal(t, completed[0], &ev)
	assert.Equal(t, int64(1234567890), ev.Response.CreatedAt)
}

func TestChatToResponseStreamBridge_ResponseIDFromContext(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	bridge.HandleChunk(c, bridgeTextChunk("hi"))
	bridge.HandleDone(c)

	events := parseBridgeSSE(w.Body.String())
	created := bridgeFindEvents(events, "response.created")
	require.Len(t, created, 1)
	var ev openai.ResponseAPIStreamEvent
	bridgeUnmarshal(t, created[0], &ev)
	assert.True(t, strings.HasPrefix(ev.Response.Id, "resp-"), "response ID should start with resp-")
}

func TestChatToResponseStreamBridge_EmitEventFormat(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	bridge.emitEvent(c, "test.event", openai.ResponseAPIStreamEvent{
		Type: "test.event",
		Text: "hello",
	})

	body := w.Body.String()
	assert.Contains(t, body, "event: test.event\n")
	assert.Contains(t, body, "data: ")
	assert.True(t, strings.HasSuffix(body, "\n\n"))

	dataLine := ""
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "data: ") {
			dataLine = strings.TrimPrefix(line, "data: ")
			break
		}
	}
	require.NotEmpty(t, dataLine)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(dataLine), &parsed))
	assert.Equal(t, "test.event", parsed["type"])
}

func TestChatToResponseStreamBridge_ToolCallWithoutFunction(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	idx := 0
	bridge.HandleChunk(c, &openai_compatible.ChatCompletionsStreamResponse{
		Choices: []openai_compatible.ChatCompletionsStreamResponseChoice{
			{
				Delta: model.Message{
					ToolCalls: []model.Tool{
						{Id: "call_nofn", Index: &idx, Function: nil},
					},
				},
			},
		},
	})
	bridge.HandleDone(c)

	events := parseBridgeSSE(w.Body.String())
	assert.Len(t, bridgeFindEvents(events, "response.completed"), 1)
}

func TestChatToResponseStreamBridge_FinalStatus_Direct(t *testing.T) {
	t.Parallel()
	tests := []struct {
		reason         string
		expectedStatus string
		hasIncomplete  bool
	}{
		{"stop", "completed", false},
		{"length", "incomplete", true},
		{"cancelled", "cancelled", false},
		{"", "completed", false},
		{"  LENGTH  ", "incomplete", true},
		{"CANCELLED", "cancelled", false},
		{"unknown_reason", "completed", false},
	}

	for _, tt := range tests {
		t.Run(tt.reason, func(t *testing.T) {
			bridge := &chatToResponseStreamBridge{}
			bridge.lastFinishReason = tt.reason
			status, incomplete := bridge.finalStatus()
			assert.Equal(t, tt.expectedStatus, status)
			if tt.hasIncomplete {
				require.NotNil(t, incomplete)
				assert.Equal(t, "max_output_tokens", incomplete.Reason)
			} else {
				assert.Nil(t, incomplete)
			}
		})
	}
}

func TestChatToResponseStreamBridge_RawMessageFromString(t *testing.T) {
	t.Parallel()

	result := rawMessageFromString("hello")
	assert.Equal(t, `"hello"`, string(result))

	result = rawMessageFromString("")
	assert.Nil(t, result)

	result = rawMessageFromString(`he said "hi"`)
	assert.NotNil(t, result)
	var decoded string
	require.NoError(t, json.Unmarshal(result, &decoded))
	assert.Equal(t, `he said "hi"`, decoded)
}

func TestChatToResponseStreamBridge_NextOutputIndex(t *testing.T) {
	t.Parallel()
	bridge := &chatToResponseStreamBridge{}
	assert.Equal(t, 0, bridge.nextOutputIndex())
	assert.Equal(t, 1, bridge.nextOutputIndex())
	assert.Equal(t, 2, bridge.nextOutputIndex())
}

func TestChatToResponseStreamBridge_ToolCallIndexLookup(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	bridge.HandleChunk(c, bridgeToolCallChunk("", 0, "my_func", `{"a"`))
	bridge.HandleChunk(c, bridgeToolCallChunk("call_resolved", 0, "", `:1}`))
	bridge.HandleDone(c)

	events := parseBridgeSSE(w.Body.String())

	argDone := bridgeFindEvents(events, "response.function_call_arguments.done")
	assert.Len(t, argDone, 1)
	var ev openai.ResponseAPIStreamEvent
	bridgeUnmarshal(t, argDone[0], &ev)
	assert.Equal(t, `{"a":1}`, ev.Arguments)
	assert.Equal(t, "call_resolved", ev.ItemId)
}

func TestChatToResponseStreamBridge_EmptyChunk(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	bridge.HandleChunk(c, &openai_compatible.ChatCompletionsStreamResponse{})
	bridge.HandleDone(c)

	events := parseBridgeSSE(w.Body.String())
	assert.Len(t, bridgeFindEvents(events, "response.completed"), 1)
}

func TestChatToResponseStreamBridge_ModelFallbackToMeta(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)

	// Create bridge with empty model in meta, so the model field stays empty
	// until buildFinalResponse falls back to meta.ActualModelName
	meta := &metalib.Meta{ActualModelName: "fallback-meta-model"}
	request := &openai.ResponseAPIRequest{Model: "fallback-request-model"}
	handler := newChatToResponseStreamBridge(c, meta, request).(*chatToResponseStreamBridge)

	// Don't send any chunk with a model name, but clear the model set during construction
	handler.model = ""

	handler.HandleDone(c)

	events := parseBridgeSSE(w.Body.String())
	completed := bridgeFindEvents(events, "response.completed")
	require.Len(t, completed, 1)
	var ev openai.ResponseAPIStreamEvent
	bridgeUnmarshal(t, completed[0], &ev)
	// Should fall back to meta.ActualModelName
	assert.Equal(t, "fallback-meta-model", ev.Response.Model)
}

func TestChatToResponseStreamBridge_ModelFallbackToRequest(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)

	// Both meta and bridge model are empty, should fall back to request.Model
	meta := &metalib.Meta{ActualModelName: ""}
	request := &openai.ResponseAPIRequest{Model: "fallback-request-model"}
	handler := newChatToResponseStreamBridge(c, meta, request).(*chatToResponseStreamBridge)

	handler.model = ""

	handler.HandleDone(c)

	events := parseBridgeSSE(w.Body.String())
	completed := bridgeFindEvents(events, "response.completed")
	require.Len(t, completed, 1)
	var ev openai.ResponseAPIStreamEvent
	bridgeUnmarshal(t, completed[0], &ev)
	assert.Equal(t, "fallback-request-model", ev.Response.Model)
}

func TestChatToResponseStreamBridge_PrefersOriginModel(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)
	meta := &metalib.Meta{OriginModelName: "public-alias", ActualModelName: "hidden-target"}
	request := &openai.ResponseAPIRequest{Model: "public-alias"}
	handler := newChatToResponseStreamBridge(c, meta, request).(*chatToResponseStreamBridge)

	chunk := bridgeTextChunk("hello")
	chunk.Model = "hidden-target"
	handler.ensureInitialized(c, chunk)

	require.Contains(t, w.Body.String(), `"model":"public-alias"`)
	require.NotContains(t, w.Body.String(), `"model":"hidden-target"`)

	handler.model = "hidden-target"
	resp := handler.buildFinalResponse("completed", nil, nil)
	require.Equal(t, "public-alias", resp.Model)
}

func TestChatToResponseStreamBridge_AppendEmptyTextDelta(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	// Send chunk with empty content - should not emit any text delta
	bridge.HandleChunk(c, bridgeTextChunk(""))
	bridge.HandleChunk(c, bridgeTextChunk("actual"))
	bridge.HandleDone(c)

	events := parseBridgeSSE(w.Body.String())
	deltas := bridgeFindEvents(events, "response.output_text.delta")
	// Only one delta (for "actual"), the empty one should be skipped
	assert.Len(t, deltas, 1)
}

func TestChatToResponseStreamBridge_StringifyArguments(t *testing.T) {
	t.Parallel()
	bridge := &chatToResponseStreamBridge{}

	assert.Equal(t, "hello", bridge.stringifyArguments("hello"))
	assert.Equal(t, "bytes", bridge.stringifyArguments([]byte("bytes")))
	assert.Equal(t, "", bridge.stringifyArguments(nil))

	result := bridge.stringifyArguments(map[string]any{"key": "value"})
	assert.Contains(t, result, `"key"`)
	assert.Contains(t, result, `"value"`)
}

// ---------------------------------------------------------------------------
// Gap-fill: upstream interruption end-to-end through bridge
// ---------------------------------------------------------------------------

// TestChatToResponseStreamBridge_UpstreamDropWithoutDone simulates the scenario
// where the upstream Chat Completions stream sends some content but never sends
// [DONE]. The bridge should still produce valid Response API terminal events
// (response.completed) and [DONE] because the Response API format always
// requires a terminal event, but the status should reflect whatever the
// finish_reason was (or "completed" if none was set, since the bridge cannot
// distinguish a clean non-[DONE] upstream from a drop).
func TestChatToResponseStreamBridge_UpstreamDropWithoutDone(t *testing.T) {
	c, w := newBridgeTestContext(t)

	bridge := newTestBridge(t, c)

	// Send some content but no finish_reason
	chunk := &openai_compatible.ChatCompletionsStreamResponse{
		Choices: []openai_compatible.ChatCompletionsStreamResponseChoice{
			{Delta: model.Message{Content: "partial content"}},
		},
	}
	handled, done := bridge.HandleChunk(c, chunk)
	assert.True(t, handled)
	assert.False(t, done)

	// DO NOT call HandleUpstreamDone — simulating upstream drop
	// Call HandleDone directly (as StreamHandler would after scanner loop exits)
	handled, done = bridge.HandleDone(c)
	assert.True(t, handled)
	assert.True(t, done)

	body := w.Body.String()
	// Terminal events should still be present (bridge generates them)
	assert.Contains(t, body, "event: response.completed")
	assert.Contains(t, body, "data: [DONE]")
	// The partial content should be in the output
	assert.Contains(t, body, "partial content")

	// Parse the response.completed event to check status
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "data: ") && strings.Contains(line, "response.completed") {
			var evt openai.ResponseAPIStreamEvent
			err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &evt)
			require.NoError(t, err)
			if evt.Response != nil {
				// Without finish_reason and without HandleUpstreamDone, status is "completed"
				// (bridge can't distinguish clean close from drop)
				assert.Equal(t, "completed", evt.Response.Status)
			}
		}
	}
}

// TestChatToResponseStreamBridge_ResponseCreatedHasOutputArray asserts that the very
// first SSE event emitted by the bridge (response.created) carries "output":[] rather
// than "output":null. This is the regression for strict OpenAI Python/TS SDK clients
// — the bug fired on every bridged stream's initial event.
func TestChatToResponseStreamBridge_ResponseCreatedHasOutputArray(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	bridge.ensureInitialized(c, &openai_compatible.ChatCompletionsStreamResponse{})

	body := w.Body.String()
	assert.NotContains(t, body, `"output":null`, "response.created event must never serialize output as null")

	events := parseBridgeSSE(body)
	created := bridgeFindEvents(events, "response.created")
	require.Len(t, created, 1)

	// Decode the data payload and inspect "response.output" raw bytes.
	var envelope struct {
		Response map[string]json.RawMessage `json:"response"`
	}
	require.NoError(t, json.Unmarshal(created[0].data, &envelope))
	require.Contains(t, envelope.Response, "output", "response.created payload must include output field")
	outputRaw := strings.TrimSpace(string(envelope.Response["output"]))
	assert.Equal(t, "[]", outputRaw, "output must serialize as an empty JSON array")

	// And via the typed accessor as a belt-and-braces check.
	var ev openai.ResponseAPIStreamEvent
	bridgeUnmarshal(t, created[0], &ev)
	require.NotNil(t, ev.Response)
	require.NotNil(t, ev.Response.Output, "Output must be a non-nil slice")
	assert.Equal(t, 0, len(ev.Response.Output))
}

// TestChatToResponseStreamBridge_FullEmptyStreamEmitsOutputArray drives the bridge with
// zero deltas (immediate HandleDone, simulating an upstream that emits [DONE] without
// any content). Every emitted event whose payload includes "output" must have it as []
// and never null.
func TestChatToResponseStreamBridge_FullEmptyStreamEmitsOutputArray(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	ok, done := bridge.HandleDone(c)
	require.True(t, ok)
	require.True(t, done)

	body := w.Body.String()
	assert.NotContains(t, body, `"output":null`, "no event may serialize output as null")

	events := parseBridgeSSE(body)
	require.NotEmpty(t, events)

	// Walk every event and, where "response.output" is present, require it to be [].
	// On response.completed the bridge places the in-progress message item into
	// response.output, so we only assert non-null (never null), not always empty.
	sawCreated := false
	sawCompleted := false
	for _, e := range events {
		var envelope struct {
			Response map[string]json.RawMessage `json:"response,omitempty"`
		}
		if err := json.Unmarshal(e.data, &envelope); err != nil {
			continue
		}
		raw, ok := envelope.Response["output"]
		if !ok {
			continue
		}
		trimmed := strings.TrimSpace(string(raw))
		assert.NotEqual(t, "null", trimmed, "event %q must not emit output:null", e.event)

		switch e.event {
		case "response.created":
			sawCreated = true
			assert.Equal(t, "[]", trimmed, "response.created must emit output:[]")
		case "response.completed":
			sawCompleted = true
			// The bridge always stages a message OutputItem on done, so this is a
			// non-empty array; the only invariant we enforce here is "not null".
			assert.True(t, strings.HasPrefix(trimmed, "["), "response.completed.output must be an array, got %q", trimmed)
		}
	}
	assert.True(t, sawCreated, "expected to observe a response.created event")
	assert.True(t, sawCompleted, "expected to observe a response.completed event")
}

// TestChatToResponseStreamBridge_BuildFinalResponseNilOutputDefaultsToEmptySlice
// guards the buildFinalResponse seam: even if a future caller passes nil, the wire
// response must still carry output:[] not output:null.
func TestChatToResponseStreamBridge_BuildFinalResponseNilOutputDefaultsToEmptySlice(t *testing.T) {
	t.Parallel()
	c, _ := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	resp := bridge.buildFinalResponse("completed", nil, nil)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Output, "buildFinalResponse must coerce nil outputs to a non-nil slice")
	assert.Equal(t, 0, len(resp.Output))

	data, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"output":[]`)
	assert.NotContains(t, string(data), `"output":null`)
}

// TestChatToResponseStreamBridge_UpstreamDropWithFinishLength verifies that
// when upstream sends finish_reason="length" but drops before [DONE], the
// bridge correctly reports "incomplete" status.
func TestChatToResponseStreamBridge_UpstreamDropWithFinishLength(t *testing.T) {
	c, w := newBridgeTestContext(t)

	bridge := newTestBridge(t, c)

	fr := "length"
	chunk := &openai_compatible.ChatCompletionsStreamResponse{
		Choices: []openai_compatible.ChatCompletionsStreamResponseChoice{
			{Delta: model.Message{Content: "truncated"}, FinishReason: &fr},
		},
	}
	bridge.HandleChunk(c, chunk)
	bridge.HandleDone(c)

	body := w.Body.String()
	assert.Contains(t, body, "event: response.completed")

	// Parse response.completed to check incomplete status
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "data: ") && strings.Contains(line, "response.completed") {
			var evt openai.ResponseAPIStreamEvent
			err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &evt)
			require.NoError(t, err)
			if evt.Response != nil {
				assert.Equal(t, "incomplete", evt.Response.Status)
			}
		}
	}
}
