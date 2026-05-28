package anthropic

import (
	"strings"

	"github.com/Laisky/one-api/relay/model"
)

// claudeAdaptiveOpusPrefixes enumerates Claude Opus model families that share the
// adaptive-thinking compatibility profile: temperature/top_p/top_k must be omitted
// and any thinking block must be `{"type":"adaptive"}` without budget_tokens.
// Starting with Opus 4.7 Anthropic froze this contract; later Opus releases inherit it.
var claudeAdaptiveOpusPrefixes = []string{
	"claude-opus-4-7",
	"claude-opus-4-8",
}

// IsClaudeOpus47Model reports whether modelName targets a Claude Opus release that
// follows the Opus 4.7 adaptive-thinking compatibility profile (currently 4.7 and 4.8).
// It normalizes whitespace and casing.
func IsClaudeOpus47Model(modelName string) bool {
	normalized := strings.ToLower(strings.TrimSpace(modelName))
	for _, prefix := range claudeAdaptiveOpusPrefixes {
		if strings.HasPrefix(normalized, prefix) {
			return true
		}
	}
	return false
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
