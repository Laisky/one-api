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
		case "image":
			if source, ok := blockMap["source"].(map[string]any); ok {
				imageURL := relaymodel.ImageURL{}
				if mediaType, exists := source["media_type"]; exists {
					if data, ok := source["data"].(string); ok {
						imageURL.Url = fmt.Sprintf("data:%s;base64,%s", mediaType, data)
					}
				} else if url, ok := source["url"].(string); ok {
					imageURL.Url = url
				}
				imageURL.Detail, _ = source["detail"].(string)
				if imageURL.Url != "" {
					parts = append(parts, relaymodel.MessageContent{Type: "image_url", ImageURL: &imageURL})
				}
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

// appendClaudeJSONTokenPart appends a JSON-decoded native block as one private text part without changing the block.
func appendClaudeJSONTokenPart(parts []relaymodel.MessageContent, value any) []relaymodel.MessageContent {
	// Request blocks originate from JSON decoding and contain only JSON-serializable values.
	if encoded, err := json.Marshal(value); err == nil {
		text := string(encoded)
		return append(parts, relaymodel.MessageContent{Type: "text", Text: &text})
	}
	return parts
}
