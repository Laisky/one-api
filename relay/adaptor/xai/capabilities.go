package xai

import "slices"

// supportsStringValue checks exact membership in a capability vocabulary.
// Parameters: values contains accepted strings and value is the requested one.
// Returns: true only when an accepted string equals the requested value.
func supportsStringValue(values []string, value string) bool {
	return slices.Contains(values, value)
}

// isLegacyPenaltyRestrictedModel identifies compatibility-only penalty rules.
// Parameters: model is the upstream model identifier. Returns: true for legacy
// redirects whose penalty restrictions must survive catalog metadata changes.
func isLegacyPenaltyRestrictedModel(model string) bool {
	switch model {
	case "grok-4-0709",
		"grok-4-1-fast-reasoning",
		"grok-4-1-fast-non-reasoning",
		"grok-4-fast-reasoning",
		"grok-4-fast-non-reasoning",
		"grok-code-fast-1":
		return true
	default:
		return false
	}
}
