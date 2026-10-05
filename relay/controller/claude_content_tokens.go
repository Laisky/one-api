package controller

import (
	"encoding/json"
	"fmt"

	relaymodel "github.com/Laisky/one-api/relay/model"
)

// claudeContentTokenParts projects native Claude blocks into semantic text and image parts.
// A shared accumulator visits nested results once; other accepted blocks retain their complete JSON text.
func claudeContentTokenParts(blocks []any) []relaymodel.MessageContent {
	var parts []relaymodel.MessageContent
	stack := [][]any{blocks}
	for len(stack) > 0 {
		i := len(stack) - 1
		if len(stack[i]) == 0 {
			stack = stack[:i]
			continue
		}
		block := stack[i][0]
		stack[i] = stack[i][1:]
		blockMap, ok := block.(map[string]any)
		if !ok {
			continue
		}
		switch blockMap["type"] {
		case "text":
			if text, ok := blockMap["text"].(string); ok {
				parts = append(parts, relaymodel.MessageContent{Type: "text", Text: &text})
			}
		case "tool_result":
			switch result := blockMap["content"].(type) {
			case string:
				parts = append(parts, relaymodel.MessageContent{Type: "text", Text: &result})
			case []any:
				stack = append(stack, result)
			default:
				if result != nil {
					parts = appendClaudeJSONTokenPart(parts, result)
				}
			}
		case "document":
			if claudeOpaqueDocumentSource(blockMap) {
				parts = appendClaudeJSONTokenPart(parts, claudeDocumentTextMetadata(blockMap))
			} else {
				parts = appendClaudeJSONTokenPart(parts, blockMap)
			}
		case "image":
			// Share the converter's URL-versus-base64 source precedence with Chat counting.
			if imageURL, ok := claudeImageCountingURL(blockMap); ok {
				parts = append(parts, relaymodel.MessageContent{Type: relaymodel.ContentTypeImageURL, ImageURL: imageURL})
			}
		case "thinking", "redacted_thinking":
			// Keep the native replay policy: opaque thinking signatures are not ordinary prompt text.
		default:
			// Documents, search results, tool inputs, and future accepted blocks must not silently disappear.
			parts = appendClaudeJSONTokenPart(parts, blockMap)
		}
	}
	return parts
}

// appendClaudeJSONTokenPart appends a JSON-decoded native block as one private
// text part without changing the block, and returns the extended parts.
func appendClaudeJSONTokenPart(parts []relaymodel.MessageContent, value any) []relaymodel.MessageContent {
	encoded, err := json.Marshal(value)
	text := string(encoded)
	if err != nil {
		// Request blocks come from JSON decoding, so this is unreachable in
		// practice; still charge the value's printed form instead of dropping it.
		text = fmt.Sprint(value)
	}
	return append(parts, relaymodel.MessageContent{Type: "text", Text: &text})
}
