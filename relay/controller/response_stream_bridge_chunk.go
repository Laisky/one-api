package controller

import (
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/streaming"
	"github.com/gin-gonic/gin"
	"net/http"
)

// HandleChunk converts chat stream content and tool deltas into Responses events.
func (h *chatToResponseStreamBridge) HandleChunk(c *gin.Context, chunk *openai_compatible.ChatCompletionsStreamResponse) (bool, bool) {
	if h.streamDone {
		return true, true
	}

	if tracker := streaming.FromContext(c); tracker != nil && !streaming.HasProtocolObservation(c) {
		if err := openai_compatible.ObserveStreamChunk(c, chunk); err != nil {
			h.FinalizeUsage(tracker.UsageSnapshot())
			streaming.StopUpstream(c)
			return h.HandleError(c, openai.ErrorWrapper(err, "streaming_billing_failed", http.StatusForbidden))
		}
	}

	if !h.streamStarted {
		h.ensureInitialized(c, chunk)
	}

	if chunk.Model != "" {
		h.model = chunk.Model
	}
	if chunk.Created != 0 {
		h.createdAt = chunk.Created
	}

	for _, choice := range chunk.Choices {
		if choice.Delta.IsStringContent() {
			if delta := choice.Delta.StringContent(); delta != "" {
				h.appendTextDelta(c, delta)
			}
		} else {
			for _, part := range convertMessageContent(choice.Delta) {
				h.appendTextDelta(c, part.Text)
			}
		}

		if choice.Delta.Reasoning != nil && *choice.Delta.Reasoning != "" {
			h.appendReasoningDelta(c, *choice.Delta.Reasoning)
		}
		if choice.Delta.ReasoningContent != nil && *choice.Delta.ReasoningContent != "" {
			h.appendReasoningDelta(c, *choice.Delta.ReasoningContent)
		}
		if choice.Delta.Thinking != nil && *choice.Delta.Thinking != "" {
			h.appendReasoningDelta(c, *choice.Delta.Thinking)
		}

		if len(choice.Delta.ToolCalls) > 0 {
			h.handleToolCalls(c, choice.Delta.ToolCalls)
		}

		if choice.FinishReason != nil {
			h.lastFinishReason = *choice.FinishReason
		}
	}

	if chunk.Usage != nil {
		h.usage = (&openai.ResponseAPIUsage{}).FromModelUsage(chunk.Usage)
	}

	return true, false
}
