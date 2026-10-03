package controller

import (
	"slices"
	"strings"

	"github.com/Laisky/one-api/relay/adaptor"
	metalib "github.com/Laisky/one-api/relay/meta"
)

// reasoningModelConfig reads capabilities from the selected upstream catalog.
// Parameters: meta identifies the channel and modelName is the mapped name.
// Returns: the read-only configuration and whether it is known. Price-only
// operator overrides must not erase capabilities, and another provider's
// global pricing fallback must not impose its API contract on this channel.
func reasoningModelConfig(meta *metalib.Meta, modelName string) (adaptor.ModelConfig, bool) {
	provider := resolvePricingAdaptor(meta)
	if provider == nil {
		return adaptor.ModelConfig{}, false
	}
	cfg, known := provider.GetDefaultModelPricing()[modelName]
	return cfg, known
}

// queryReasoningEffort chooses a supported query effort without narrowing the
// request body's vocabulary. Parameters: meta/modelName select the upstream
// catalog and requested is an optional query value. Returns: an advertised
// effort, or the historical fallback when the catalog has no model entry.
func queryReasoningEffort(meta *metalib.Meta, modelName, requested string) string {
	cfg, known := reasoningModelConfig(meta, modelName)
	if known {
		allowed := cfg.SupportedReasoningEfforts
		if len(allowed) == 0 {
			return ""
		}
		requested = strings.ToLower(strings.TrimSpace(requested))
		if requested != "" && slices.Contains(allowed, requested) {
			return requested
		}
		// Retain established compatibility aliases only when their normalized
		// value is also declared by this provider, never before exact matching.
		if requested != "" {
			if normalized := normalizeReasoningEffort(modelName, requested); normalized != "" && slices.Contains(allowed, normalized) {
				return normalized
			}
		}
		// Keep the existing thinking=true preference where the model supports
		// it; otherwise use a valid catalog default rather than inventing high.
		if preferred := defaultReasoningEffort(modelName); slices.Contains(allowed, preferred) {
			return preferred
		}
		if cfg.DefaultReasoningEffort != "" && slices.Contains(allowed, cfg.DefaultReasoningEffort) {
			return cfg.DefaultReasoningEffort
		}
		return allowed[0]
	}
	if normalized := normalizeReasoningEffort(modelName, requested); normalized != "" {
		return normalized
	}
	return defaultReasoningEffort(modelName)
}
