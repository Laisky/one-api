package openai_compatible

import (
	"encoding/json"
	"fmt"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/model"
)

// claudeImageBlockPart lowers one Claude image block to a Chat content part.
// Parameters: block is a decoded Claude content block of type "image".
// Returns the part and true when the source is one of the supported shapes
// (base64, url or file); otherwise false, matching the established lenient
// handling of malformed images in ordinary message content.
func claudeImageBlockPart(block map[string]any) (model.MessageContent, bool) {
	source, ok := block["source"].(map[string]any)
	if !ok {
		return model.MessageContent{}, false
	}
	sourceType, _ := source["type"].(string)
	switch sourceType {
	case "base64":
		mediaType, okType := source["media_type"].(string)
		data, okData := source["data"].(string)
		if !okType || !okData {
			return model.MessageContent{}, false
		}
		imageURL := model.ImageURL{Url: fmt.Sprintf("data:%s;base64,%s", mediaType, data)}
		if detail, ok := source["detail"].(string); ok {
			imageURL.Detail = detail
		}
		return model.MessageContent{Type: model.ContentTypeImageURL, ImageURL: &imageURL}, true
	case "url":
		url, ok := source["url"].(string)
		if !ok {
			return model.MessageContent{}, false
		}
		imageURL := model.ImageURL{Url: url}
		if detail, ok := source["detail"].(string); ok {
			imageURL.Detail = detail
		}
		return model.MessageContent{Type: model.ContentTypeImageURL, ImageURL: &imageURL}, true
	case "file":
		fileID, _ := source["file_id"].(string)
		fileData, _ := source["file_data"].(string)
		filename, _ := source["filename"].(string)
		if fileID == "" && fileData == "" {
			return model.MessageContent{}, false
		}
		return model.MessageContent{Type: model.ContentTypeFile, FileID: fileID, FileData: fileData, Filename: filename}, true
	}
	return model.MessageContent{}, false
}

// claudeToolResultBlocks returns the content blocks of a Claude tool_result.
// Parameters: content is the tool_result "content" value. Returns the array
// elements, a single object promoted to a one-element list, or nil for string
// and absent content.
func claudeToolResultBlocks(content any) []any {
	switch value := content.(type) {
	case []any:
		return value
	case map[string]any:
		return []any{value}
	}
	return nil
}

// isClaudeMediaBlock reports whether a Claude content block carries media that a text-only tool
// result cannot represent. Images, audio and video always do; a document does unless its source is
// Anthropic's plain-text or custom-content form, which keeps the established text flattening.
// Parameters: block is a decoded content block.
func isClaudeMediaBlock(block map[string]any) bool {
	switch blockType, _ := block["type"].(string); blockType {
	case "image", "video", "audio":
		return true
	case "document":
		source, _ := block["source"].(map[string]any)
		sourceType, _ := source["type"].(string)
		return sourceType != "text" && sourceType != "content"
	}
	return false
}

// claudeToolResultMediaPart decodes one media block nested in a tool_result.
// Parameters: block is a content block that satisfies isClaudeMediaBlock.
// Returns the typed part, or an error naming no payload when the block is malformed.
func claudeToolResultMediaPart(block map[string]any) (model.MessageContent, error) {
	if blockType, _ := block["type"].(string); blockType == "image" {
		part, ok := claudeImageBlockPart(block)
		if !ok {
			return model.MessageContent{}, errors.New("validation failed: invalid Claude tool_result image source")
		}
		return part, nil
	}
	part, _, err := model.DecodeClaudeMediaPart(block)
	if err != nil {
		return model.MessageContent{}, errors.Wrap(err, "validation failed: invalid Claude tool_result media")
	}
	return part, nil
}

// validateClaudeNativeToolResults rejects malformed media nested in tool_result blocks before
// native lowering, so a native transport never receives undecodable media as text.
// Parameters: blocks are one Claude message's content blocks. Returns a validation error or nil.
func validateClaudeNativeToolResults(blocks []any) error {
	for _, raw := range blocks {
		block, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if blockType, _ := block["type"].(string); blockType != "tool_result" {
			continue
		}
		for _, nested := range claudeToolResultBlocks(block["content"]) {
			item, ok := nested.(map[string]any)
			if !ok {
				continue
			}
			if !isClaudeMediaBlock(item) {
				continue
			}
			if _, err := claudeToolResultMediaPart(item); err != nil {
				return err
			}
		}
	}
	return nil
}

// claudeToolResultParts preserves the ordered typed parts of a media-bearing tool_result
// for native transports; the tool message Content keeps its established flattened text.
// Parameters: content is the tool_result "content" value, already validated by
// validateClaudeNativeToolResults. Returns nil when the result carries no media, so
// text-only results keep their existing single-string contract.
func claudeToolResultParts(content any) []model.MessageContent {
	blocks := claudeToolResultBlocks(content)
	hasMedia := false
	parts := make([]model.MessageContent, 0, len(blocks))
	for _, raw := range blocks {
		block, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		blockType, _ := block["type"].(string)
		switch {
		case isClaudeMediaBlock(block):
			part, err := claudeToolResultMediaPart(block)
			if err != nil {
				// Validation runs first; keep the result non-text so a native consumer still fails closed.
				part = model.MessageContent{Type: model.ContentTypeFile}
			}
			hasMedia = true
			parts = append(parts, part)
		case blockType == model.ContentTypeText:
			if text, ok := block["text"].(string); ok {
				parts = append(parts, model.MessageContent{Type: model.ContentTypeText, Text: &text})
			}
		default:
			// Mirrors extractClaudeToolResultContent, which flattens the same block into Content; a
			// decoded JSON map always re-encodes, and a failure would only omit this text part.
			if encoded, err := json.Marshal(block); err == nil {
				text := string(encoded)
				parts = append(parts, model.MessageContent{Type: model.ContentTypeText, Text: &text})
			}
		}
	}
	if !hasMedia {
		return nil
	}
	return parts
}
