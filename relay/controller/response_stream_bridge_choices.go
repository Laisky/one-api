package controller

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/random"
	"github.com/Laisky/one-api/relay/adaptor/openai"
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

// appendChoiceText streams one text delta of choice's message item. It
// returns whether the delta was written.
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

// appendChoiceReasoning streams one reasoning summary delta of choice, opening
// its reasoning item and summary part (summary_index 0) on first use.
func (h *chatToResponseStreamBridge) appendChoiceReasoning(c *gin.Context, choice *responseChoiceState, text string) {
	if choice == nil || text == "" {
		return
	}
	if !choice.reasoningInitialized {
		h.beforeItemAdded(c, choice, itemKindReasoning)
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

// choiceMessage renders choice's message item with the given status and the
// text streamed so far.
func (h *chatToResponseStreamBridge) choiceMessage(choice *responseChoiceState, status string) openai.OutputItem {
	return openai.OutputItem{Id: choice.messageID, Type: "message", Status: status, Role: "assistant", Content: h.choiceContents(choice)}
}

// choiceReasoning renders choice's streamed reasoning as a reasoning item with
// a summary_text part and a reasoning_text content part, or nil when no
// reasoning text was streamed.
func (h *chatToResponseStreamBridge) choiceReasoning(choice *responseChoiceState, status string) *openai.OutputItem {
	text := strings.TrimSpace(choice.reasoning.String())
	if text == "" {
		return nil
	}
	return &openai.OutputItem{Id: choice.reasoningID, Type: "reasoning", Status: status, Content: []openai.OutputContent{{Type: "reasoning_text", Text: text}}, Summary: []openai.OutputContent{{Type: "summary_text", Text: text}}}
}

// finishChoice emits the completion events of choice's secondary message item
// and of its streamed reasoning item (summary text done, summary part done,
// output_item.done).
func (h *chatToResponseStreamBridge) finishChoice(c *gin.Context, choice *responseChoiceState) {
	if choice.index != h.primaryChoiceIndex {
		item := h.choiceMessage(choice, "completed")
		h.emitChoiceContentDone(c, choice)
		h.emitEvent(c, "response.output_item.done", openai.ResponseAPIStreamEvent{OutputIndex: choice.messageIndex, Item: &item})
	}
	if item := h.choiceReasoning(choice, "completed"); item != nil {
		summary := item.Summary[0].Text
		h.emitEvent(c, "response.reasoning_summary_text.done", openai.ResponseAPIStreamEvent{ItemId: choice.reasoningID, OutputIndex: choice.reasoningIndex, Text: summary})
		h.emitEvent(c, "response.reasoning_summary_part.done", openai.ResponseAPIStreamEvent{ItemId: choice.reasoningID, OutputIndex: choice.reasoningIndex, Part: &openai.OutputContent{Type: "summary_text", Text: summary}})
		h.emitEvent(c, "response.output_item.done", openai.ResponseAPIStreamEvent{OutputIndex: choice.reasoningIndex, Item: item})
	}
}
