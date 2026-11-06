package model

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/Laisky/errors/v2"
)

const (
	// ClaudeThinkingBlockTypeThinking identifies a signed Claude thinking block.
	ClaudeThinkingBlockTypeThinking = "thinking"
	// ClaudeThinkingBlockTypeRedacted identifies an encrypted Claude redacted_thinking block.
	ClaudeThinkingBlockTypeRedacted = "redacted_thinking"

	// ClaudeThinkingEnvelopePrefix versions the opaque Responses encrypted_content
	// envelope that carries one Claude thinking block through a Responses client.
	ClaudeThinkingEnvelopePrefix = "oneapi.claude.v1."

	// MaxClaudeThinkingEnvelopeBytes bounds one encoded envelope before any
	// decoding work or allocation. A maximal 128K-token thinking block plus its
	// signature stays far below this limit after base64 expansion.
	MaxClaudeThinkingEnvelopeBytes = 4 << 20

	// maxClaudeThinkingAnchor bounds the decoded tool-call anchor so a forged
	// envelope cannot claim an absurd position inside an assistant message.
	maxClaudeThinkingAnchor = 1 << 16
)

// ClaudeThinkingBlock is one Claude thinking or redacted_thinking content block
// carried verbatim between the Claude adaptor and the Responses API bridge.
//
// Claude's preserved-thinking contract requires every block to be passed back
// unchanged (exact text plus signature, or the redacted data) in its original
// position inside the assistant turn. ToolCallsBefore and AfterText record that
// position relative to the Chat-visible blocks: the number of tool_use blocks
// and whether any text block preceded this block in the original message.
type ClaudeThinkingBlock struct {
	Type            string `json:"type"`
	Thinking        string `json:"thinking,omitempty"`
	Signature       string `json:"signature,omitempty"`
	Data            string `json:"data,omitempty"`
	ToolCallsBefore int    `json:"tool_calls_before,omitempty"`
	AfterText       bool   `json:"after_text,omitempty"`
}

// claudeThinkingEnvelopeWire is the compact JSON payload inside an envelope.
type claudeThinkingEnvelopeWire struct {
	Thinking        string `json:"t,omitempty"`
	Signature       string `json:"s,omitempty"`
	Redacted        string `json:"r,omitempty"`
	ToolCallsBefore int    `json:"k,omitempty"`
	AfterText       bool   `json:"a,omitempty"`
}

// claudeThinkingEnvelopeKeys lists the exact, case-sensitive envelope keys.
var claudeThinkingEnvelopeKeys = map[string]struct{}{"t": {}, "s": {}, "r": {}, "k": {}, "a": {}}

// Replayable reports whether block carries provider state Claude can verify:
// a thinking block with a signature or a redacted_thinking block with data.
// The receiver is the block to inspect; the return value is false otherwise.
func (block ClaudeThinkingBlock) Replayable() bool {
	switch block.Type {
	case ClaudeThinkingBlockTypeThinking:
		return block.Signature != "" && block.Data == ""
	case ClaudeThinkingBlockTypeRedacted:
		return block.Data != "" && block.Signature == "" && block.Thinking == ""
	default:
		return false
	}
}

// EncodeClaudeThinkingEnvelope serializes block into the versioned opaque
// envelope placed in a Responses reasoning item's encrypted_content. The block
// parameter must be Replayable. It returns the envelope or a wrapped error.
func EncodeClaudeThinkingEnvelope(block ClaudeThinkingBlock) (string, error) {
	if !block.Replayable() {
		return "", errors.New("claude thinking block is not replayable")
	}
	wire := claudeThinkingEnvelopeWire{ToolCallsBefore: block.ToolCallsBefore, AfterText: block.AfterText}
	if block.Type == ClaudeThinkingBlockTypeRedacted {
		wire.Redacted = block.Data
	} else {
		wire.Thinking, wire.Signature = block.Thinking, block.Signature
	}
	payload, err := json.Marshal(wire)
	if err != nil {
		return "", errors.Wrap(err, "encode claude thinking envelope")
	}
	envelope := ClaudeThinkingEnvelopePrefix + base64.RawURLEncoding.EncodeToString(payload)
	if len(envelope) > MaxClaudeThinkingEnvelopeBytes {
		return "", errors.New("claude thinking envelope exceeds size limit")
	}
	return envelope, nil
}

