package openai_compatible

import (
	"encoding/json"
	"io"

	"github.com/Laisky/errors/v2"

	commonsse "github.com/Laisky/one-api/common/sse"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// Claude content block and delta type names used by the Messages stream.
const (
	claudeBlockText     = "text"
	claudeBlockThinking = "thinking"
	claudeBlockToolUse  = "tool_use"
)

// claudeEmitterBlock tracks one content block of the message being rendered.
type claudeEmitterBlock struct {
	blockType string
	open      bool
}

// ClaudeStreamEmitter renders one Anthropic Messages streaming response as a
// spec-conformant SSE event sequence: a single message_start carrying a full
// Message envelope (id, model, empty content, null stop_reason/stop_sequence,
// usage), dense content block indices assigned in start order, deltas only for
// open blocks of a matching type, content_block_stop exactly once per block,
// then one message_delta with a non-null stop_reason followed by message_stop.
// Every event is written as "event: <type>" plus "data: <json>" lines.
//
// The emitter enforces the ordering invariants itself: deltas or stops aimed at
// a closed or unknown block are dropped and counted, and Finish closes every
// still-open block in ascending index order. It is request-local and not
// goroutine-safe.
type ClaudeStreamEmitter struct {
	out      io.Writer
	flush    func()
	started  bool
	finished bool
	failed   bool
	blocks   []*claudeEmitterBlock
	dropped  int
	err      error
}

// NewClaudeStreamEmitter returns an emitter writing SSE events to out. flush,
// when non-nil, is invoked after each event so the client sees it immediately.
func NewClaudeStreamEmitter(out io.Writer, flush func()) *ClaudeStreamEmitter {
	return &ClaudeStreamEmitter{out: out, flush: flush}
}

// writeEvent marshals payload and writes it as one SSE event whose event name
// equals its type. The first write error is retained and later writes are
// skipped so a disconnected client does not produce repeated failures.
func (e *ClaudeStreamEmitter) writeEvent(eventType string, payload any) {
	if e.err != nil {
		return
	}
	data, err := json.Marshal(payload)
	if err != nil {
		e.err = errors.Wrapf(err, "marshal Claude %s event", eventType)
		return
	}
	frame, err := commonsse.EncodeFrame(eventType, data)
	if err != nil {
		e.err = errors.Wrapf(err, "encode Claude %s event", eventType)
		return
	}
	if _, err := e.out.Write(frame); err != nil {
		e.err = errors.Wrapf(err, "write Claude %s event", eventType)
		return
	}
	if e.flush != nil {
		e.flush()
	}
}

// Start emits message_start once. id falls back to a generated Claude-style
// message id, and usage is the initial (cumulative) usage snapshot.
func (e *ClaudeStreamEmitter) Start(id, model string, usage relaymodel.ClaudeUsage) {
	if e.started {
		return
	}
	e.started = true
	e.writeEvent("message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":            ClaudeMessageID(id),
			"type":          "message",
			"role":          "assistant",
			"model":         model,
			"content":       []any{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage":         usage,
		},
	})
}

// claudeStartPlaceholder returns the content_block_start payload for block:
// the block's identity with empty streaming fields, as the SDKs expect.
func claudeStartPlaceholder(block relaymodel.ClaudeContent) map[string]any {
	switch block.Type {
	case claudeBlockText:
		return map[string]any{"type": claudeBlockText, "text": ""}
	case claudeBlockThinking:
		return map[string]any{"type": claudeBlockThinking, "thinking": "", "signature": ""}
	case claudeBlockToolUse:
		return map[string]any{"type": claudeBlockToolUse, "id": block.ID, "name": block.Name, "input": map[string]any{}}
	default:
		return map[string]any{"type": block.Type}
	}
}

// StartBlock emits content_block_start for a text, thinking, or tool_use block
// and returns its index (the number of blocks started before it). Only the
// block's type, and for tool_use its id and name, are used.
func (e *ClaudeStreamEmitter) StartBlock(block relaymodel.ClaudeContent) int {
	index := len(e.blocks)
	e.blocks = append(e.blocks, &claudeEmitterBlock{blockType: block.Type, open: true})
	e.writeEvent("content_block_start", map[string]any{
		"type":          "content_block_start",
		"index":         index,
		"content_block": claudeStartPlaceholder(block),
	})
	return index
}

// openBlock returns the open block at index when it has the wanted type, or
// nil after counting a dropped event.
func (e *ClaudeStreamEmitter) openBlock(index int, blockType string) *claudeEmitterBlock {
	if index < 0 || index >= len(e.blocks) || !e.blocks[index].open || e.blocks[index].blockType != blockType {
		e.dropped++
		return nil
	}
	return e.blocks[index]
}

