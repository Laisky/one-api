package builtinname

import (
	"regexp"
	"strings"
)

// datedServerTool matches only documented eight-digit Anthropic tool versions.
// Unrecognized suffixes retain independent capability/pricing identities.
var datedServerTool = regexp.MustCompile(`^(web_search|web_fetch|code_execution)_\d{8}$`)

// Normalize maps known built-in tool spellings to one capability name.
// Unknown names return empty, leaving their handling to the caller.
func Normalize(toolType string) string {
	normalized := strings.ToLower(strings.TrimSpace(toolType))
	switch normalized {
	case "web_search", "web_search_preview", "web-search":
		return "web_search"
	case "tool_search_tool_regex", "tool_search_tool_bm25":
		return "tool_search"
	}
	if strings.HasPrefix(normalized, "tool_search_tool_regex_") || strings.HasPrefix(normalized, "tool_search_tool_bm25_") {
		return "tool_search"
	}
	if match := datedServerTool.FindStringSubmatch(normalized); len(match) == 2 {
		return match[1]
	}
	return ""
}

// Canonical shares billing's existing fallback for unknown tool names,
// while preventing known aliases from bypassing a receipt-contract check.
func Canonical(toolName string) string {
	trimmed := strings.TrimSpace(toolName)
	if trimmed == "" {
		return ""
	}
	if known := Normalize(trimmed); known != "" {
		return known
	}
	return strings.ToLower(trimmed)
}
