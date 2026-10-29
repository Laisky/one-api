package controller

import (
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/streaming"
	"github.com/gin-gonic/gin"
	"net/http"
)

// HandleChunk converts chat stream content and tool deltas into Responses events.
func (h *chatToResponseStreamBridge) HandleChunk(c *gin.Context, chunk *openai_compatible.ChatCompletionsStreamResponse) (bool, bool) {
	h.deliveredImages = 0
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
		state := h.choiceState(c, choice.Index)
		h.activeChoiceIndex = choice.Index
		if h.writeErr != nil {
			return true, true
		}
		if choice.Delta.IsStringContent() {
			if delta := choice.Delta.StringContent(); delta != "" {
				h.appendChoiceText(c, state, delta)
			}
		} else {
			for _, part := range choice.Delta.ParseContent() {
				var text string
				image := false
				switch part.Type {
				case model.ContentTypeText:
					if part.Text != nil {
						text = *part.Text
					}
				case model.ContentTypeImageURL:
					if part.ImageURL != nil && part.ImageURL.Url != "" {
						// Intentional compatibility contract: the Chat-to-Responses
						// fallback projects image URLs (including base64 data URLs)
						// as output_text deltas. Existing consumers rely on this
						// representation. "image" tracks complete image-frame
						// delivery for billing; it is not a typed image SSE event.
						text, image = part.ImageURL.Url, true
					}
				case model.ContentTypeInputAudio:
					if part.InputAudio != nil {
						text = part.InputAudio.Data
					}
				}
				if h.appendChoiceText(c, state, text) && image {
					h.deliveredImages++
				}
				if h.writeErr != nil {
					return true, true
				}
			}
		}
		if h.writeErr != nil {
			return true, true
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
			state.finishReason = *choice.FinishReason
			if choice.Index == h.primaryChoiceIndex {
				h.lastFinishReason = *choice.FinishReason
			}
		}
		if h.writeErr != nil {
			return true, true
		}
	}

	if chunk.Usage != nil {
		h.usage = (&openai.ResponseAPIUsage{}).FromModelUsage(chunk.Usage)
	}

	return true, false
}
