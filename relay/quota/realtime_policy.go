package quota

import (
	"math"
	"time"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/pricing"
)

// nativeRealtimePricingPolicy separates a provider's receipt protocol from its
// model catalog and published prices. Methods take a configured model ID and
// return policy flags only; they never query account entitlements.
type nativeRealtimePricingPolicy interface {
	UsesGeminiRealtimePricing() bool
	RequiresExplicitRealtimePricing(model string) bool
}

// requiresExplicitRealtimePricing reports whether provider defaults are absent
// or inappropriate for a model. Parameters: provider and name identify pricing.
// Returns: false for providers retaining the existing pricing contract.
func requiresExplicitRealtimePricing(provider adaptor.Adaptor, name string) bool {
	policy, ok := provider.(nativeRealtimePricingPolicy)
	return ok && policy.RequiresExplicitRealtimePricing(name)
}

// usesGeminiRealtimePricing selects the modality resolver by provider protocol.
// Parameters: provider is the selected pricing adaptor. Returns: whether native
// Gemini receipt pricing applies, including to unlisted operator-configured IDs.
func usesGeminiRealtimePricing(provider adaptor.Adaptor) bool {
	policy, ok := provider.(nativeRealtimePricingPolicy)
	return ok && policy.UsesGeminiRealtimePricing()
}

// ValidateRealtimeModelPricing validates operator-owned prices before provider
// work and again at settlement. Parameters: name, configs, provider, and at
// identify the model's effective pricing. Returns: an error for missing or
// incomplete explicit rates, never for a model's preview or permission status.
func ValidateRealtimeModelPricing(name string, configs map[string]model.ModelConfigLocal, provider adaptor.Adaptor, at time.Time) error {
	if !requiresExplicitRealtimePricing(provider, name) {
		return nil
	}
	local, exists := configs[name]
	if !exists {
		return errors.Wrap(ErrRealtimePriceUnavailable, "configure this Live model's channel pricing; no upstream price is inferred")
	}
	finite := func(n float64) bool { return n >= 0 && !math.IsNaN(n) && !math.IsInf(n, 0) }
	if !finite(local.Ratio) {
		return errors.Wrap(ErrRealtimePriceUnavailable, "invalid configured Live input price")
	}
	// Paid base pricing must be explicit, not filled from a different backend's
	// defaults. Zero input ratio is the existing deliberate free-model policy.
	if local.Ratio > 0 && (!finite(local.CompletionRatio) || local.CompletionRatio <= 0 ||
		local.Audio == nil || local.Audio.PromptRatio <= 0 || local.Audio.CompletionRatio <= 0 ||
		!finite(local.Audio.PromptRatio) || !finite(local.Audio.CompletionRatio) ||
		local.Image == nil || local.Image.PromptRatio <= 0 || !finite(local.Image.PromptRatio)) {
		return errors.Wrap(ErrRealtimePriceUnavailable, "configure Live completion_ratio, audio prompt/completion ratios, and image prompt_ratio (image/video input)")
	}
	cfg, known := pricing.ResolveModelConfigRatioOnly(name, configs, provider, at)
	if !known || !finite(cfg.Ratio) {
		return errors.Wrap(ErrRealtimePriceUnavailable, "invalid effective Live pricing")
	}
	if cfg.Ratio > 0 && (cfg.CompletionRatio <= 0 || !finite(cfg.CompletionRatio) ||
		cfg.Audio == nil || cfg.Audio.PromptRatio <= 0 || cfg.Audio.CompletionRatio <= 0 ||
		!finite(cfg.Audio.PromptRatio) || !finite(cfg.Audio.CompletionRatio) || cfg.Audio.UsdPerSecond != 0 ||
		cfg.Image == nil || cfg.Image.PromptRatio <= 0 || !finite(cfg.Image.PromptRatio)) {
		return errors.Wrap(ErrRealtimePriceUnavailable, "incomplete effective Live token prices")
	}
	return nil
}
