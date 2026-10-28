package controller

import (
	"errors"
	"strings"
	"testing"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestBridgeChoiceReasoningAndToolIsolation covers identical provider tool identities across choices.
func TestBridgeChoiceReasoningAndToolIsolation(t *testing.T) {
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)
	first, second := "first thought", "second thought"
	index := 0
	choice := func(idx int, reasoning, args string) openai_compatible.ChatCompletionsStreamResponseChoice {
		return openai_compatible.ChatCompletionsStreamResponseChoice{Index: idx, Delta: model.Message{Reasoning: &reasoning, ToolCalls: []model.Tool{{Id: "call_shared", Index: &index, Function: &model.Function{Name: "lookup", Arguments: args}}}}}
	}
	bridge.HandleChunk(c, &openai_compatible.ChatCompletionsStreamResponse{Choices: []openai_compatible.ChatCompletionsStreamResponseChoice{choice(1, first, `{"a":`), choice(0, second, `{"b":`)}})
	bridge.HandleChunk(c, &openai_compatible.ChatCompletionsStreamResponse{Choices: []openai_compatible.ChatCompletionsStreamResponseChoice{choice(0, "", `2}`), choice(1, "", `1}`)}})
	bridge.HandleDone(c)
	require.NoError(t, bridge.StreamWriteError())
	events := parseBridgeSSE(w.Body.String())
	reasoning := bridgeFindEvents(events, "response.reasoning_summary_text.delta")
	require.Len(t, reasoning, 2)
	var r1, r2 openai.ResponseAPIStreamEvent
	bridgeUnmarshal(t, reasoning[0], &r1)
	bridgeUnmarshal(t, reasoning[1], &r2)
	require.NotEmpty(t, r1.ItemId)
	require.NotEqual(t, r1.ItemId, r2.ItemId)
	require.NotEqual(t, r1.OutputIndex, r2.OutputIndex)
	calls := bridgeFindEvents(events, "response.function_call_arguments.done")
	require.Len(t, calls, 2)
	var t1, t2 openai.ResponseAPIStreamEvent
	bridgeUnmarshal(t, calls[0], &t1)
	bridgeUnmarshal(t, calls[1], &t2)
	require.NotEqual(t, t1.ItemId, t2.ItemId)
	require.Equal(t, `{"a":1}`, t1.Arguments)
	require.Equal(t, `{"b":2}`, t2.Arguments)
}

type rejectDoneWriter struct{ gin.ResponseWriter }

func (w rejectDoneWriter) Write(data []byte) (int, error) {
	if strings.Contains(string(data), "[DONE]") {
		return 0, errors.New("terminal write failed")
	}
	return w.ResponseWriter.Write(data)
}

func (w rejectDoneWriter) WriteString(data string) (int, error) {
	return w.Write([]byte(data))
}

// TestBridgeTerminalWriteError checks that the terminal delimiter error is surfaced.
func TestBridgeTerminalWriteError(t *testing.T) {
	c, _ := newBridgeTestContext(t)
	c.Writer = rejectDoneWriter{c.Writer}
	bridge := newTestBridge(t, c)
	bridge.HandleChunk(c, bridgeTextChunk("ok"))
	bridge.HandleDone(c)
	require.ErrorContains(t, bridge.StreamWriteError(), "terminal write failed")
}

// TestBridgeChoiceFinishStatusIsOrderIndependent checks cancellation and length precedence.
func TestBridgeChoiceFinishStatusIsOrderIndependent(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		c, _ := newBridgeTestContext(t)
		bridge := newTestBridge(t, c)
		length, cancelled := "length", "cancelled"
		choices := []openai_compatible.ChatCompletionsStreamResponseChoice{{Index: 0, FinishReason: &length}, {Index: 1, FinishReason: &cancelled}}
		if reverse {
			choices[0], choices[1] = choices[1], choices[0]
		}
		bridge.HandleChunk(c, &openai_compatible.ChatCompletionsStreamResponse{Choices: choices})
		status, _ := bridge.finalStatus()
		require.Equal(t, "cancelled", status)
	}
}
