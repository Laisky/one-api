package pricing

import (
	"time"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
)

// ResolveImagePricing resolves an image tariff and its request defaults.
// Parameters: modelName identifies the model, channelConfigs holds operator
// overrides, provider supplies defaults, and at is the request start time.
// Returns: a private configuration and whether any image data exists. Tariffs
// retain channel/provider/global precedence; request-only overrides do not erase
// a tariff, and price-only overrides do not erase the provider's request defaults.
func ResolveImagePricing(modelName string, channelConfigs map[string]model.ModelConfigLocal, provider adaptor.Adaptor, at time.Time) (*adaptor.ImagePricingConfig, bool) {
	var lower *adaptor.ImagePricingConfig
	if provider != nil {
		if cfg, ok := provider.GetDefaultModelPricing()[modelName]; ok {
			cfg = ApplyTimeWindow(cloneModelConfig(cfg), at)
			if hasImageConfiguration(cfg.Image) {
				lower = cfg.Image.Clone()
			}
		}
	}
	if lower == nil {
		if cfg, ok := GetGlobalModelConfig(modelName); ok {
			cfg = ApplyTimeWindow(cfg, at)
			if hasImageConfiguration(cfg.Image) {
				lower = cfg.Image.Clone()
			}
		}
	}

	localModel, exists := channelConfigs[modelName]
	if !exists {
		return lower, lower != nil
	}
	local := ApplyTimeWindow(convertLocalModelConfig(localModel), at).Image
	if !hasImageConfiguration(local) {
		return lower, lower != nil
	}
	if hasImageTariff(local) {
		// An explicit tariff is a complete billing override, not an additive
		// surcharge. Inherit request defaults only, never price multipliers.
		result := local.Clone()
		fillImageRequestDefaults(result, lower)
		return result, true
	}
	if lower == nil {
		return local.Clone(), true
	}
	// These fields do not change the operator's billing policy. Overlay them
	// on the selected tariff, including its already-applied dated transition.
	if local.DefaultSize != "" {
		lower.DefaultSize = local.DefaultSize
	}
	if local.DefaultQuality != "" {
		lower.DefaultQuality = local.DefaultQuality
	}
	if local.PromptTokenLimit != 0 {
		lower.PromptTokenLimit = local.PromptTokenLimit
	}
	if local.MinImages != 0 {
		lower.MinImages = local.MinImages
	}
	if local.MaxImages != 0 {
		lower.MaxImages = local.MaxImages
	}
	return lower, true
}

// hasImageTariff distinguishes billing overrides from request defaults.
// Parameters: cfg is optional image metadata. Returns: true for explicit
// prices or multiplier tables; this function does not validate their values.
func hasImageTariff(cfg *adaptor.ImagePricingConfig) bool {
	return cfg != nil && (cfg.PricePerImageUsd != 0 || cfg.PromptRatio != 0 ||
		len(cfg.SizeMultipliers) > 0 || len(cfg.QualityMultipliers) > 0 || len(cfg.QualitySizeMultipliers) > 0)
}

// hasImageConfiguration detects tariff or request metadata, including defaults
// omitted by the legacy HasData predicate. Parameters: cfg may be nil.
// Returns: whether cfg has any effective image setting.
func hasImageConfiguration(cfg *adaptor.ImagePricingConfig) bool {
	return cfg != nil && (hasImageTariff(cfg) || cfg.DefaultSize != "" || cfg.DefaultQuality != "" ||
		cfg.PromptTokenLimit != 0 || cfg.MinImages != 0 || cfg.MaxImages != 0)
}

// fillImageRequestDefaults fills only missing non-price fields in a private copy.
// Parameters: dst is mutated and src may be nil. Returns: none; prices and
// multiplier maps are never inherited over an explicit operator tariff.
func fillImageRequestDefaults(dst, src *adaptor.ImagePricingConfig) {
	if src == nil {
		return
	}
	if dst.DefaultSize == "" {
		dst.DefaultSize = src.DefaultSize
	}
	if dst.DefaultQuality == "" {
		dst.DefaultQuality = src.DefaultQuality
	}
	if dst.PromptTokenLimit == 0 {
		dst.PromptTokenLimit = src.PromptTokenLimit
	}
	if dst.MinImages == 0 {
		dst.MinImages = src.MinImages
	}
	if dst.MaxImages == 0 {
		dst.MaxImages = src.MaxImages
	}
}
