package adaptor

import (
	"slices"
	"strings"
	"sync/atomic"
)

// ModelBillingPolicy declares which billing dimensions of a publisher catalog
// entry carry a verified tariff, so request admission, transport selection and
// receipt settlement consult descriptive capabilities instead of model names.
// The zero value imposes no restriction; publisher catalogs list only models
// whose published tariff leaves a billable dimension unpriced.
type ModelBillingPolicy struct {
	// SupportedServiceTiers lists the only provider service tiers the catalog
	// prices, pinned request tier first. Empty means the catalog does not
	// restrict the requested or provider-reported service tier.
	SupportedServiceTiers []string
	// HTTPOnly reports that only HTTP requests pass the tariff admission guards.
	// WebSocket frames bypass them, so WebSocket billing is not qualified.
	HTTPOnly bool
	// CacheWriteUnpricedFromInputTokens is the input-token threshold of the
	// catalog tier for which the publisher publishes no cache-write price. Zero
	// means every catalog tier has a known cache-write tariff. Tier resolution
	// otherwise inherits the base write rate for the omitted tier field.
	CacheWriteUnpricedFromInputTokens int
	// UnpricedCacheWriteInputMultiplier prices a cache write that still reaches
	// the unpriced tier (for example when admission's local token estimate fell
	// below the threshold) as an estimate no cheaper than this multiple of the
	// tier's uncached input rate. It must come from the publisher's documented
	// generic write rule; zero keeps the tier resolver's inherited write rate.
	// Such receipts remain unpriced estimates either way.
	UnpricedCacheWriteInputMultiplier float64
	// NativeTariffOnly reports that the policy describes only the publisher's
	// first-party tariff; a known hosted provider bills the model by its own
	// operator tariff, so these restrictions do not apply to its receipts.
	NativeTariffOnly bool
	// CaptureRawUsageReceipt reports that settlement must observe the raw
	// upstream usage receipt, because presentation converters drop billable
	// envelope dimensions such as the actual service tier.
	CaptureRawUsageReceipt bool
}

// RestrictsServiceTier reports whether the policy limits provider service
// tiers. It takes no parameters and returns true when SupportedServiceTiers
// is non-empty.
func (p ModelBillingPolicy) RestrictsServiceTier() bool {
	return len(p.SupportedServiceTiers) > 0
}

// SupportsServiceTier reports whether tier has a catalog tariff. Parameter
// tier is a requested or provider-reported service tier; matching ignores case
// and surrounding whitespace. It returns true for every tier when the policy is
// unrestricted, and false for an empty tier when it is restricted.
func (p ModelBillingPolicy) SupportsServiceTier(tier string) bool {
	if !p.RestrictsServiceTier() {
		return true
	}
	tier = strings.TrimSpace(tier)
	for _, supported := range p.SupportedServiceTiers {
		if strings.EqualFold(tier, supported) {
			return true
		}
	}
	return false
}

// PinnedServiceTier returns the service tier a restricted request must carry so
// the upstream cannot select an unpriced tier. It takes no parameters and
// returns the first supported tier, or an empty string when unrestricted.
func (p ModelBillingPolicy) PinnedServiceTier() string {
	if !p.RestrictsServiceTier() {
		return ""
	}
	return p.SupportedServiceTiers[0]
}

// PermitsWebSocket reports whether WebSocket transport billing is qualified.
// It takes no parameters and returns false for HTTP-only policies.
func (p ModelBillingPolicy) PermitsWebSocket() bool {
	return !p.HTTPOnly
}

// HasCacheWriteTariff reports whether cache writes have a published tariff for
// a request. Parameter promptTokens is the request's total input-token count.
// It returns false only when the request reaches the catalog tier whose
// cache-write price the publisher leaves unpublished.
func (p ModelBillingPolicy) HasCacheWriteTariff(promptTokens int) bool {
	return p.CacheWriteUnpricedFromInputTokens <= 0 || promptTokens < p.CacheWriteUnpricedFromInputTokens
}

// Clone returns a copy of the policy that shares no mutable state with p. It
// takes no parameters and returns the copy.
func (p ModelBillingPolicy) Clone() ModelBillingPolicy {
	p.SupportedServiceTiers = slices.Clone(p.SupportedServiceTiers)
	return p
}

// ModelBillingPolicyLookup resolves a publisher catalog policy. Parameter
// modelName is a requested or mapped model ID; implementations ignore case and
// surrounding whitespace. It returns the policy and whether one is declared.
type ModelBillingPolicyLookup func(modelName string) (ModelBillingPolicy, bool)

// modelBillingPolicyLookup holds the catalog lookup installed by the publisher
// package. relay/quota sits below the provider adaptors in the import graph
// (openai imports streaming, which imports quota), so it cannot import them.
var modelBillingPolicyLookup atomic.Pointer[ModelBillingPolicyLookup]

// SetModelBillingPolicyLookup installs the publisher catalog lookup consulted
// by LookupModelBillingPolicy. Parameter lookup replaces any earlier lookup; a
// nil lookup clears it. It returns nothing.
func SetModelBillingPolicyLookup(lookup ModelBillingPolicyLookup) {
	if lookup == nil {
		modelBillingPolicyLookup.Store(nil)
		return
	}
	modelBillingPolicyLookup.Store(&lookup)
}

// LookupModelBillingPolicy returns the publisher catalog policy for modelName
// independently of the settling provider, for layers that cannot import the
// publisher package. Parameter modelName is a requested or mapped model ID. It
// returns the policy and true when the installed catalog declares one.
func LookupModelBillingPolicy(modelName string) (ModelBillingPolicy, bool) {
	lookup := modelBillingPolicyLookup.Load()
	if lookup == nil {
		return ModelBillingPolicy{}, false
	}
	return (*lookup)(modelName)
}
