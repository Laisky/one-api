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
// verified catalog tariff, as declared by the model's billing policy. The
// resolved cfg may supply an explicit operator long-context cache-write tier;
// scalar overrides alone do not qualify it.
func hasUnpricedOpenAIReceipt(input ComputeInput, cfg adaptor.ModelConfig) bool {
	policy, declared := adaptor.LookupModelBillingPolicy(input.ModelName)
	if !declared {
		return false
	}
	if policy.NativeTariffOnly && input.PricingAdaptor != nil {
		// Model names alone do not identify a hosted provider's tariff. Keep
		// the conservative fallback when provider identity is unavailable.
		provider, native := input.PricingAdaptor.(nativeOpenAIPricingProvider)
		if !native || !provider.UsesNativeOpenAIPricing() {
			return false
		}
	}
	if tier := strings.TrimSpace(input.Usage.ServiceTier); tier != "" && !policy.SupportsServiceTier(tier) {
		return true
	}
	if input.Usage.ServiceTierUnverified && policy.RestrictsServiceTier() {
		// A tier-restricted tariff prices only the tiers it lists; a receipt
		// whose actual tier is unknown is not proven to be one of them.
		return true
	}
	if input.Usage.CacheWrite5mTokens <= 0 {
		return false
	}
	// The publisher prices no cache write in the reached catalog tier. Do not
	// settle the base short-context write rate that the generic tier resolver
	// otherwise inherits for an omitted tier field. Admission applies the same
	// rule before dispatch (relay/controller native prompt-cache admission).
	return pricing.CacheWriteUnpriced(policy, cfg, input.Usage.PromptTokens, input.Usage.CompletionTokens)
}

// unpricedCacheWriteInputMultiplier returns the policy's generic write
// multiplier when the receipt's 30m cache writes fall in a catalog tier with no
// published write price, so settlement can price that estimate no lower than
// the publisher's documented generic rule. Parameters: input is the settlement
// input and cfg the resolved effective model config. It returns the multiplier
// and true only for such an unpriced write under a declared multiplier.
func unpricedCacheWriteInputMultiplier(input ComputeInput, cfg adaptor.ModelConfig) (float64, bool) {
	if input.Usage == nil || input.Usage.CacheWrite5mTokens <= 0 {
		return 0, false
	}
	policy, declared := adaptor.LookupModelBillingPolicy(input.ModelName)
	if !declared || policy.UnpricedCacheWriteInputMultiplier <= 0 {
		return 0, false
	}
	if policy.NativeTariffOnly && input.PricingAdaptor != nil {
		provider, native := input.PricingAdaptor.(nativeOpenAIPricingProvider)
		if !native || !provider.UsesNativeOpenAIPricing() {
			return 0, false
		}
	}
	if !pricing.CacheWriteUnpriced(policy, cfg, input.Usage.PromptTokens, input.Usage.CompletionTokens) {
		return 0, false
	}
	return policy.UnpricedCacheWriteInputMultiplier, true
}