// IsClaudeThinkingEnvelope reports whether value claims the gateway envelope
// format by prefix only. It performs no decoding, so callers that act on the
// content must still call DecodeClaudeThinkingEnvelope.
func IsClaudeThinkingEnvelope(value string) bool {
	return strings.HasPrefix(value, ClaudeThinkingEnvelopePrefix)
}

// DecodeClaudeThinkingEnvelope strictly decodes an envelope produced by
// EncodeClaudeThinkingEnvelope. The value parameter is untrusted client input.
// It returns the reconstructed block, or a wrapped error for foreign, oversized,
// non-canonical or malformed envelopes; callers must then ignore the envelope.
// The decoded text and signature are always taken together from the envelope,
// so a signature is never paired with text the gateway did not receive with it.
func DecodeClaudeThinkingEnvelope(value string) (ClaudeThinkingBlock, error) {
	if !IsClaudeThinkingEnvelope(value) {
		return ClaudeThinkingBlock{}, errors.New("not a claude thinking envelope")
	}
	// Bound the untrusted value before decoding so it cannot drive allocation.
	if len(value) > MaxClaudeThinkingEnvelopeBytes {
		return ClaudeThinkingBlock{}, errors.New("claude thinking envelope exceeds size limit")
	}
	payload, err := base64.RawURLEncoding.Strict().DecodeString(value[len(ClaudeThinkingEnvelopePrefix):])
	if err != nil {
		return ClaudeThinkingBlock{}, errors.Wrap(err, "decode claude thinking envelope encoding")
	}
	wire, err := decodeClaudeThinkingEnvelopeWire(payload)
	if err != nil {
		return ClaudeThinkingBlock{}, err
	}
	if wire.ToolCallsBefore < 0 || wire.ToolCallsBefore > maxClaudeThinkingAnchor {
		return ClaudeThinkingBlock{}, errors.New("claude thinking envelope anchor is out of range")
	}
	block := ClaudeThinkingBlock{ToolCallsBefore: wire.ToolCallsBefore, AfterText: wire.AfterText}
	switch {
	case wire.Redacted != "" && wire.Signature == "" && wire.Thinking == "":
		block.Type, block.Data = ClaudeThinkingBlockTypeRedacted, wire.Redacted
	case wire.Signature != "" && wire.Redacted == "":
		block.Type, block.Thinking, block.Signature = ClaudeThinkingBlockTypeThinking, wire.Thinking, wire.Signature
	default:
		return ClaudeThinkingBlock{}, errors.New("claude thinking envelope has no replayable state")
	}
	return block, nil
}

// decodeClaudeThinkingEnvelopeWire decodes payload as one JSON object whose keys
// are exactly the lowercase envelope keys. Go's case-insensitive struct decoding
// is bypassed so a polyglot payload cannot smuggle alternate spellings.
// It returns the decoded wire value or a wrapped validation error.
func decodeClaudeThinkingEnvelopeWire(payload []byte) (claudeThinkingEnvelopeWire, error) {
	var wire claudeThinkingEnvelopeWire
	decoder := json.NewDecoder(bytes.NewReader(payload))
	var fields map[string]json.RawMessage
	if err := decoder.Decode(&fields); err != nil {
		return wire, errors.Wrap(err, "decode claude thinking envelope payload")
	}
	if fields == nil {
		return wire, errors.New("claude thinking envelope payload must be an object")
	}
	if decoder.More() {
		return wire, errors.New("claude thinking envelope payload has trailing data")
	}
	for key := range fields {
		if _, ok := claudeThinkingEnvelopeKeys[key]; !ok {
			return wire, errors.New("claude thinking envelope payload has an unknown field")
		}
	}
	targets := map[string]any{
		"t": &wire.Thinking, "s": &wire.Signature, "r": &wire.Redacted,
		"k": &wire.ToolCallsBefore, "a": &wire.AfterText,
	}
	for key, raw := range fields {
		if err := json.Unmarshal(raw, targets[key]); err != nil {
			return claudeThinkingEnvelopeWire{}, errors.Wrap(err, "decode claude thinking envelope field")
		}
	}
	return wire, nil
}

// HasReplayableClaudeThinkingBlocks reports whether any block in blocks can be
// replayed to Claude. The blocks parameter may be nil.
func HasReplayableClaudeThinkingBlocks(blocks []ClaudeThinkingBlock) bool {
	for _, block := range blocks {
		if block.Replayable() {
			return true
		}
	}
	return false
}
