package openai

import (
	"strconv"
	"strings"
)

// ResponseAPIInputContentNormalizationStats reports how many legacy content-type
// values were rewritten and how many embedded data URLs were redacted when
// normalizing Response API input items.
type ResponseAPIInputContentNormalizationStats struct {
	AssistantInputTextFixed     int
	NonAssistantOutputTextFixed int
	ReasoningSummaryFixed       int
	DataURLRedacted             int
	DataURLRedactedBytes        int
}

// responseAPIEmbeddedImageDataURLRedactionThreshold defines the minimum base64 payload length
// inside embedded image data URLs that triggers redaction in assistant message content.
const responseAPIEmbeddedImageDataURLRedactionThreshold = 1024 * 1024

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

// NormalizeResponseAPIInputEmbeddedImageDataURLs redacts oversized embedded image data URLs
// inside assistant text content items to prevent oversized payloads reaching upstream models.
// Parameters: input is the Response API input slice to normalize in place.
// Returns: normalization stats and a boolean indicating whether any changes were made.
func NormalizeResponseAPIInputEmbeddedImageDataURLs(input *ResponseAPIInput) (ResponseAPIInputContentNormalizationStats, bool) {
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
		if role != "assistant" {
			continue
		}

		content, ok := itemMap["content"]
		if !ok {
			continue
		}

		switch typed := content.(type) {
		case string:
			sanitized, redactedCount, redactedBytes := redactEmbeddedImageDataURLs(typed, responseAPIEmbeddedImageDataURLRedactionThreshold)
			if redactedCount > 0 {
				itemMap["content"] = sanitized
				(*input)[i] = itemMap
				stats.DataURLRedacted += redactedCount
				stats.DataURLRedactedBytes += redactedBytes
				changed = true
			}
		case []any:
			contentChanged := false
			for j := range typed {
				part, ok := typed[j].(map[string]any)
				if !ok || part == nil {
					continue
				}
				typeStr, _ := part["type"].(string)
				typeStr = strings.ToLower(strings.TrimSpace(typeStr))
				if typeStr != "input_text" && typeStr != "output_text" && typeStr != "text" && typeStr != "refusal" && typeStr != "reasoning" {
					continue
				}
				text, ok := part["text"].(string)
				if !ok || text == "" {
					continue
				}
				sanitized, redactedCount, redactedBytes := redactEmbeddedImageDataURLs(text, responseAPIEmbeddedImageDataURLRedactionThreshold)
				if redactedCount == 0 {
					continue
				}
				part["text"] = sanitized
				typed[j] = part
				stats.DataURLRedacted += redactedCount
				stats.DataURLRedactedBytes += redactedBytes
				contentChanged = true
			}
			if contentChanged {
				itemMap["content"] = typed
				(*input)[i] = itemMap
				changed = true
			}
		}
	}

	return stats, changed
}

// redactEmbeddedImageDataURLs replaces large embedded data:image/*;base64 payloads in text.
// Parameters: text is the input string; minBase64Len is the threshold for redaction.
// Returns: sanitized text, number of redactions, and total redacted base64 byte length.
func redactEmbeddedImageDataURLs(text string, minBase64Len int) (string, int, int) {
	if text == "" {
		return text, 0, 0
	}
	lower := strings.ToLower(text)
	if !strings.Contains(lower, "data:image/") {
		return text, 0, 0
	}

	var out strings.Builder
	out.Grow(len(text))

	cursor := 0
	redactedCount := 0
	redactedBytes := 0

	for {
		idx := strings.Index(lower[cursor:], "data:image/")
		if idx < 0 {
			break
		}
		idx += cursor
		base64Idx := strings.Index(lower[idx:], "base64,")
		if base64Idx < 0 {
			cursor = idx + len("data:image/")
			continue
		}
		base64Idx = idx + base64Idx + len("base64,")

		end := len(text)
		for i := base64Idx; i < len(text); i++ {
			if isEmbeddedDataURLTerminator(text[i]) {
				end = i
				break
			}
		}
		dataLen := end - base64Idx

		out.WriteString(text[cursor:base64Idx])
		if dataLen >= minBase64Len {
			out.WriteString("[truncated base64 len=")
			out.WriteString(strconv.Itoa(dataLen))
			out.WriteString("]")
			redactedCount++
			redactedBytes += dataLen
		} else {
			out.WriteString(text[base64Idx:end])
		}
		cursor = end
	}

	if redactedCount == 0 {
		return text, 0, 0
	}

	out.WriteString(text[cursor:])
	return out.String(), redactedCount, redactedBytes
}

// isEmbeddedDataURLTerminator reports whether a byte should terminate a data URL scan.
// Parameters: b is the byte to inspect.
// Returns: true when the byte indicates the end of a data URL payload in text.
func isEmbeddedDataURLTerminator(b byte) bool {
	switch b {
	case ' ', '\n', '\r', '\t', ')', '"', '\'', '>', ']', '}':
		return true
	default:
		return false
	}
}
