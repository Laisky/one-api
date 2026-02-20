package openai

import (
	"strings"

	"github.com/songquanpeng/one-api/relay/model"
)

// countResponseAPIUnsupportedContentFields counts content block fields that
// are known to be rejected by OpenAI Response API and will be stripped during
// conversion.
func countResponseAPIUnsupportedContentFields(messages []model.Message) int {
	if len(messages) == 0 {
		return 0
	}

	count := 0
	for _, message := range messages {
		content, ok := message.Content.([]any)
		if !ok {
			continue
		}
		for _, rawItem := range content {
			itemMap, ok := rawItem.(map[string]any)
			if !ok {
				continue
			}

			contentType, _ := itemMap["type"].(string)
			for fieldName := range itemMap {
				if isAllowedResponseAPIContentField(contentType, fieldName) {
					continue
				}
				count++
			}
		}
	}

	return count
}

// isAllowedResponseAPIContentField reports whether a field is expected for the
// given Response API content item type.
func isAllowedResponseAPIContentField(contentType, fieldName string) bool {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "input_text", "output_text", "text":
		return fieldName == "type" || fieldName == "text"
	case "input_image", "image_url":
		return fieldName == "type" || fieldName == "image_url" || fieldName == "detail" || fieldName == "file_id"
	case "input_audio":
		return fieldName == "type" || fieldName == "input_audio"
	case "input_file":
		return fieldName == "type" || fieldName == "file_id" || fieldName == "file_url" || fieldName == "file_data" || fieldName == "filename"
	default:
		return fieldName == "type" || fieldName == "text"
	}
}
