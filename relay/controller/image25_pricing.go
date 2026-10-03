package controller

import (
	"time"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
)

// resolveGPTImage25UsageConfig captures effective pricing at request start.
// Parameters: name selects the model, configs/ratios contain channel overrides,
// provider supplies defaults, and at anchors time windows. Returns: a private
// configuration with unspecified fields inherited and explicit overrides retained.
func resolveGPTImage25UsageConfig(name string, configs map[string]model.ModelConfigLocal, ratios map[string]float64, provider adaptor.Adaptor, at time.Time) adaptor.ModelConfig {
	if !isGPTImage25Model(name) {
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

// computeGPTImage25ConfiguredQuota honors channel prices across all token buckets.
// Parameters: usage is normalized provider usage, cfg is request-start pricing,
// and group scales quota. Returns: billable quota, or zero for unusable usage.
func computeGPTImage25ConfiguredQuota(usage *relaymodel.Usage, cfg adaptor.ModelConfig, group float64) float64 {
	if usage == nil {
		return 0
	}
	effective := pricing.ResolveEffectivePricingForUsageFromConfig(usage.PromptTokens, usage.CompletionTokens, cfg)
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
	prices := gptImageTokenBucketPricing{
		inputTextUSD:        effective.InputRatio / billingratio.MilliTokensUsd,
		cachedInputTextUSD:  cached / billingratio.MilliTokensUsd,
		inputImageUSD:       effective.InputRatio * imageRatio / billingratio.MilliTokensUsd,
		cachedInputImageUSD: cached * imageRatio / billingratio.MilliTokensUsd,
		outputImageUSD:      effective.OutputRatio / billingratio.MilliTokensUsd,
	}
	return computeGPTImage25TokenQuota(usage, prices, group)
}
