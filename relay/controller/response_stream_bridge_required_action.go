package controller

import (
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor/openai"
)

// The required_action events below are a gateway extension that mirrors the
// pending tool calls in the Assistants-style submit_tool_outputs shape for
// clients that track tool progress that way; spec clients ignore them.

// emitRequiredActionDelta streams the required_action payload so clients can observe tool call progress.
// When markDone is true, a terminal done event is emitted after the delta message.
func (h *chatToResponseStreamBridge) emitRequiredActionDelta(c *gin.Context, markDone bool) {
	required := h.buildRequiredActionSnapshot()
	if required == nil {
		return
	}

	eventType := "response.required_action.delta"
	if !h.requiredActionInitialized {
		eventType = "response.required_action.added"
		h.requiredActionInitialized = true
	}

	h.emitEvent(c, eventType, openai.ResponseAPIStreamEvent{
		RequiredAction: required,
	})

	if markDone {
		h.emitEvent(c, "response.required_action.done", openai.ResponseAPIStreamEvent{
			RequiredAction: required,
		})
	}
}

// buildRequiredActionSnapshot constructs the Response API required_action object based on current tool call state.
func (h *chatToResponseStreamBridge) buildRequiredActionSnapshot() *openai.ResponseAPIRequiredAction {
	toolCalls := h.currentToolCallSnapshots()
	if len(toolCalls) == 0 {
		return nil
	}

	return &openai.ResponseAPIRequiredAction{
		Type: "submit_tool_outputs",
		SubmitToolOutputs: &openai.ResponseAPISubmitToolOutputs{
			ToolCalls: toolCalls,
		},
	}
}

// currentToolCallSnapshots returns copies of the accumulated tool call information in stream order.
func (h *chatToResponseStreamBridge) currentToolCallSnapshots() []openai.ResponseAPIToolCall {
	if len(h.toolOrder) == 0 {
		return nil
	}

	calls := make([]openai.ResponseAPIToolCall, 0, len(h.toolOrder))
	for _, id := range h.toolOrder {
		state := h.toolCalls[id]
		if state == nil {
			continue
		}

		callID := ensureResponseAPICallID(state.id)
		state.id = callID
		calls = append(calls, openai.ResponseAPIToolCall{
			ExtraContent: state.extraContent,
			Id:           callID,
			Type:         "function",
			Function: &openai.ResponseAPIFunctionCall{
				Name:      state.name,
				Arguments: state.arguments.String(),
			},
		})
	}

	return calls
}
