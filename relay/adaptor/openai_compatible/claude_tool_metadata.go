package openai_compatible

import (
	"encoding/json"

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
	extra, ok := block["extra_content"].(map[string]any)
	if !ok {
		return nil
	}
	var result *model.ToolExtraContent
	if google, ok := extra["google"].(map[string]any); ok {
		signature, _ := google["thought_signature"].(string)
		thought, _ := google["thought"].(bool)
		result = model.GooglePartMetadata(signature, thought)
	}
	if oneapi, ok := extra["oneapi"].(map[string]any); ok {
		if present, ok := oneapi["gemini_text_present"].(bool); ok {
			result = model.WithGeminiTextPresent(result, present)
		}
		if raw, present := oneapi["gemini_part_index"]; present && raw != nil {
			// The native request validator already checked the typed integer representation.
			index := -1
			switch number := raw.(type) {
			case float64:
				index = int(number)
			case int:
				index = number
			case int64:
				index = int(number)
			case json.Number:
				if parsed, err := number.Int64(); err == nil {
					index = int(parsed)
				}
			}
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
			raw, present := block["extra_content"]
			if !present || raw == nil {
				continue
			}
			encoded, err := json.Marshal(raw)
			if err != nil {
				return errors.New("validation failed: invalid native Claude provider metadata")
			}
			var metadata model.ToolExtraContent
			if err := json.Unmarshal(encoded, &metadata); err != nil {
				return errors.New("validation failed: invalid native Claude provider metadata")
			}
		}
	}
	return nil
}