// IsOpen reports whether the block at index has started and not yet stopped.
func (e *ClaudeStreamEmitter) IsOpen(index int) bool {
	return index >= 0 && index < len(e.blocks) && e.blocks[index].open
}

// TextDelta appends text to the open text block at index.
func (e *ClaudeStreamEmitter) TextDelta(index int, text string) {
	block := e.openBlock(index, claudeBlockText)
	if block == nil || text == "" {
		return
	}
	e.writeEvent("content_block_delta", map[string]any{
		"type": "content_block_delta", "index": index,
		"delta": map[string]any{"type": "text_delta", "text": text},
	})
}

// ThinkingDelta appends reasoning text to the open thinking block at index.
func (e *ClaudeStreamEmitter) ThinkingDelta(index int, thinking string) {
	block := e.openBlock(index, claudeBlockThinking)
	if block == nil || thinking == "" {
		return
	}
	e.writeEvent("content_block_delta", map[string]any{
		"type": "content_block_delta", "index": index,
		"delta": map[string]any{"type": "thinking_delta", "thinking": thinking},
	})
}

// SignatureDelta attaches a signature to the open thinking block at index.
func (e *ClaudeStreamEmitter) SignatureDelta(index int, signature string) {
	block := e.openBlock(index, claudeBlockThinking)
	if block == nil || signature == "" {
		return
	}
	e.writeEvent("content_block_delta", map[string]any{
		"type": "content_block_delta", "index": index,
		"delta": map[string]any{"type": "signature_delta", "signature": signature},
	})
}

// InputJSONDelta appends a tool input JSON fragment to the open tool_use block.
func (e *ClaudeStreamEmitter) InputJSONDelta(index int, partial string) {
	block := e.openBlock(index, claudeBlockToolUse)
	if block == nil || partial == "" {
		return
	}
	e.writeEvent("content_block_delta", map[string]any{
		"type": "content_block_delta", "index": index,
		"delta": map[string]any{"type": "input_json_delta", "partial_json": partial},
	})
}

// StopBlock emits content_block_stop for the open block at index; stopping an
// unknown or already-stopped block is a counted no-op.
func (e *ClaudeStreamEmitter) StopBlock(index int) {
	if !e.IsOpen(index) {
		e.dropped++
		return
	}
	e.blocks[index].open = false
	e.writeEvent("content_block_stop", map[string]any{"type": "content_block_stop", "index": index})
}

// Finish closes every open block in ascending index order and emits the final
// message_delta (with stopReason, a null stop_sequence, and the cumulative
// usage) followed by message_stop. An empty stopReason renders as end_turn.
func (e *ClaudeStreamEmitter) Finish(stopReason string, usage relaymodel.ClaudeUsage) {
	if e.finished || e.failed {
		return
	}
	for index := range e.blocks {
		if e.blocks[index].open {
			e.StopBlock(index)
		}
	}
	if stopReason == "" {
		stopReason = ClaudeStopEndTurn
	}
	e.finished = true
	e.writeEvent("message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": stopReason, "stop_sequence": nil},
		"usage": usage,
	})
	e.writeEvent("message_stop", map[string]any{"type": "message_stop"})
}

// Fail emits an Anthropic error event (the upstream failed mid-stream) and
// suppresses any later Finish, so the client never mistakes a failed upstream
// response for a complete answer.
func (e *ClaudeStreamEmitter) Fail(errorType, message string) {
	if e.finished || e.failed {
		return
	}
	e.failed = true
	if errorType == "" {
		errorType = "api_error"
	}
	e.writeEvent("error", map[string]any{
		"type":  "error",
		"error": map[string]any{"type": errorType, "message": message},
	})
}

// HasToolUse reports whether any tool_use block was started.
func (e *ClaudeStreamEmitter) HasToolUse() bool {
	for _, block := range e.blocks {
		if block.blockType == claudeBlockToolUse {
			return true
		}
	}
	return false
}

// Finished reports whether message_stop was emitted.
func (e *ClaudeStreamEmitter) Finished() bool { return e.finished }

// Failed reports whether an error event terminated the stream.
func (e *ClaudeStreamEmitter) Failed() bool { return e.failed }

// Dropped returns how many out-of-order deltas or stops were suppressed.
func (e *ClaudeStreamEmitter) Dropped() int { return e.dropped }

// Err returns the first marshal or write error, if any.
func (e *ClaudeStreamEmitter) Err() error { return e.err }
