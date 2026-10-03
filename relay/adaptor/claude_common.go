package adaptor

import "strings"

// ExtractClaudeContentText flattens Claude content payloads into a single text
// string. The content parameter may be a string, a block array, or a nested
// block map. The return value contains non-empty text fragments joined by
// newlines.
func ExtractClaudeContentText(content any) string {
	var parts []string
	collectClaudeContentTextParts(content, &parts)
	return strings.Join(parts, "\n")
}

// collectClaudeContentTextParts appends text fragments from Claude content
// payloads into parts. It accepts nested strings, arrays, and maps and returns
// through the parts slice pointer.
func collectClaudeContentTextParts(content any, parts *[]string) {
	switch val := content.(type) {
	case string:
		if strings.TrimSpace(val) != "" {
			*parts = append(*parts, val)
		}
	case []any:
		for _, entry := range val {
			collectClaudeContentTextParts(entry, parts)
		}
	case map[string]any:
		if text, ok := val["text"].(string); ok && strings.TrimSpace(text) != "" {
			*parts = append(*parts, text)
		}
		if nested, ok := val["content"]; ok {
			collectClaudeContentTextParts(nested, parts)
		}
	}
}
