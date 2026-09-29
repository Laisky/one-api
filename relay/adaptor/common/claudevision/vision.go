// Package claudevision defines network-free image reservation policies.
package claudevision

import "strings"

// Sonnet55MaxImageTokens is a reservation allowance for one high-resolution image,
// not its measured usage. Final billing must use the upstream token receipt.
// Source: https://platform.claude.com/docs/en/models/sonnet-5-5/migration-guide
const Sonnet55MaxImageTokens = 4784

// IsSonnet55 reports whether name is the documented native Sonnet 5.5 model ID.
// Unknown names and separately hosted model namespaces retain their own policies.
func IsSonnet55(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), "claude-sonnet-5-5")
}

// CountNativeImages counts image blocks, including images inside tool-result content.
// It does not inspect opaque tool inputs, schema properties, or image payload bytes.
func CountNativeImages(content any) int {
	switch value := content.(type) {
	case []any:
		count := 0
		for _, block := range value {
			count += CountNativeImages(block)
		}
		return count
	case map[string]any:
		kind, _ := value["type"].(string)
		switch kind {
		case "image":
			if source, ok := value["source"].(map[string]any); ok && len(source) > 0 {
				return 1
			}
		case "tool_result", "search_result":
			return CountNativeImages(value["content"])
		}
	}
	return 0
}
