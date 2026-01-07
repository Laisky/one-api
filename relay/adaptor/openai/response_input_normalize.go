package openai

import "strings"

// ResponseAPIInputContentNormalizationStats reports how many legacy content-type
// values were rewritten when normalizing Response API input items.
type ResponseAPIInputContentNormalizationStats struct {
	AssistantInputTextFixed     int
	NonAssistantOutputTextFixed int
	ReasoningSummaryFixed       int
}

// NormalizeResponseAPIInputContentTypes rewrites Response API message content item
// types for backward compatibility with clients that send OpenAI-incompatible
// values.
//
// OpenAI's Responses API expects:
// - user/system/developer message text content items: type="input_text"
// - assistant message text content items: type="output_text" (or "refusal")
//
// Some clients incorrectly send assistant history with type="input_text" which
// OpenAI rejects with 400 (invalid_value). This function fixes those cases in
// place while leaving non-text/tool content items unchanged.
func NormalizeResponseAPIInputContentTypes(input *ResponseAPIInput) (ResponseAPIInputContentNormalizationStats, bool) {
	var stats ResponseAPIInputContentNormalizationStats
	if input == nil || len(*input) == 0 {
		return stats, false
	}

	changed := false
	for i := range *input {
		itemMap, ok := (*input)[i].(map[string]any)
		if !ok || itemMap == nil {
			continue
		}
		role, _ := itemMap["role"].(string)
		role = strings.ToLower(strings.TrimSpace(role))
		if role == "" {
			continue
		}
		content, ok := itemMap["content"].([]any)
		if !ok || len(content) == 0 {
			continue
		}

		contentChanged := false
		for j := range content {
			part, ok := content[j].(map[string]any)
			if !ok || part == nil {
				continue
			}
			typ, _ := part["type"].(string)
			typ = strings.ToLower(strings.TrimSpace(typ))
			if typ == "" {
				continue
			}

			if role == "assistant" {
				// Assistant message history uses output types.
				if typ == "input_text" || typ == "text" {
					part["type"] = "output_text"
					stats.AssistantInputTextFixed++
					content[j] = part
					contentChanged = true
				}
				continue
			}

			// Non-assistant message history uses input types.
			if typ == "output_text" {
				part["type"] = "input_text"
				stats.NonAssistantOutputTextFixed++
				content[j] = part
				contentChanged = true
			}
		}

		if contentChanged {
			itemMap["content"] = content
			(*input)[i] = itemMap
			changed = true
		}
	}

	return stats, changed
}
