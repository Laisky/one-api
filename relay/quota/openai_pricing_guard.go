package quota

import (
	"strings"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/pricing"
)

// nativeOpenAIPricingProvider identifies native OpenAI tariff semantics without
// importing a concrete adaptor into quota. Its method takes no parameters and
// returns whether native pricing restrictions apply to the current channel.
type nativeOpenAIPricingProvider interface {
	UsesNativeOpenAIPricing() bool
}

// hasUnpricedOpenAIReceipt reports whether input includes usage outside its
// verified OpenAI tariff. The resolved cfg may supply an explicit operator
// long-context cache-write tier; scalar overrides alone do not qualify it.
func hasUnpricedOpenAIReceipt(input ComputeInput, cfg adaptor.ModelConfig) bool {
	name := strings.ToLower(strings.TrimSpace(input.ModelName))
	if name != "gpt-6.1-terra" && name != "gpt-6-cyber" {
		return false
	}
	if name == "gpt-6-cyber" && input.PricingAdaptor != nil {
		// Model names alone do not identify a hosted provider's tariff. Keep
		// the conservative fallback when provider identity is unavailable.
		provider, native := input.PricingAdaptor.(nativeOpenAIPricingProvider)
		if !native || !provider.UsesNativeOpenAIPricing() {
			return false
		}
	}
	if tier := strings.TrimSpace(input.Usage.ServiceTier); tier != "" && !strings.EqualFold(tier, "default") {
		return true
	}
	if name != "gpt-6-cyber" || input.Usage.PromptTokens <= 272_000 || input.Usage.CacheWrite5mTokens <= 0 {
		return false
	}
	// OpenAI publishes Cyber long input/output/cache-read prices, but no long
	// cache-write price. Do not settle the base short-context write rate that
	// the generic tier resolver otherwise inherits for an omitted tier field.
	// https://developers.openai.com/api/docs/pricing.md
	for _, tier := range cfg.Tiers {
		if pricing.TierApplies(input.Usage.PromptTokens, input.Usage.CompletionTokens, tier) && tier.CacheWrite5mRatio != 0 {
			return false
		}
	}
	return true
}
