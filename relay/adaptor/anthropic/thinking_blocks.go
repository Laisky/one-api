package anthropic

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/model"
)

// ThinkingBlocksRequested reports whether the Chat output of this Claude call
// feeds the Responses API fallback bridge, which is the only consumer of
// model.Message.ClaudeThinkingBlocks. The c parameter is the request context;
// the return value is false for plain Chat Completions clients, so they never
// receive the gateway-internal field.
func ThinkingBlocksRequested(c *gin.Context) bool {
	if c == nil {
		return false
	}
	_, ok := c.Get(ctxkey.ResponseAPIRequestOriginal)
	return ok
}

// claudeThinkingBlocksFromContent extracts every replayable thinking and
// redacted_thinking block of one Claude message in original order, recording
// each block's position relative to the Chat-visible text and tool_use blocks.
// The content parameter is the decoded Claude message content; the return value
// is nil when the message carries no replayable thinking state.
func claudeThinkingBlocksFromContent(content []Content) []model.ClaudeThinkingBlock {
	var blocks []model.ClaudeThinkingBlock
	toolCalls, sawText := 0, false
	for _, item := range content {
		switch item.Type {
		case model.ClaudeThinkingBlockTypeThinking, model.ClaudeThinkingBlockTypeRedacted:
			block := model.ClaudeThinkingBlock{Type: item.Type, ToolCallsBefore: toolCalls, AfterText: sawText}
			if item.Thinking != nil {
				block.Thinking = *item.Thinking
			}
			if item.Signature != nil {
				block.Signature = *item.Signature
			}
			if item.Data != nil {
				block.Data = *item.Data
			}
			if block.Replayable() {
				blocks = append(blocks, block)
			}
		case "text":
			sawText = true
		case "tool_use":
			toolCalls++
		}
	}
	return blocks
}

// ThinkingBlockTracker accumulates streamed Claude thinking and redacted_thinking
// blocks so a complete block (exact text plus signature, or redacted data) can
// be handed to the Responses bridge when its content_block_stop arrives. A nil
// tracker is valid and records nothing.
type ThinkingBlockTracker struct {
	open      map[int]*model.ClaudeThinkingBlock
	oversized map[int]bool
	toolCalls int
	sawText   bool
}

// NewThinkingBlockTracker returns a tracker when the request feeds the
// Responses bridge, otherwise nil. The c parameter is the request context.
func NewThinkingBlockTracker(c *gin.Context) *ThinkingBlockTracker {
	if !ThinkingBlocksRequested(c) {
		return nil
	}
	return &ThinkingBlockTracker{open: map[int]*model.ClaudeThinkingBlock{}, oversized: map[int]bool{}}
}

// Observe records one decoded content_block_start or content_block_delta event.
// The index parameter is the Claude content index and event the decoded event.
// It returns nothing; unrelated events and nil receivers are ignored.
func (t *ThinkingBlockTracker) Observe(index int, event *StreamResponse) {
	if t == nil || event == nil {
		return
	}
	switch event.Type {
	case "content_block_start":
		if event.ContentBlock == nil {
			return
		}
		switch event.ContentBlock.Type {
		case model.ClaudeThinkingBlockTypeThinking, model.ClaudeThinkingBlockTypeRedacted:
			block := &model.ClaudeThinkingBlock{Type: event.ContentBlock.Type, ToolCallsBefore: t.toolCalls, AfterText: t.sawText}
			if event.ContentBlock.Thinking != nil {
				block.Thinking = *event.ContentBlock.Thinking
			}
			if event.ContentBlock.Signature != nil {
				block.Signature = *event.ContentBlock.Signature
			}
			if event.ContentBlock.Data != nil {
				block.Data = *event.ContentBlock.Data
			}
			t.open[index] = block
			t.checkSize(index)
		case "text":
			t.sawText = true
		case "tool_use":
			t.toolCalls++
		}
	case "content_block_delta":
		block := t.open[index]
		if block == nil || event.Delta == nil || t.oversized[index] {
			return
		}
		switch event.Delta.Type {
		case "thinking_delta":
			if event.Delta.Thinking != nil {
				block.Thinking += *event.Delta.Thinking
			}
		case "signature_delta":
			if event.Delta.Signature != nil {
				block.Signature += *event.Delta.Signature
			}
		}
		t.checkSize(index)
	}
}

