package model

import (
	"bytes"
	"encoding/json"

	"github.com/Laisky/errors/v2"
)

// StripClaudeMessagesProviderMetadata removes every carrier from a raw Claude messages array.
// The Anthropic Messages wire (Anthropic, Bedrock, Vertex Claude and compatible hosts) defines no
// extra_content field and rejects unknown content-block fields. An assistant text block that only
// existed to carry metadata (empty or absent text plus a carrier, as emitted for signature-only Gemini
// parts) is removed together with its carrier, because Anthropic also rejects empty text blocks; an
// assistant message left without content is removed, as the unsigned-thinking sanitizer does.
// Parameters: messages is the raw "messages" value. Returns the value itself when unchanged, the
// count of removed carriers, and an error for an invalid subtree.
func StripClaudeMessagesProviderMetadata(messages json.RawMessage) (json.RawMessage, int, error) {
	trimmed := bytes.TrimSpace(messages)
	if len(trimmed) == 0 || trimmed[0] != '[' || !mayHoldCarrier(trimmed) {
		return messages, 0, nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(trimmed, &items); err != nil {
		return nil, 0, errors.Wrap(err, "decode Claude messages for provider metadata")
	}
	kept := make([]json.RawMessage, 0, len(items))
	changed := 0
	for i, item := range items {
		scoped, count, keep, err := stripClaudeMessageCarriers(item)
		if err != nil {
			return nil, 0, errors.Wrapf(err, "message %d", i)
		}
		changed += count
		if keep {
			kept = append(kept, scoped)
		}
	}
	if changed == 0 {
		return messages, 0, nil
	}
	encoded, err := encodeRawJSONValue(kept)
	if err != nil {
		return nil, 0, err
	}
	return encoded, changed, nil
}

// stripClaudeMessageCarriers removes the carriers of one message's content blocks.
// Returns the message (unchanged bytes when nothing changed), the removed-carrier count, whether the
// message is kept, and a decode error.
func stripClaudeMessageCarriers(item json.RawMessage) (json.RawMessage, int, bool, error) {
	trimmed := bytes.TrimSpace(item)
	if len(trimmed) == 0 || trimmed[0] != '{' || !mayHoldCarrier(trimmed) {
		return item, 0, true, nil
	}
	var message map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &message); err != nil {
		return nil, 0, false, errors.Wrap(err, "decode Claude message")
	}
	content := bytes.TrimSpace(message["content"])
	if len(content) == 0 || content[0] != '[' || !mayHoldCarrier(content) {
		return item, 0, true, nil
	}
	var blocks []json.RawMessage
	if err := json.Unmarshal(content, &blocks); err != nil {
		return nil, 0, false, errors.Wrap(err, "decode Claude content blocks")
	}
	var role string
	if raw, present := message["role"]; present {
		if err := json.Unmarshal(raw, &role); err != nil {
			return nil, 0, false, errors.Wrap(err, "decode Claude message role")
		}
	}
	keptBlocks := make([]json.RawMessage, 0, len(blocks))
	changed := 0
	for i, block := range blocks {
		scoped, count, keep, err := stripClaudeBlockCarriers(block, role == "assistant")
		if err != nil {
			return nil, 0, false, errors.Wrapf(err, "content block %d", i)
		}
		changed += count
		if keep {
			keptBlocks = append(keptBlocks, scoped)
		}
	}
	if changed == 0 {
		return item, 0, true, nil
	}
	if len(keptBlocks) == 0 && role == "assistant" {
		return nil, changed, false, nil
	}
	encodedBlocks, err := encodeRawJSONValue(keptBlocks)
	if err != nil {
		return nil, 0, false, err
	}
	message["content"] = encodedBlocks
	encoded, err := encodeRawJSONValue(message)
	if err != nil {
		return nil, 0, false, err
	}
	return encoded, changed, true, nil
}

// stripClaudeBlockCarriers removes one content block's carriers. Parameters: dropEmptyText allows a
// metadata-only empty text block to be removed. Returns the block, the removed-carrier count, whether
// the block is kept, and a decode error.
func stripClaudeBlockCarriers(block json.RawMessage, dropEmptyText bool) (json.RawMessage, int, bool, error) {
	trimmed := bytes.TrimSpace(block)
	if len(trimmed) == 0 || trimmed[0] != '{' || !mayHoldCarrier(trimmed) {
		return block, 0, true, nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &object); err != nil {
		return nil, 0, false, errors.Wrap(err, "decode Claude content block")
	}
	count, err := scopeRawCarriers(object, ProviderMetadataNone)
	if err != nil {
		return nil, 0, false, err
	}
	if count == 0 {
		return block, 0, true, nil
	}
	if dropEmptyText && isEmptyClaudeTextBlock(object) {
		return nil, count, false, nil
	}
	encoded, err := encodeRawJSONValue(object)
	if err != nil {
		return nil, 0, false, err
	}
	return encoded, count, true, nil
}

// isEmptyClaudeTextBlock reports whether a decoded block is a text block with absent or empty text.
func isEmptyClaudeTextBlock(object map[string]json.RawMessage) bool {
	var kind, text string
	if json.Unmarshal(object["type"], &kind) != nil || kind != "text" {
		return false
	}
	raw, present := object["text"]
	if !present {
		return true
	}
	return json.Unmarshal(raw, &text) == nil && text == ""
}
