package anthropic

import (
	"strings"

	"github.com/Laisky/one-api/relay/model"
)

const claudeOpus47ModelPrefix = "claude-opus-4-7"

// IsClaudeOpus47Model reports whether modelName targets Claude Opus 4.7.
// It normalizes whitespace and casing and returns true for plain or versioned Claude Opus 4.7 identifiers.
func IsClaudeOpus47Model(modelName string) bool {
	normalized := strings.ToLower(strings.TrimSpace(modelName))
	return strings.HasPrefix(normalized, claudeOpus47ModelPrefix)
}

// NormalizeModelCompatibility normalizes Anthropic request parameters for model-specific compatibility.
// It mutates the provided parameter pointers in place and strips or rewrites fields that upstream rejects.
func NormalizeModelCompatibility(modelName string, temperature **float64, topP **float64, topK **int, thinking **model.Thinking) {
	if temperature != nil && *temperature != nil && topP != nil && *topP != nil {
		*topP = nil
	}

	if !IsClaudeOpus47Model(modelName) {
		return
	}

	if temperature != nil {
		*temperature = nil
	}
	if topP != nil {
		*topP = nil
	}
	if topK != nil {
		*topK = nil
	}
	if thinking == nil || *thinking == nil {
		return
	}

	(*thinking).Type = "adaptive"
	(*thinking).BudgetTokens = nil
}
