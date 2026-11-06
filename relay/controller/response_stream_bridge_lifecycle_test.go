package controller

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

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

	// The upstream ID arrives only in the second chunk, after output_item.added has
	// already published an ID. Adopting it there used to split one logical call across
	// two item_ids, so a client accumulating arguments by item_id reconstructed `{"a"`
	// under one ID and `:1}` under the other — invalid JSON for both. The published ID
	// is therefore stable for the whole call; it only has to be self-consistent, since
	// convertResponseAPIIDToToolCall maps it back on replay.
	assert.Equal(t, bridgeSoleToolCallItemID(t, events), ev.ItemId)
}

// bridgeSoleToolCallItemID returns the single item_id every event of the stream's
// only function_call must carry, failing the test when the events disagree.
// Parameters: t is the testing handle and events are the parsed SSE frames.
// Returns: the one item_id shared by the call's added, delta, and done events.
func bridgeSoleToolCallItemID(t *testing.T, events []bridgeSSE) string {
	t.Helper()
	seen := make(map[string]struct{})
	for _, name := range []string{
		"response.output_item.added",
		"response.function_call_arguments.delta",
		"response.function_call_arguments.done",
		"response.output_item.done",
	} {
		for _, e := range bridgeFindEvents(events, name) {
			var ev openai.ResponseAPIStreamEvent
			bridgeUnmarshal(t, e, &ev)
			if ev.Item != nil {
				if ev.Item.Type != "function_call" {
					continue
				}
				seen[ev.Item.Id] = struct{}{}
				require.Equal(t, ev.Item.Id, ev.Item.CallId, "%s must carry id == call_id", name)
				continue
			}
			if ev.ItemId != "" {
				seen[ev.ItemId] = struct{}{}
			}
		}
	}
	require.Len(t, seen, 1, "one tool call must use exactly one item_id across its events")
	for id := range seen {
		return id
	}
	return ""
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

// TestChatToResponseStreamBridge_ToolCallIDStableWhenUpstreamIDArrivesLate pins the
// identity contract the added event publishes. Upstreams that send an index-only
// tool_call delta first and the real ID later must not rename the call after
// response.output_item.added announced it: clients such as pi read call_id off the
// added event and never revisit it, and every event of one call must share one
// item_id so argument deltas reassemble into valid JSON.
func TestChatToResponseStreamBridge_ToolCallIDStableWhenUpstreamIDArrivesLate(t *testing.T) {
	t.Parallel()
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)

	bridge.HandleChunk(c, bridgeToolCallChunk("", 0, "read_file", `{"path"`))
	bridge.HandleChunk(c, bridgeToolCallChunk("call_late", 0, "", `:"a"}`))
	bridge.HandleChunk(c, bridgeFinishChunk("tool_calls"))
	bridge.HandleDone(c)

	events := parseBridgeSSE(w.Body.String())
	stableID := bridgeSoleToolCallItemID(t, events)
	require.NotEmpty(t, stableID)

	var argsDone openai.ResponseAPIStreamEvent
	done := bridgeFindEvents(events, "response.function_call_arguments.done")
	require.Len(t, done, 1)
	bridgeUnmarshal(t, done[0], &argsDone)
	require.Equal(t, `{"path":"a"}`, argsDone.Arguments,
		"a renamed item_id would scatter the argument fragments across two calls")

	var completed openai.ResponseAPIStreamEvent
	final := bridgeFindEvents(events, "response.completed")
	require.Len(t, final, 1)
	bridgeUnmarshal(t, final[0], &completed)
	require.NotNil(t, completed.Response)
	for _, item := range completed.Response.Output {
		if item.Type == "function_call" {
			require.Equal(t, stableID, item.Id)
			require.Equal(t, stableID, item.CallId)
		}
	}
}

// TestBuildResponseOutput_FunctionCallCarriesBothIDs verifies the non-streaming
// fallback emits the same function_call identity as the streaming bridge. A client
// keying the tool call on the item id must not get an empty string just because the
// channel answered without streaming.
func TestBuildResponseOutput_FunctionCallCarriesBothIDs(t *testing.T) {
	t.Parallel()
	for name, upstreamID := range map[string]string{
		"already_prefixed": "call_read",
		"bare_id":          "read_raw",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			out := buildResponseOutput(logger.Logger, []openai_compatible.TextResponseChoice{{
				Message: model.Message{
					Role:    "assistant",
					Content: "Let me read it.",
					ToolCalls: []model.Tool{{
						Id:       upstreamID,
						Type:     "function",
						Function: &model.Function{Name: "read_file", Arguments: `{"path":"a"}`},
					}},
				},
			}}, nil)

			var seen int
			for _, item := range out {
				if item.Type != "function_call" {
					continue
				}
				seen++
				require.NotEmpty(t, item.Id, "function_call items must carry an id")
				require.Equal(t, item.Id, item.CallId)
				require.Equal(t, ensureResponseAPICallID(upstreamID), item.CallId)
			}
			require.Equal(t, 1, seen)
		})
	}
}