// checkSize drops the accumulated state of a block that could no longer fit in
// an envelope, bounding memory for a misbehaving upstream. The index parameter
// is the Claude content index of the block to check.
func (t *ThinkingBlockTracker) checkSize(index int) {
	block := t.open[index]
	if block == nil {
		return
	}
	if len(block.Thinking)+len(block.Signature)+len(block.Data) > model.MaxClaudeThinkingEnvelopeBytes {
		t.oversized[index] = true
		block.Thinking, block.Signature, block.Data = "", "", ""
	}
}

// Complete closes the block at index and returns it when it is a replayable
// thinking block, otherwise nil. The index parameter is the Claude content index.
func (t *ThinkingBlockTracker) Complete(index int) *model.ClaudeThinkingBlock {
	if t == nil {
		return nil
	}
	block := t.open[index]
	oversized := t.oversized[index]
	delete(t.open, index)
	delete(t.oversized, index)
	if block == nil || oversized || !block.Replayable() {
		return nil
	}
	return block
}

// ThinkingBlockChunk wraps one completed thinking block in a Chat stream chunk
// for the Responses bridge. The block parameter must be replayable; the chunk
// carries no text, so streaming billing observes nothing twice.
func ThinkingBlockChunk(block model.ClaudeThinkingBlock) *openai.ChatCompletionsStreamResponse {
	chunk := &openai.ChatCompletionsStreamResponse{Choices: []openai.ChatCompletionsStreamResponseChoice{{}}}
	chunk.Choices[0].Delta.Role = "assistant"
	chunk.Choices[0].Delta.ClaudeThinkingBlocks = []model.ClaudeThinkingBlock{block}
	return chunk
}

// claudeReplayThinkingBlocks returns the replayable gateway thinking blocks of
// an assistant message when the target request may carry them. Parameters:
// message is the Chat message, modelName the upstream model, and thinking the
// effective thinking configuration. It returns nil when the model cannot think
// or thinking is explicitly disabled; callers then keep the legacy behavior.
func claudeReplayThinkingBlocks(message model.Message, modelName string, thinking *model.Thinking) []model.ClaudeThinkingBlock {
	if message.Role != "assistant" || len(message.ClaudeThinkingBlocks) == 0 {
		return nil
	}
	if !isModelSupportThinking(modelName) {
		return nil
	}
	if thinking != nil && strings.EqualFold(strings.TrimSpace(thinking.Type), "disabled") {
		return nil
	}
	blocks := make([]model.ClaudeThinkingBlock, 0, len(message.ClaudeThinkingBlocks))
	for _, block := range message.ClaudeThinkingBlocks {
		if block.Replayable() {
			blocks = append(blocks, block)
		}
	}
	if len(blocks) == 0 {
		return nil
	}
	return blocks
}

// claudeThinkingContent renders one replayable block exactly as Claude issued it.
// The block parameter must be replayable; the return value is the Claude block.
func claudeThinkingContent(block model.ClaudeThinkingBlock) Content {
	if block.Type == model.ClaudeThinkingBlockTypeRedacted {
		data := block.Data
		return Content{Type: model.ClaudeThinkingBlockTypeRedacted, Data: &data}
	}
	thinking, signature := block.Thinking, block.Signature
	return Content{Type: model.ClaudeThinkingBlockTypeThinking, Thinking: &thinking, Signature: &signature}
}

// insertClaudeThinkingBlocks places replayed thinking blocks back into an
// assistant message rebuilt from Chat fields (text first, then tool_use). Each
// block goes before the tool_use it preceded originally, after the text when
// text preceded it, and the blocks never change their relative order. The
// contents parameter is the rebuilt content and blocks the replayable blocks;
// the return value is a new slice when blocks is non-empty.
func insertClaudeThinkingBlocks(contents []Content, blocks []model.ClaudeThinkingBlock) []Content {
	if len(blocks) == 0 {
		return contents
	}
	out := make([]Content, 0, len(contents)+len(blocks))
	next, toolsSeen := 0, 0
	flush := func(ready func(model.ClaudeThinkingBlock) bool) {
		for next < len(blocks) && ready(blocks[next]) {
			out = append(out, claudeThinkingContent(blocks[next]))
			next++
		}
	}
	for _, content := range contents {
		if content.Type == "tool_use" {
			flush(func(block model.ClaudeThinkingBlock) bool { return block.ToolCallsBefore <= toolsSeen })
			out = append(out, content)
			toolsSeen++
			continue
		}
		flush(func(block model.ClaudeThinkingBlock) bool {
			return block.ToolCallsBefore < toolsSeen || (block.ToolCallsBefore == toolsSeen && !block.AfterText)
		})
		out = append(out, content)
	}
	flush(func(model.ClaudeThinkingBlock) bool { return true })
	return out
}
