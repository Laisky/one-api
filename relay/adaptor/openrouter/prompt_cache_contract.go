package openrouter

import "strings"

// ImplicitPromptCacheWrites reports whether modelName is a known OpenRouter
// GPT-5.6 or GPT-6 model, or a documented modern-family routing alias, with a
// published charge for automatic cache writes. It accepts the mapped host ID,
// ignoring case and surrounding whitespace, and returns false for unknown IDs,
// native provider IDs and other model families. Prices remain OpenRouter's own.
// Source: https://openrouter.ai/docs/guides/best-practices/prompt-caching
func ImplicitPromptCacheWrites(modelName string) bool {
	name := strings.ToLower(strings.TrimSpace(modelName))
	cfg, known := ModelRatios[name]
	if !known || cfg.CacheWrite5mRatio <= 0 {
		return false
	}
	if strings.HasPrefix(name, "openai/gpt-5.6-") ||
		strings.HasPrefix(name, "openai/gpt-6-") ||
		strings.HasPrefix(name, "openai/gpt-6.") {
		return true
	}
	// These aliases have individually published modern-family write quotes.
	// The retained legacy ~openai/gpt-latest target is not established by the
	// current catalog, so a historical write field alone cannot qualify it.
	switch name {
	case "~openai/gpt-astra-latest", "~openai/gpt-sol-latest",
		"~openai/gpt-terra-latest", "~openai/gpt-luna-latest":
		return true
	default:
		return false
	}
}
