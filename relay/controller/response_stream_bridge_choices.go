package controller

import (
	"strings"

	"github.com/Laisky/one-api/common/random"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/gin-gonic/gin"
)

// choiceState gives each upstream choice a stable message item and output index.
func (h *chatToResponseStreamBridge) choiceState(c *gin.Context, index int) *responseChoiceState {
	if h.choices == nil {
		h.choices = make(map[int]*responseChoiceState)
	}
	if choice := h.choices[index]; choice != nil {
		return choice
	}
	choice := &responseChoiceState{index: index}
	if !h.primaryChoiceBound {
		h.primaryChoiceBound, h.primaryChoiceIndex = true, index
		choice.messageID, choice.messageIndex = h.messageItemID, h.messageOutputIndex
	} else {
		choice.messageID, choice.messageIndex = "msg_"+random.GetRandomString(16), h.nextOutputIndex()
		item := &openai.OutputItem{Id: choice.messageID, Type: "message", Status: "in_progress", Role: "assistant", Content: make([]openai.OutputContent, 0)}
		h.emitEvent(c, "response.output_item.added", openai.ResponseAPIStreamEvent{OutputIndex: choice.messageIndex, Item: item})
		h.emitEvent(c, "response.content_part.added", openai.ResponseAPIStreamEvent{ItemId: choice.messageID, OutputIndex: choice.messageIndex, ContentIndex: 0, Part: &openai.OutputContent{Type: "output_text"}})
	}
	h.choices[index] = choice
	h.choiceOrder = append(h.choiceOrder, choice)
	return choice
}

func (h *chatToResponseStreamBridge) appendChoiceText(c *gin.Context, choice *responseChoiceState, text string) bool {
	if text == "" {
		return false
	}
	if choice.index == h.primaryChoiceIndex {
		h.appendTextDelta(c, text)
		return h.writeErr == nil
	}
	if !h.emitEvent(c, "response.output_text.delta", openai.ResponseAPIStreamEvent{ItemId: choice.messageID, OutputIndex: choice.messageIndex, ContentIndex: 0, Delta: rawMessageFromString(text)}) {
		return false
	}
	choice.text.WriteString(text)
	return true
}

func (h *chatToResponseStreamBridge) appendChoiceReasoning(c *gin.Context, choice *responseChoiceState, text string) {
	if choice == nil || text == "" {
		return
	}
	if !choice.reasoningInitialized {
		choice.reasoningID, choice.reasoningIndex = "rs_"+random.GetRandomString(16), h.nextOutputIndex()
		item := &openai.OutputItem{Id: choice.reasoningID, Type: "reasoning", Status: "in_progress"}
		if !h.emitEvent(c, "response.output_item.added", openai.ResponseAPIStreamEvent{OutputIndex: choice.reasoningIndex, Item: item}) {
			return
		}
		if !h.emitEvent(c, "response.reasoning_summary_part.added", openai.ResponseAPIStreamEvent{ItemId: choice.reasoningID, OutputIndex: choice.reasoningIndex, Part: &openai.OutputContent{Type: "summary_text"}}) {
			return
		}
		choice.reasoningInitialized = true
	}
	if !h.emitEvent(c, "response.reasoning_summary_text.delta", openai.ResponseAPIStreamEvent{ItemId: choice.reasoningID, OutputIndex: choice.reasoningIndex, Delta: rawMessageFromString(text)}) {
		return
	}
	choice.reasoning.WriteString(text)
	if choice.index == h.primaryChoiceIndex {
		h.reasoningBuilder.WriteString(text)
		h.reasoningInitialized = true
	}
}

func (h *chatToResponseStreamBridge) choiceMessage(choice *responseChoiceState, status string) openai.OutputItem {
	text := choice.text.String()
	if choice.index == h.primaryChoiceIndex {
		text = h.textBuilder.String()
	}
	return openai.OutputItem{Id: choice.messageID, Type: "message", Status: status, Role: "assistant", Content: []openai.OutputContent{{Type: "output_text", Text: text}}}
}

func (h *chatToResponseStreamBridge) choiceReasoning(choice *responseChoiceState, status string) *openai.OutputItem {
	text := strings.TrimSpace(choice.reasoning.String())
	if text == "" {
		return nil
	}
	return &openai.OutputItem{Id: choice.reasoningID, Type: "reasoning", Status: status, Content: []openai.OutputContent{{Type: "text", Text: text}}, Summary: []openai.OutputContent{{Type: "summary_text", Text: text}}}
}

func (h *chatToResponseStreamBridge) finishChoice(c *gin.Context, choice *responseChoiceState) {
	if choice.index != h.primaryChoiceIndex {
		item := h.choiceMessage(choice, "completed")
		text := item.Content[0].Text
		h.emitEvent(c, "response.output_text.done", openai.ResponseAPIStreamEvent{ItemId: choice.messageID, OutputIndex: choice.messageIndex, Text: text})
		h.emitEvent(c, "response.content_part.done", openai.ResponseAPIStreamEvent{ItemId: choice.messageID, OutputIndex: choice.messageIndex, Part: &openai.OutputContent{Type: "output_text", Text: text}})
		h.emitEvent(c, "response.output_item.done", openai.ResponseAPIStreamEvent{OutputIndex: choice.messageIndex, Item: &item})
	}
	if item := h.choiceReasoning(choice, "completed"); item != nil {
		h.emitEvent(c, "response.reasoning_summary_text.done", openai.ResponseAPIStreamEvent{ItemId: choice.reasoningID, OutputIndex: choice.reasoningIndex, Part: &openai.OutputContent{Type: "summary_text", Text: item.Summary[0].Text}})
		h.emitEvent(c, "response.output_item.done", openai.ResponseAPIStreamEvent{OutputIndex: choice.reasoningIndex, Item: item})
	}
}
