package controller

import (
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/model"
)

// responseGoogleTextParts decodes typed metadata without changing legacy parsing on malformed unrelated content.
func responseGoogleTextParts(message model.Message) []model.MessageContent {
	parts, err := message.ParseContentChecked()
	if err != nil {
		return message.ParseContent()
	}
	return parts
}

// choiceContents returns positional content for metadata-bearing choices or the established aggregate text for ordinary choices.
func (h *chatToResponseStreamBridge) choiceContents(choice *responseChoiceState) []openai.OutputContent {
	if choice != nil && choice.contentParts != nil {
		return append([]openai.OutputContent(nil), choice.contentParts...)
	}
	text := ""
	if choice != nil {
		text = choice.text.String()
	}
	if choice == nil || choice.index == h.primaryChoiceIndex {
		text = h.textBuilder.String()
	}
	return []openai.OutputContent{{Type: "output_text", Text: text}}
}

// appendChoiceMetadataText emits a native Google text part and keeps its exact metadata boundary for terminal replay.
func (h *chatToResponseStreamBridge) appendChoiceMetadataText(c *gin.Context, choice *responseChoiceState, text string, extra *model.ToolExtraContent) bool {
	if choice.contentParts == nil && extra == nil {
		return h.appendChoiceText(c, choice, text)
	}
	if choice.index == h.primaryChoiceIndex {
		h.openPrimaryMessage(c)
	}
	if choice.contentParts == nil {
		previous := h.choiceContents(choice)[0].Text
		choice.contentParts = make([]openai.OutputContent, 0)
		if previous != "" {
			choice.contentParts = append(choice.contentParts, openai.OutputContent{Type: "output_text", Text: previous})
		}
	}
	if extra != nil || len(choice.contentParts) == 0 || choice.contentDone == len(choice.contentParts) {
		h.emitChoiceContentDone(c, choice)
		index := len(choice.contentParts)
		part := openai.OutputContent{Type: "output_text", ExtraContent: extra}
		choice.contentParts = append(choice.contentParts, part)
		if index > 0 {
			if !h.emitEvent(c, "response.content_part.added", openai.ResponseAPIStreamEvent{ItemId: choice.messageID, OutputIndex: choice.messageIndex, ContentIndex: index, Part: &part}) {
				return false
			}
		}
	}
	index := len(choice.contentParts) - 1
	if text != "" {
		if !h.emitEvent(c, "response.output_text.delta", openai.ResponseAPIStreamEvent{ItemId: choice.messageID, OutputIndex: choice.messageIndex, ContentIndex: index, Delta: rawMessageFromString(text)}) {
			return false
		}
		choice.contentParts[index].Text += text
		if choice.index == h.primaryChoiceIndex {
			h.textBuilder.WriteString(text)
		} else {
			choice.text.WriteString(text)
		}
	}
	if extra != nil {
		h.emitChoiceContentDone(c, choice)
	}
	return h.writeErr == nil
}

// emitChoiceContentDone completes each unclosed positional content part once, including empty signed parts.
func (h *chatToResponseStreamBridge) emitChoiceContentDone(c *gin.Context, choice *responseChoiceState) {
	contents := h.choiceContents(choice)
	first := 0
	if choice != nil && choice.contentParts != nil {
		first = choice.contentDone
	}
	id, index := h.messageItemID, h.messageOutputIndex
	if choice != nil {
		id, index = choice.messageID, choice.messageIndex
	}
	for i := first; i < len(contents); i++ {
		part := contents[i]
		h.emitEvent(c, "response.output_text.done", openai.ResponseAPIStreamEvent{ItemId: id, OutputIndex: index, ContentIndex: i, Text: part.Text})
		h.emitEvent(c, "response.content_part.done", openai.ResponseAPIStreamEvent{ItemId: id, OutputIndex: index, ContentIndex: i, Part: &part})
	}
	if choice != nil && choice.contentParts != nil {
		choice.contentDone = len(contents)
	}
}

// primaryMessage returns the current primary message with its positional Google metadata.
func (h *chatToResponseStreamBridge) primaryMessage(status string) openai.OutputItem {
	return openai.OutputItem{Id: h.messageItemID, Type: "message", Status: status, Role: "assistant", Content: h.choiceContents(h.choices[h.primaryChoiceIndex])}
}
