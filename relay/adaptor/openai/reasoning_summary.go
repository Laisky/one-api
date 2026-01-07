package openai

import (
	"strings"

	"github.com/songquanpeng/one-api/relay/model"
)

// ReasoningSummaryNormalization captures an in-place reasoning.summary adjustment.
type ReasoningSummaryNormalization struct {
	Changed bool
	Before  string
	After   string
}

// NormalizeResponseReasoningSummaryForModel normalizes the Response API reasoning.summary
// value for the given model.
//
// Parameters:
//   - modelName: the target upstream model name.
//   - reasoning: the Response API reasoning object to normalize in place.
//
// Returns:
//   - a result describing whether a change was applied.
//
// This keeps backward compatibility by only adjusting values that would otherwise
// trigger upstream validation errors.
func NormalizeResponseReasoningSummaryForModel(modelName string, reasoning *model.OpenAIResponseReasoning) ReasoningSummaryNormalization {
	var res ReasoningSummaryNormalization
	if reasoning == nil || reasoning.Summary == nil {
		return res
	}

	before := strings.TrimSpace(*reasoning.Summary)
	res.Before = before
	if before == "" {
		// Treat empty values as absent to avoid upstream validation errors.
		reasoning.Summary = nil
		res.Changed = true
		res.After = ""
		return res
	}

	normalized := strings.ToLower(before)
	// OpenAI currently rejects reasoning.summary=auto/concise for o4-* (e.g. o4-mini-2025-04-16).
	// Coerce to "detailed" when an unsupported value is provided.
	if isO4FamilyModel(modelName) && normalized != "detailed" {
		value := "detailed"
		reasoning.Summary = &value
		res.Changed = true
		res.After = value
		return res
	}

	if normalized != before {
		// Normalize casing/whitespace to keep payload consistent.
		value := normalized
		reasoning.Summary = &value
		res.Changed = true
		res.After = value
		return res
	}

	res.After = before
	return res
}

// DefaultResponseReasoningSummaryForModel returns the default reasoning.summary
// value used when converting Chat Completions requests to the Response API.
//
// Parameters:
//   - modelName: the target upstream model name.
//
// Returns:
//   - the default reasoning.summary string.
func DefaultResponseReasoningSummaryForModel(modelName string) string {
	// o4-* currently only accepts "detailed".
	if isO4FamilyModel(modelName) {
		return "detailed"
	}

	return "auto"
}

func isO4FamilyModel(modelName string) bool {
	normalized := strings.ToLower(strings.TrimSpace(modelName))
	return strings.HasPrefix(normalized, "o4")
}
