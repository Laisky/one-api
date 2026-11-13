package openai_compatible

import (
	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/model"
)

// claudeNativePartMetadata reads the gateway's Google state extension on a native content block.
// Parameters: block is caller-owned content and native selects a native-capable lowering path.
// Returns: typed opaque metadata without changing the source; unrelated providers retain their
// existing conversion contract. This is a gateway extension, not an Anthropic signature.
func claudeNativePartMetadata(block map[string]any, native bool) *model.ToolExtraContent {
	if !native {
		return nil
	}
	// Native routes run validateClaudeNativeMetadata over every block first and reject a malformed
	// carrier there, so a decode error cannot occur here; nil keeps the established "no metadata" result.
	extra, err := model.DecodeToolExtraContent(block["extra_content"])
	if err != nil || extra == nil {
		return nil
	}
	signature, thought := model.GoogleMetadata(extra)
	result := model.GooglePartMetadata(signature, thought)
	if extra.OneAPI != nil {
		if extra.OneAPI.GeminiTextPresent != nil {
			result = model.WithGeminiTextPresent(result, *extra.OneAPI.GeminiTextPresent)
		}
		if index, positioned := model.GeminiPartIndex(extra); positioned {
			result = model.WithGeminiPartIndex(result, index)
		}
	}
	return result
}

// claudeGoogleTextBlocks projects an explicitly metadata-bearing Chat message into exact text blocks.
// Parameters: message is a caller-owned provider response. Returns ordered blocks and whether the Google path applies.
func claudeGoogleTextBlocks(message model.Message) ([]model.ClaudeContent, bool) {
	parts, err := message.ParseContentChecked()
	if err != nil {
		return nil, false
	}
	hasMetadata := message.ExtraContent != nil
	for _, part := range parts {
		if part.ExtraContent != nil {
			hasMetadata = true
		}
	}
	if !hasMetadata {
		return nil, false
	}
	var blocks []model.ClaudeContent
	for _, part := range parts {
		if part.Type != model.ContentTypeText {
			continue
		}
		text := ""
		if part.Text != nil {
			text = *part.Text
		}
		metadata := part.ExtraContent
		if metadata == nil && len(parts) == 1 {
			metadata = message.ExtraContent
		}
		blocks = append(blocks, model.ClaudeContent{Type: "text", Text: text, ExtraContent: metadata})
	}
	if len(parts) == 0 && message.ExtraContent != nil {
		blocks = append(blocks, model.ClaudeContent{Type: "text", Text: "", ExtraContent: message.ExtraContent})
	}
	return blocks, true
}

// recordClaudeNativePart records the next content or call position on native Claude lowering.
// Parameters: pending owns the message, tool selects its call list, and native enables this
// internal order carrier. It returns no values and does not change public wire metadata.
func recordClaudeNativePart(pending *pendingOpenAIMessage, tool bool, native ...bool) {
	if len(native) == 0 || !native[0] {
		return
	}
	reference := model.NativePartReference{}
	if tool {
		index := len(pending.message.ToolCalls)
		reference.ToolCallIndex = &index
	} else {
		index := len(pending.contentParts)
		reference.ContentIndex = &index
	}
	pending.message.NativePartOrder = append(pending.message.NativePartOrder, reference)
}

// validateClaudeNativeMetadata validates the typed optional provider extension before native lowering.
// Parameters: request is caller-owned Claude history. Returns a sanitized error for malformed
// metadata, including fractional/overflow position values, without logging opaque signatures.
func validateClaudeNativeMetadata(request *model.ClaudeRequest) error {
	for _, message := range request.Messages {
		blocks, ok := message.Content.([]any)
		if !ok {
			continue
		}
		for _, item := range blocks {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if _, err := model.DecodeToolExtraContent(block["extra_content"]); err != nil {
				// Client-facing text stays fixed: decoder details name Go types, never useful to callers.
				return errors.New("validation failed: invalid native Claude provider metadata")
			}
		}
	}
	return nil
}
