package openai

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/channeltype"
)

// standardServiceTierOnly is OpenAI's Standard ("default") service tier, the
// only tier priced for policy models. Request guards pin its first entry.
var standardServiceTierOnly = []string{"default"}

// modelBillingPolicies lists native OpenAI catalog entries whose published
// tariff leaves a billable dimension unpriced. Keys are lower-case model IDs
// present in ModelRatios; models absent here carry no billing restriction.
//
//   - gpt-6.1-terra: the catalog prices only Standard; WebSocket frames bypass
//     the HTTP Standard-tier and cache-lifetime admission guards.
//   - gpt-6-cyber: as Terra, plus OpenAI publishes no long-context cache-write
//     price, the model is published only for OpenAI's first-party API, and the
//     raw receipt carries the actual service tier that converters discard.
//
// Sources verified 2026-11-16:
//   - https://developers.openai.com/api/docs/models/gpt-6.1-terra
//   - https://developers.openai.com/api/docs/models/gpt-6-cyber
//   - https://developers.openai.com/api/docs/pricing.md
var modelBillingPolicies = map[string]adaptor.ModelBillingPolicy{
	"gpt-6.1-terra": {
		SupportedServiceTiers: standardServiceTierOnly,
		HTTPOnly:              true,
	},
	"gpt-6-cyber": {
		SupportedServiceTiers:             standardServiceTierOnly,
		HTTPOnly:                          true,
		CacheWriteUnpricedFromInputTokens: gpt6CyberLongContextTier.InputTokenThreshold,
		// "For GPT-5.6 and later, cache writes cost 1.25x the standard,
		// uncached input-token rate for the default 30-minute lifetime."
		// https://developers.openai.com/api/docs/guides/prompt-caching.md
		UnpricedCacheWriteInputMultiplier: 1.25,
		NativeTariffOnly:                  true,
		CaptureRawUsageReceipt:            true,
	},
}

// init publishes the OpenAI catalog policies to layers that cannot import this
// package, such as receipt settlement in relay/quota. It takes no parameters
// and returns nothing.
func init() {
	adaptor.SetModelBillingPolicyLookup(CatalogModelBillingPolicy)
}

// CatalogModelBillingPolicy returns the OpenAI catalog billing policy for
// modelName regardless of the channel serving it. Parameter modelName is a
// requested or mapped model ID; matching ignores case and surrounding
// whitespace. It returns a private copy of the policy and whether one exists.
func CatalogModelBillingPolicy(modelName string) (adaptor.ModelBillingPolicy, bool) {
	policy, ok := modelBillingPolicies[normalizedModelName(modelName)]
	if !ok {
		return adaptor.ModelBillingPolicy{}, false
	}
	return policy.Clone(), true
}

// NativeModelBillingPolicy returns the catalog policy only when channelType is
// OpenAI's first-party channel, whose upstream bills by this catalog. Parameters
// are the relay channel type and the mapped model ID. It returns the policy and
// true for a native policy model; compatible and hosted channels keep their
// operator-controlled request semantics and return false.
func NativeModelBillingPolicy(channelType int, modelName string) (adaptor.ModelBillingPolicy, bool) {
	if channelType != channeltype.OpenAI {
		return adaptor.ModelBillingPolicy{}, false
	}
	return CatalogModelBillingPolicy(modelName)
}

// CapturesRawUsageReceipt reports whether a native response must be observed
// before presentation conversion so settlement keeps billable receipt fields.
// Parameters are the relay channel type and the mapped model ID. It returns
// true only for native OpenAI channels serving a capture-policy model.
func CapturesRawUsageReceipt(channelType int, modelName string) bool {
	policy, ok := NativeModelBillingPolicy(channelType, modelName)
	return ok && policy.CaptureRawUsageReceipt
}
