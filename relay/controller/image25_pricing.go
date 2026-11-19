package controller

import (
	"time"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
)

// resolveStrictGPTImageUsageConfig captures effective pricing at request start.
// Parameters: name selects the model, configs/ratios contain channel overrides,
// provider supplies defaults, and at anchors time windows. Returns: a private
// configuration with unspecified fields inherited and explicit overrides retained.
func resolveStrictGPTImageUsageConfig(name string, configs map[string]model.ModelConfigLocal, ratios map[string]float64, provider adaptor.Adaptor, at time.Time) adaptor.ModelConfig {
	if !usesStrictGPTImageTokenBilling(name) {
		return adaptor.ModelConfig{}
	}
	base, _ := pricing.ResolveModelConfig(name, nil, provider, at)
	resolved, _ := pricing.ResolveModelConfig(name, configs, provider, at)
	base.Ratio = pricing.ResolveModelRatioAt(name, configs, ratios, provider, at)
	base.CompletionRatio = pricing.ResolveCompletionRatioAt(name, configs, nil, provider, at)
	if resolved.CachedInputRatio != 0 {
		base.CachedInputRatio = resolved.CachedInputRatio
	}
	if len(resolved.Tiers) > 0 {
		base.Tiers = resolved.Tiers
	}
	if resolved.Image != nil && resolved.Image.PromptRatio != 0 {
		if base.Image == nil {
			base.Image = &adaptor.ImagePricingConfig{}
		}
		base.Image.PromptRatio = resolved.Image.PromptRatio
	}
	return base
}

// computeStrictGPTImageConfiguredQuote honors channel prices across all token buckets.
// Parameters: usage is normalized provider usage, cfg is request-start pricing,
// and group scales quota. Returns: the strict quote (zero quota for unusable
// usage; upperBound when the receipt omitted part of its modality/cache split).
func computeStrictGPTImageConfiguredQuote(usage *relaymodel.Usage, cfg adaptor.ModelConfig, group float64) strictGPTImageQuote {
	if usage == nil {
		return strictGPTImageQuote{}
	}
	return computeStrictGPTImageTokenQuote(usage, strictGPTImageConfiguredPrices(usage, cfg), group)
}

// strictGPTImageConfiguredPrices converts request-start channel pricing into
// the five strict bucket prices for the tier the receipt reaches. Parameters:
// usage selects the tier and cfg is the resolved configuration. Returns: USD
// per million tokens for each bucket.
func strictGPTImageConfiguredPrices(usage *relaymodel.Usage, cfg adaptor.ModelConfig) gptImageTokenBucketPricing {
	completion := 0
	if usage != nil {
		completion = usage.CompletionTokens
	}
	effective := pricing.ResolveEffectivePricingForUsageFromConfig(strictGPTImageInputTokens(usage), completion, cfg)
	imageRatio := 8.0 / 5.0
	if cfg.Image != nil && cfg.Image.PromptRatio != 0 {
		imageRatio = cfg.Image.PromptRatio
	}
	cached := effective.CachedInputRatio
	if cached == 0 {
		cached = effective.InputRatio
	} else if cached < 0 {
		cached = 0
	}
	return gptImageTokenBucketPricing{
		inputTextUSD:        effective.InputRatio / billingratio.MilliTokensUsd,
		cachedInputTextUSD:  cached / billingratio.MilliTokensUsd,
		inputImageUSD:       effective.InputRatio * imageRatio / billingratio.MilliTokensUsd,
		cachedInputImageUSD: cached * imageRatio / billingratio.MilliTokensUsd,
		outputImageUSD:      effective.OutputRatio / billingratio.MilliTokensUsd,
	}
}

// strictGPTImagePrices returns the bucket prices settlement applies to a
// strict GPT Image receipt: request-start channel pricing when supplied, else
// the model's published catalog prices. Parameters: name is the mapped model,
// usage selects the tier, and configs optionally carries channel pricing.
// Returns: the prices and whether any are known for the model.
func strictGPTImagePrices(name string, usage *relaymodel.Usage, configs []adaptor.ModelConfig) (gptImageTokenBucketPricing, bool) {
	if len(configs) > 0 {
		return strictGPTImageConfiguredPrices(usage, configs[0]), true
	}
	prices, ok := gptImageTokenBucketPrices[name]
	return prices, ok
}
