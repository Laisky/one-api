package openai_compatible

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor/common/toolnamesafe"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// claudeChatTool tracks one streamed Chat Completions tool call. Its block is
// started only once the function name is known, because content_block_start
// must carry the name; argument fragments that arrive earlier are buffered.
type claudeChatTool struct {
	upstreamID   string
	name         string
	index        int
	started      bool
	pending      strings.Builder
	extraContent *relaymodel.ToolExtraContent
}

// claudeChatStreamTranslator converts OpenAI Chat Completions stream chunks
// into Claude content block events on a shared emitter.
//
// Text and thinking deltas reuse the currently open block of the same type and
// otherwise close it and start a new one, so text/thinking blocks never
// interleave. Tool calls are keyed by their stream index (falling back to id),
// because only the first fragment of an OpenAI tool call carries id and name;
// tool blocks stay open until the message finishes so late fragments from
// providers that interleave parallel calls are never lost.
type claudeChatStreamTranslator struct {
	c            *gin.Context
	em           *ClaudeStreamEmitter
	ids          *ClaudeToolIDAllocator
	current      int
	currentKind  string
	tools        map[string]*claudeChatTool
	toolOrder    []*claudeChatTool
	lastToolKey  string
	stopReason   string
	finishSeen   bool
	anonymousSeq int
}

// newClaudeChatStreamTranslator returns a translator writing to em.
func newClaudeChatStreamTranslator(c *gin.Context, em *ClaudeStreamEmitter, ids *ClaudeToolIDAllocator) *claudeChatStreamTranslator {
	return &claudeChatStreamTranslator{c: c, em: em, ids: ids, current: -1, tools: map[string]*claudeChatTool{}}
}

// ChatMessageThinking returns the reasoning text carried by a Chat message or
// stream delta, trying the thinking, reasoning_content, and reasoning fields in
// that order. It returns "" when none is set.
func ChatMessageThinking(delta relaymodel.Message) string {
	for _, candidate := range []*string{delta.Thinking, delta.ReasoningContent, delta.Reasoning} {
		if candidate != nil && *candidate != "" {
			return *candidate
		}
	}
	return ""
}

// ensureBlock returns the index of an open text or thinking block of kind,
// closing the current text/thinking block first when it has another kind.
func (t *claudeChatStreamTranslator) ensureBlock(kind string) int {
	if t.current >= 0 && t.currentKind == kind && t.em.IsOpen(t.current) {
		return t.current
	}
	t.closeCurrent()
	t.current = t.em.StartBlock(relaymodel.ClaudeContent{Type: kind})
	t.currentKind = kind
	return t.current
}

// closeCurrent stops the open text/thinking block, if any.
func (t *claudeChatStreamTranslator) closeCurrent() {
	if t.current >= 0 && t.em.IsOpen(t.current) {
		t.em.StopBlock(t.current)
	}
	t.current, t.currentKind = -1, ""
}

// consume translates one Chat Completions stream chunk.
func (t *claudeChatStreamTranslator) consume(chunk *ChatCompletionsStreamResponse) {
	for _, choice := range chunk.Choices {
		if thinking := ChatMessageThinking(choice.Delta); thinking != "" {
			t.em.ThinkingDelta(t.ensureBlock(claudeBlockThinking), thinking)
		}
		if choice.Delta.Signature != nil && *choice.Delta.Signature != "" {
			t.em.SignatureDelta(t.ensureBlock(claudeBlockThinking), *choice.Delta.Signature)
		}
		if blocks, native := claudeGoogleTextBlocks(choice.Delta); native {
			t.closeCurrent()
			for _, block := range blocks {
				index := t.em.StartBlock(block)
				t.em.TextDelta(index, block.Text)
				t.em.StopBlock(index)
			}
		} else if text := choice.Delta.StringContent(); text != "" {
			t.em.TextDelta(t.ensureBlock(claudeBlockText), text)
		}
		if len(choice.Delta.ToolCalls) > 0 {
			// Restore sanitized tool names before they reach the client.
			toolnamesafe.RestoreToolCallNames(t.c, choice.Delta.ToolCalls)
			for i := range choice.Delta.ToolCalls {
				t.consumeToolCall(&choice.Delta.ToolCalls[i])
			}
		}
		if choice.FinishReason != nil && strings.TrimSpace(*choice.FinishReason) != "" {
			t.stopReason = ClaudeStopReasonFromChatFinish(*choice.FinishReason)
			t.finishSeen = true
		}
	}
}

// toolKey identifies the tool call a fragment belongs to.
func (t *claudeChatStreamTranslator) toolKey(tc *relaymodel.Tool) string {
	switch {
	case tc.Index != nil:
		return "index:" + strconv.Itoa(*tc.Index)
	case tc.Id != "":
		return "id:" + tc.Id
	case t.lastToolKey != "":
		return t.lastToolKey
	default:
		t.anonymousSeq++
		return "seq:" + strconv.Itoa(t.anonymousSeq)
	}
}

// chatToolArguments renders a fragment's arguments as the raw JSON text the
// upstream streamed.
func chatToolArguments(tc *relaymodel.Tool) string {
	if tc.Function == nil || tc.Function.Arguments == nil {
		return ""
	}
	switch v := tc.Function.Arguments.(type) {
	case string:
		return v
	default:
		if b, err := json.Marshal(v); err == nil {
			return string(b)
		}
		return ""
	}
}

// consumeToolCall routes one tool call fragment to its tool_use block.
func (t *claudeChatStreamTranslator) consumeToolCall(tc *relaymodel.Tool) {
	key := t.toolKey(tc)
	tool, exists := t.tools[key]
	if exists && tc.Id != "" && tool.upstreamID != "" && tc.Id != tool.upstreamID {
		// A provider reused the stream index for a distinct call.
		exists = false
	}
	if !exists {
		tool = &claudeChatTool{index: -1}
		t.tools[key] = tool
		t.toolOrder = append(t.toolOrder, tool)
	}
	t.lastToolKey = key
	if tool.upstreamID == "" {
		tool.upstreamID = tc.Id
	}
	if tool.name == "" && tc.Function != nil {
		tool.name = tc.Function.Name
	}
	if tc.ExtraContent != nil {
		tool.extraContent = tc.ExtraContent
	}
	args := chatToolArguments(tc)
	if !tool.started {
		tool.pending.WriteString(args)
		if tool.name != "" {
			t.startTool(tool)
		}
		return
	}
	t.em.InputJSONDelta(tool.index, args)
}

// startTool opens the tool_use block and flushes buffered argument fragments.
func (t *claudeChatStreamTranslator) startTool(tool *claudeChatTool) {
	t.closeCurrent()
	tool.started = true
	tool.index = t.em.StartBlock(relaymodel.ClaudeContent{Type: claudeBlockToolUse, ID: t.ids.Allocate(tool.upstreamID), Name: tool.name, ExtraContent: tool.extraContent})
	if tool.pending.Len() > 0 {
		t.em.InputJSONDelta(tool.index, tool.pending.String())
		tool.pending.Reset()
	}
}

// flushPendingTools starts tool calls whose name never arrived so their
// buffered arguments are still delivered before the message finishes.
func (t *claudeChatStreamTranslator) flushPendingTools() {
	for _, tool := range t.toolOrder {
		if !tool.started {
			t.startTool(tool)
		}
	}
}

// finalStopReason returns the Claude stop_reason for the finished message.
func (t *claudeChatStreamTranslator) finalStopReason() string {
	return FinalizeClaudeStopReason(t.stopReason, t.em.HasToolUse())
}
