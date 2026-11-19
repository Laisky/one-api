package pricing

import "github.com/Laisky/one-api/relay/adaptor"

// CacheWriteUnpriced reports whether a cache write at the given token counts
// falls in a catalog tier whose cache-write price the publisher leaves
// unpublished, with no applicable explicit tier in cfg supplying the rate.
// Admission (before dispatch) and settlement (after the receipt) share this
// rule so they always agree on which writes are qualified. Parameters: policy
// is the publisher billing policy, cfg the resolved effective model config
// (including operator tiers), and promptTokens/completionTokens select the
// tier; admission passes zero completion tokens. It returns true only when the
// write has no qualified tariff.
func CacheWriteUnpriced(policy adaptor.ModelBillingPolicy, cfg adaptor.ModelConfig, promptTokens, completionTokens int) bool {
	if policy.HasCacheWriteTariff(promptTokens) {
		return false
	}
	// The generic tier resolver inherits the base short-context write rate for
	// an omitted tier field; only an explicit write rate qualifies the tier.
	for _, tier := range cfg.Tiers {
		if TierApplies(promptTokens, completionTokens, tier) && tier.CacheWrite5mRatio != 0 {
			return false
		}
	}
	return true
}
