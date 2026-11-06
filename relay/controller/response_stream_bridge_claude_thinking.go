package controller

import (
	"strings"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/model"
)

// handleClaudeThinkingBlocks turns verbatim Claude thinking blocks into one
// completed reasoning item each. Parameters: c is the request context, choice
// the upstream choice, blocks the replayable blocks in original order, searches
// the provider-executed web searches and tools any tool calls of the same delta
// (a whole message rendered as one chunk), so each block lands before the
// function_call it preceded and searches follow the leading thinking. Streaming Claude
// sends each block in its own chunk once its signature arrived, which emits
// response.output_item.done immediately and lets clients that record items in
// done order (Codex) keep the thinking ahead of the turn's calls and text.
func (h *chatToResponseStreamBridge) handleClaudeThinkingBlocks(c *gin.Context, choice *responseChoiceState, blocks []model.ClaudeThinkingBlock, searches []model.WebSearchCall, tools []model.Tool) {
	lg := gmw.GetLogger(c)
	next := 0
	seal := func(ready func(model.ClaudeThinkingBlock) bool) {
		for next < len(blocks) && ready(blocks[next]) && h.writeErr == nil {
			h.sealClaudeThinkingBlock(c, lg, choice, blocks[next])
			next++
		}
	}
	// Thinking that preceded every caller tool call comes first, then the
	// provider-executed searches, matching buildResponseOutput's JSON order.
	seal(func(block model.ClaudeThinkingBlock) bool { return block.ToolCallsBefore == 0 })
	if len(searches) > 0 && h.writeErr == nil {
		h.handleWebSearchCalls(c, searches)
	}
	for i := range tools {
		seal(func(block model.ClaudeThinkingBlock) bool { return block.ToolCallsBefore <= i })
		if h.writeErr != nil {
			return
		}
		h.handleToolCalls(c, tools[i:i+1])
	}
	seal(func(model.ClaudeThinkingBlock) bool { return true })
	lg.Debug("bridged Claude thinking blocks to reasoning items",
		zap.Int("thinking_blocks", len(blocks)),
		zap.Int("sealed_reasoning_items", len(choice.sealedReasoning)),
		zap.Bool("include_encrypted_content", h.includeEncrypted),
	)
}

// sealClaudeThinkingBlock completes the choice's open reasoning item with block,
// or opens and completes a new one when nothing was streamed for it (omitted
// display or redacted thinking). Parameters: c is the request context, lg the
// request logger, choice the upstream choice and block one replayable block.
// The sealed item keeps its envelope for state; the emitted copy honors include.
func (h *chatToResponseStreamBridge) sealClaudeThinkingBlock(c *gin.Context, lg glog.Logger, choice *responseChoiceState, block model.ClaudeThinkingBlock) {
	if !block.Replayable() {
		return
	}
	// The block's exact text is authoritative for the completed item; streamed
	// deltas only served live display.
	summary := strings.TrimSpace(block.Thinking)
	if choice.reasoningInitialized {
		if summary == "" {
			summary = strings.TrimSpace(choice.reasoning.String())
		}
	} else {
		h.beforeItemAdded(c, choice, itemKindReasoning)
		choice.reasoningID, choice.reasoningIndex = newClaudeThinkingReasoningID(), h.nextOutputIndex()
		added := &openai.OutputItem{Id: choice.reasoningID, Type: "reasoning", Status: "in_progress"}
		h.emitEvent(c, "response.output_item.added", openai.ResponseAPIStreamEvent{OutputIndex: choice.reasoningIndex, Item: added})
		if summary != "" {
			h.emitEvent(c, "response.reasoning_summary_part.added", openai.ResponseAPIStreamEvent{ItemId: choice.reasoningID, OutputIndex: choice.reasoningIndex, Part: &openai.OutputContent{Type: "summary_text"}})
			h.emitEvent(c, "response.reasoning_summary_text.delta", openai.ResponseAPIStreamEvent{ItemId: choice.reasoningID, OutputIndex: choice.reasoningIndex, Delta: rawMessageFromString(summary)})
		}
	}
	item := newClaudeThinkingReasoningItem(lg, choice.reasoningID, block, summary)
	if summary != "" {
		h.emitEvent(c, "response.reasoning_summary_text.done", openai.ResponseAPIStreamEvent{ItemId: choice.reasoningID, OutputIndex: choice.reasoningIndex, Text: summary})
		h.emitEvent(c, "response.reasoning_summary_part.done", openai.ResponseAPIStreamEvent{ItemId: choice.reasoningID, OutputIndex: choice.reasoningIndex, Part: &openai.OutputContent{Type: "summary_text", Text: summary}})
	}
	visible := clientVisibleOutputItem(item, h.includeEncrypted)
	h.emitEvent(c, "response.output_item.done", openai.ResponseAPIStreamEvent{OutputIndex: choice.reasoningIndex, Item: &visible})
	choice.sealedReasoning = append(choice.sealedReasoning, sealedReasoningItem{index: choice.reasoningIndex, item: item})
	// The next thinking block, if any, starts a fresh reasoning item.
	choice.reasoning.Reset()
	choice.reasoningInitialized = false
	choice.reasoningID = ""
}
