package deepseekcompat

import "strings"

// NormalizeThinkingType maps provider-incompatible thinking.type values to DeepSeek-safe enums.
//
// Parameters:
//   - rawType: the incoming thinking.type value from client payload.
//   - budgetTokens: the incoming thinking.budget_tokens value.
//
// Returns:
//   - normalized: normalized DeepSeek-compatible value, always either enabled or disabled.
//   - changed: true when normalization changed the original effective value.
func NormalizeThinkingType(rawType string, budgetTokens *int) (normalized string, changed bool) {
	typeValue := strings.ToLower(strings.TrimSpace(rawType))
	hasBudget := budgetTokens != nil && *budgetTokens > 0
	switch typeValue {
	case "enabled", "disabled":
		if rawType == typeValue {
			return typeValue, false
		}
		return typeValue, true
	case "adaptive":
		return "enabled", true
	case "":
		if hasBudget {
			return "enabled", true
		}
		return "disabled", true
	default:
		if hasBudget {
			return "enabled", true
		}
		return "disabled", true
	}
}
