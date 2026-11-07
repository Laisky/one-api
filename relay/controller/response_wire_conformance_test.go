package controller

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/model"
)

// TestResponseFallbackWireConformanceCodex relays the synthetic Codex request
// (text, namespaced, custom and tool_search calls) through the Chat and Claude
// fallbacks, streamed and not, and validates the client-facing Responses wire
// shape against the spec contract and the client parser rules.
func TestResponseFallbackWireConformanceCodex(t *testing.T) {
	for _, channel := range []int{channeltype.OpenAICompatible, channeltype.Anthropic} {
		for _, stream := range []bool{true, false} {
			name := channeltype.IdToName(channel) + map[bool]string{true: "/sse", false: "/json"}[stream]
			t.Run(name, func(t *testing.T) {
				_, recorder, _, _, _ := codexFallbackRun(t, channel, stream, 1_000_000)
				if stream {
					requireResponsesStreamConformance(t, recorder.Body.String())
					return
				}
				requireResponsesJSONConformance(t, recorder.Body.Bytes())
			})
		}
	}
}

// bridgeConformanceChunk builds a Chat stream chunk with one choice delta and
// an optional finish reason for the bridge conformance scenarios.
func bridgeConformanceChunk(delta model.Message, finish string) *openai_compatible.ChatCompletionsStreamResponse {
	choice := openai_compatible.ChatCompletionsStreamResponseChoice{Delta: delta}
	if finish != "" {
		choice.FinishReason = &finish
	}
	return &openai_compatible.ChatCompletionsStreamResponse{Model: "gpt-4o-test", Created: 1700000000, Choices: []openai_compatible.ChatCompletionsStreamResponseChoice{choice}}
}

// TestChatToResponseStreamBridgeWireConformance drives the stream bridge
// through the upstream shapes fallback meets (plain text, the three reasoning
// fields, parallel and empty-argument tool calls, length truncation, upstream
// failure and an empty stream) and validates every emitted SSE body against
// the Responses wire contract. Truncation must end with response.incomplete,
// the terminal event the spec defines for an incomplete response.
func TestChatToResponseStreamBridgeWireConformance(t *testing.T) {
	reasoning, thinking, content := "Let me think.", "Weighing options.", "Plan first."
	cases := []struct {
		name     string
		chunks   []*openai_compatible.ChatCompletionsStreamResponse
		fail     bool
		terminal string
	}{
		{name: "text", chunks: []*openai_compatible.ChatCompletionsStreamResponse{bridgeTextChunk("Hello"), bridgeTextChunk(" world"), bridgeFinishChunk("stop")}, terminal: "response.completed"},
		{name: "reasoning_then_text", chunks: []*openai_compatible.ChatCompletionsStreamResponse{bridgeConformanceChunk(model.Message{Reasoning: &reasoning}, ""), bridgeTextChunk("Answer"), bridgeFinishChunk("stop")}, terminal: "response.completed"},
		{name: "reasoning_content_then_tool", chunks: []*openai_compatible.ChatCompletionsStreamResponse{bridgeConformanceChunk(model.Message{ReasoningContent: &content}, ""), bridgeToolCallChunk("call_a", 0, "lookup", `{"q":`), bridgeToolCallChunk("", 0, "", `"x"}`), bridgeFinishChunk("tool_calls")}, terminal: "response.completed"},
		{name: "thinking_field", chunks: []*openai_compatible.ChatCompletionsStreamResponse{bridgeConformanceChunk(model.Message{Thinking: &thinking}, ""), bridgeTextChunk("Done"), bridgeFinishChunk("stop")}, terminal: "response.completed"},
		{name: "parallel_and_empty_argument_tools", chunks: []*openai_compatible.ChatCompletionsStreamResponse{bridgeToolCallChunk("call_a", 0, "first", ""), bridgeToolCallChunk("call_b", 1, "second", `{"k":1}`), bridgeFinishChunk("tool_calls")}, terminal: "response.completed"},
		{name: "length_truncation", chunks: []*openai_compatible.ChatCompletionsStreamResponse{bridgeConformanceChunk(model.Message{Content: "truncated"}, "length")}, terminal: "response.incomplete"},
		{name: "upstream_failure", chunks: []*openai_compatible.ChatCompletionsStreamResponse{bridgeTextChunk("partial")}, fail: true, terminal: "response.failed"},
		{name: "empty_stream", terminal: "response.completed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, w := newBridgeTestContext(t)
			bridge := newTestBridge(t, c)
			for _, chunk := range tc.chunks {
				bridge.HandleChunk(c, chunk)
			}
			if tc.fail {
				bridge.HandleError(c, &model.ErrorWithStatusCode{StatusCode: 502, Error: model.Error{Message: "upstream reset", Type: "upstream_error"}})
			} else {
				bridge.HandleDone(c)
			}
			requireResponsesStreamConformance(t, w.Body.String())
			events := parseSSEEvents(w.Body.String())
			last := events[len(events)-1]
			if last.data == "[DONE]" {
				last = events[len(events)-2]
			}
			require.Equal(t, tc.terminal, last.event)
		})
	}
}

// TestChatToResponseStreamBridgeCodexActiveItem proves the bridge streams so a
// single-active-item client (Codex) shows every text and reasoning delta: the
// message item opens only when its first text arrives, after reasoning that
// preceded it, and a tool-only turn emits no empty message item.
func TestChatToResponseStreamBridgeCodexActiveItem(t *testing.T) {
	reasoning := "Let me think."
	cases := map[string][]*openai_compatible.ChatCompletionsStreamResponse{
		"reasoning_then_text": {bridgeConformanceChunk(model.Message{Reasoning: &reasoning}, ""), bridgeTextChunk("Answer"), bridgeTextChunk(" now"), bridgeFinishChunk("stop")},
		"text_then_tool":      {bridgeTextChunk("Working."), bridgeToolCallChunk("call_a", 0, "lookup", `{"q":1}`), bridgeFinishChunk("tool_calls")},
		"tool_only":           {bridgeToolCallChunk("call_a", 0, "lookup", `{"q":1}`), bridgeFinishChunk("tool_calls")},
		"reasoning_then_tool": {bridgeConformanceChunk(model.Message{Reasoning: &reasoning}, ""), bridgeToolCallChunk("call_a", 0, "lookup", `{}`), bridgeFinishChunk("tool_calls")},
	}
	for name, chunks := range cases {
		t.Run(name, func(t *testing.T) {
			c, w := newBridgeTestContext(t)
			bridge := newTestBridge(t, c)
			for _, chunk := range chunks {
				bridge.HandleChunk(c, chunk)
			}
			bridge.HandleDone(c)
			requireResponsesStreamConformance(t, w.Body.String())
			requireCodexActiveItemStreaming(t, w.Body.String())
		})
	}
}
