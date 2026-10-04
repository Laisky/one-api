package controller

import (
	"encoding/json"
	"fmt"

	relaymodel "github.com/Laisky/one-api/relay/model"
)

// claudeContentTokenParts projects structured Claude content into semantic text and image parts.
// It includes tool inputs and recursively counts tool results without changing the provider payload.
func claudeContentTokenParts(blocks []any) []relaymodel.MessageContent {
	var contentParts []relaymodel.MessageContent
	for _, block := range blocks {
		if blockMap, ok := block.(map[string]any); ok {
			if blockType, exists := blockMap["type"]; exists {
				switch blockType {
				case "text":
					if text, exists := blockMap["text"]; exists {
						if textStr, ok := text.(string); ok {
							contentParts = append(contentParts, relaymodel.MessageContent{
								Type: "text",
								Text: &textStr,
							})
						}
					}
				case "tool_use":
					// Tool names and arguments are prompt content, even when there is no text block.
					if encoded, err := json.Marshal(blockMap); err == nil {
						text := string(encoded)
						contentParts = append(contentParts, relaymodel.MessageContent{Type: "text", Text: &text})
					}
				case "tool_result":
					// Reuse the semantic projection for nested text and images instead of tokenizing base64 JSON.
					switch result := blockMap["content"].(type) {
					case string:
						contentParts = append(contentParts, relaymodel.MessageContent{Type: "text", Text: &result})
					case []any:
						contentParts = append(contentParts, claudeContentTokenParts(result)...)
					default:
						if result != nil {
							if encoded, err := json.Marshal(result); err == nil {
								text := string(encoded)
								contentParts = append(contentParts, relaymodel.MessageContent{Type: "text", Text: &text})
							}
						}
					}
				case "image":
					if source, exists := blockMap["source"]; exists {
						if sourceMap, ok := source.(map[string]any); ok {
							imageURL := relaymodel.ImageURL{}
							if mediaType, exists := sourceMap["media_type"]; exists {
								if data, exists := sourceMap["data"]; exists {
									if dataStr, ok := data.(string); ok {
										// Convert to data URL format for token counting
										imageURL.Url = fmt.Sprintf("data:%s;base64,%s", mediaType, dataStr)
									}
								}
							} else if url, exists := sourceMap["url"]; exists {
								if urlStr, ok := url.(string); ok {
									imageURL.Url = urlStr
								}
							}
							if detail, ok := sourceMap["detail"].(string); ok {
								imageURL.Detail = detail
							}
							if imageURL.Url != "" {
								contentParts = append(contentParts, relaymodel.MessageContent{
									Type:     "image_url",
									ImageURL: &imageURL,
								})
							}
						}
					}
				}
			}
		}
	}
	return contentParts
}
