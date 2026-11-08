package controller

import (
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/random"
	"github.com/Laisky/one-api/relay/adaptor/openai"
)

// Output items of the primary choice stream one after another, the way the
// Responses API emits them: before a new item is announced, the open message
// or streamed reasoning item is completed. Clients that track a single active
// item (Codex) therefore show every delta, record items in the order the
// model produced them, and never see a message closed after the tool calls
// that followed it. Tool calls and web searches still complete at the end of
// the response, after their arguments or results arrived.

// itemKind names the kind of output item about to be announced.
type itemKind int

const (
	// itemKindMessage is an assistant message item.
	itemKindMessage itemKind = iota
	// itemKindReasoning is a reasoning item.
	itemKindReasoning
	// itemKindOther is a tool call or web search item.
	itemKindOther
)

// beforeItemAdded completes the open primary-choice items that the next item
// supersedes. Parameters: c is the request context, choice the upstream choice
// the next item belongs to (nil for the primary) and next its kind. A message
// completes before any other kind; streamed reasoning completes before any
// other kind. Secondary choices (n > 1) keep their legacy single items.
func (h *chatToResponseStreamBridge) beforeItemAdded(c *gin.Context, choice *responseChoiceState, next itemKind) {
	if choice != nil && h.primaryChoiceBound && choice.index != h.primaryChoiceIndex {
		return
	}
	if next != itemKindMessage {
		h.closePrimaryMessage(c)
	}
	if next != itemKindReasoning {
		if primary := h.choices[h.primaryChoiceIndex]; primary != nil && h.primaryChoiceBound {
			h.closeStreamedReasoning(c, primary)
		}
	}
}

// closePrimaryMessage completes the open primary message item: it emits the
// text, part and item done events, keeps the completed item for the terminal
// response, and prepares a fresh item id for text that may follow later.
func (h *chatToResponseStreamBridge) closePrimaryMessage(c *gin.Context) {
	if !h.messageOpened {
		return
	}
	item := h.emitPrimaryMessageDone(c)
	h.sealedMessages = append(h.sealedMessages, sealedReasoningItem{index: h.messageOutputIndex, item: item})
	h.messageOpened = false
	h.textBuilder.Reset()
	h.messageItemID = "msg_" + random.GetRandomString(16)
	h.messageOutputIndex = -1
}

// emitPrimaryMessageDone emits the completion events of the open primary
// message and returns its completed item.
func (h *chatToResponseStreamBridge) emitPrimaryMessageDone(c *gin.Context) openai.OutputItem {
	text := h.textBuilder.String()
	h.emitEvent(c, "response.output_text.done", openai.ResponseAPIStreamEvent{
		ItemId:       h.messageItemID,
		OutputIndex:  h.messageOutputIndex,
		ContentIndex: 0,
		Text:         text,
	})
	h.emitEvent(c, "response.content_part.done", openai.ResponseAPIStreamEvent{
		ItemId:       h.messageItemID,
		OutputIndex:  h.messageOutputIndex,
		ContentIndex: 0,
		Part:         &openai.OutputContent{Type: "output_text", Text: text},
	})
	item := openai.OutputItem{
		Id:      h.messageItemID,
		Type:    "message",
		Status:  "completed",
		Role:    "assistant",
		Content: []openai.OutputContent{{Type: "output_text", Text: text}},
	}
	h.emitEvent(c, "response.output_item.done", openai.ResponseAPIStreamEvent{
		OutputIndex: h.messageOutputIndex,
		Item:        &item,
	})
	return item
}

// closeStreamedReasoning completes choice's open streamed reasoning item (from
// reasoning, reasoning_content or thinking deltas) and keeps it with the
// choice's sealed reasoning, so later reasoning opens a new item.
func (h *chatToResponseStreamBridge) closeStreamedReasoning(c *gin.Context, choice *responseChoiceState) {
	if choice == nil || !choice.reasoningInitialized {
		return
	}
	item := h.choiceReasoning(choice, "completed")
	if item == nil {
		// The item was announced, so it must complete even without text.
		item = &openai.OutputItem{Id: choice.reasoningID, Type: "reasoning", Status: "completed", Summary: []openai.OutputContent{{Type: "summary_text"}}}
	}
	{
		summary := item.Summary[0].Text
		h.emitEvent(c, "response.reasoning_summary_text.done", openai.ResponseAPIStreamEvent{ItemId: choice.reasoningID, OutputIndex: choice.reasoningIndex, Text: summary})
		h.emitEvent(c, "response.reasoning_summary_part.done", openai.ResponseAPIStreamEvent{ItemId: choice.reasoningID, OutputIndex: choice.reasoningIndex, Part: &openai.OutputContent{Type: "summary_text", Text: summary}})
		h.emitEvent(c, "response.output_item.done", openai.ResponseAPIStreamEvent{OutputIndex: choice.reasoningIndex, Item: item})
		choice.sealedReasoning = append(choice.sealedReasoning, sealedReasoningItem{index: choice.reasoningIndex, item: *item})
	}
	choice.reasoning.Reset()
	choice.reasoningInitialized = false
	choice.reasoningID = ""
}
