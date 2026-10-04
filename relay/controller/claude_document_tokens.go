package controller

import (
	"maps"
	"strings"

	"github.com/Laisky/one-api/common/config"
)

// claudeOpaqueDocumentSource recognizes document bytes/handles that a native
// provider interprets as a document rather than a literal base64 prompt string.
func claudeOpaqueDocumentSource(block map[string]any) bool {
	kind, _ := block["type"].(string)
	if kind != "document" {
		return false
	}
	source, ok := block["source"].(map[string]any)
	if !ok {
		return false
	}
	mediaType, _ := source["media_type"].(string)
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(mediaType)), "text/") {
		return false
	}
	sourceType, _ := source["type"].(string)
	switch sourceType {
	case "base64", "file", "url":
		return true
	default:
		return false
	}
}

// claudeDocumentTextMetadata excludes only transport bytes/handles from a private
// quote view. Titles, context, citations and other text metadata remain counted;
// native dispatch retains the untouched original source.
func claudeDocumentTextMetadata(block map[string]any) map[string]any {
	quote := maps.Clone(block)
	source, _ := block["source"].(map[string]any)
	sourceQuote := maps.Clone(source)
	for _, key := range []string{"data", "url", "file_id", "file"} {
		delete(sourceQuote, key)
	}
	quote["source"] = sourceQuote
	return quote
}

// countClaudeNativeDocumentAllowance visits nested tool results iteratively and
// adds one trusted allowance per opaque document. It performs no remote fetch,
// PDF decompression or caller-controlled page-count interpretation. Converted
// text requests use preparedClaudeChatTokens instead and never enter this path.
func countClaudeNativeDocumentAllowance(request *ClaudeMessagesRequest) int {
	if request == nil {
		return 0
	}
	var stack [][]any
	for _, message := range request.Messages {
		if blocks, ok := message.Content.([]any); ok && len(blocks) > 0 {
			stack = append(stack, blocks)
		}
	}
	if blocks, ok := request.System.([]any); ok && len(blocks) > 0 {
		stack = append(stack, blocks)
	}
	total := 0
	for len(stack) > 0 {
		i := len(stack) - 1
		if len(stack[i]) == 0 {
			stack = stack[:i]
			continue
		}
		value := stack[i][0]
		stack[i] = stack[i][1:]
		block, ok := value.(map[string]any)
		if !ok {
			continue
		}
		if claudeOpaqueDocumentSource(block) {
			total += config.ClaudeNativeDocumentTokenAllowance
		}
		if block["type"] == "tool_result" {
			if nested, ok := block["content"].([]any); ok && len(nested) > 0 {
				stack = append(stack, nested)
			}
		}
	}
	return total
}
