package pricing

import (
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/adaptor"
)

const (
	// DefaultAudioPromptRatio is used when no audio configuration is published for a model.
	DefaultAudioPromptRatio = 16.0
	// DefaultAudioCompletionRatio is used when audio completion pricing metadata is absent.
	DefaultAudioCompletionRatio = 2.0
	// DefaultAudioPromptTokensPerSecond is used when models omit explicit duration pricing metadata.
	DefaultAudioPromptTokensPerSecond = 10.0
)

// ResolveModelConfig returns the effective model configuration by applying
// channel overrides first, then adaptor defaults, then global fallbacks.
// The returned configuration is a clone that callers can mutate safely.
func ResolveModelConfig(modelName string, channelConfigs map[string]model.ModelConfigLocal, provider adaptor.Adaptor) (adaptor.ModelConfig, bool) {
	if channelConfigs != nil {
		if local, ok := channelConfigs[modelName]; ok {
			cfg := convertLocalModelConfig(local)
			return cfg, true
		}
	}

	if provider != nil {
		if defaults := provider.GetDefaultModelPricing(); defaults != nil {
			if cfg, ok := defaults[modelName]; ok {
				return cloneModelConfig(cfg), true
			}
		}
	}

	if cfg, ok := GetGlobalModelConfig(modelName); ok {
		return cfg, true
	}

	return adaptor.ModelConfig{}, false
}

// ResolveAudioPricing resolves audio pricing metadata with three-layer precedence:
// channel overrides (when they include audio pricing), provider defaults, then global pricing.
// It returns nil when no audio metadata is defined in any layer.
func ResolveAudioPricing(modelName string, channelConfigs map[string]model.ModelConfigLocal, provider adaptor.Adaptor) (*adaptor.AudioPricingConfig, bool) {
	if channelConfigs != nil {
		if local, ok := channelConfigs[modelName]; ok {
			cfg := convertLocalModelConfig(local)
			if cfg.Audio != nil && cfg.Audio.HasData() {
				return cfg.Audio.Clone(), true
			}
		}
	}

	if provider != nil {
		if defaults := provider.GetDefaultModelPricing(); defaults != nil {
			if cfg, ok := defaults[modelName]; ok {
				if cfg.Audio != nil && cfg.Audio.HasData() {
					return cfg.Audio.Clone(), true
				}
			}
		}
	}

	if cfg := GetGlobalAudioPricing(modelName); cfg != nil {
		if cfg.HasData() {
			return cfg, true
		}
	}

	return nil, false
}

// ResolveImagePricing resolves image pricing metadata with three-layer precedence:
// channel overrides (when they include image pricing), provider defaults, then global pricing.
// It returns nil when no image metadata is defined in any layer.
func ResolveImagePricing(modelName string, channelConfigs map[string]model.ModelConfigLocal, provider adaptor.Adaptor) (*adaptor.ImagePricingConfig, bool) {
	if channelConfigs != nil {
		if local, ok := channelConfigs[modelName]; ok {
			cfg := convertLocalModelConfig(local)
			if cfg.Image != nil && cfg.Image.HasData() {
				return cfg.Image.Clone(), true
			}
		}
	}

	if provider != nil {
		if defaults := provider.GetDefaultModelPricing(); defaults != nil {
			if cfg, ok := defaults[modelName]; ok {
				if cfg.Image != nil && cfg.Image.HasData() {
					return cfg.Image.Clone(), true
				}
			}
		}
	}

	if cfg := GetGlobalImagePricing(modelName); cfg != nil {
		if cfg.HasData() {
			return cfg, true
		}
	}

	return nil, false
}

func convertLocalModelConfig(local model.ModelConfigLocal) adaptor.ModelConfig {
	cfg := adaptor.ModelConfig{
		Ratio:           local.Ratio,
		CompletionRatio: local.CompletionRatio,
		MaxTokens:       local.MaxTokens,
	}
	if local.Video != nil {
		cfg.Video = convertLocalVideo(local.Video)
	}
	if local.Audio != nil {
		cfg.Audio = convertLocalAudio(local.Audio)
	}
	if local.Image != nil {
		cfg.Image = convertLocalImage(local.Image)
	}
	return cfg
}

func convertLocalVideo(local *model.VideoPricingLocal) *adaptor.VideoPricingConfig {
	if local == nil {
		return nil
	}
	cfg := &adaptor.VideoPricingConfig{
		PerSecondUsd:   local.PerSecondUsd,
		BaseResolution: local.BaseResolution,
	}
	if len(local.ResolutionMultipliers) > 0 {
		cfg.ResolutionMultipliers = make(map[string]float64, len(local.ResolutionMultipliers))
		for k, v := range local.ResolutionMultipliers {
			cfg.ResolutionMultipliers[k] = v
		}
	}
	return cfg
}

func convertLocalAudio(local *model.AudioPricingLocal) *adaptor.AudioPricingConfig {
	if local == nil {
		return nil
	}
	return &adaptor.AudioPricingConfig{
		PromptRatio:               local.PromptRatio,
		CompletionRatio:           local.CompletionRatio,
		PromptTokensPerSecond:     local.PromptTokensPerSecond,
		CompletionTokensPerSecond: local.CompletionTokensPerSecond,
		UsdPerSecond:              local.UsdPerSecond,
	}
}

func convertLocalImage(local *model.ImagePricingLocal) *adaptor.ImagePricingConfig {
	if local == nil {
		return nil
	}
	cfg := &adaptor.ImagePricingConfig{
		PricePerImageUsd: local.PricePerImageUsd,
		PromptRatio:      local.PromptRatio,
		DefaultSize:      local.DefaultSize,
		DefaultQuality:   local.DefaultQuality,
		PromptTokenLimit: local.PromptTokenLimit,
		MinImages:        local.MinImages,
		MaxImages:        local.MaxImages,
	}
	if len(local.SizeMultipliers) > 0 {
		cfg.SizeMultipliers = make(map[string]float64, len(local.SizeMultipliers))
		for k, v := range local.SizeMultipliers {
			cfg.SizeMultipliers[k] = v
		}
	}
	if len(local.QualityMultipliers) > 0 {
		cfg.QualityMultipliers = make(map[string]float64, len(local.QualityMultipliers))
		for k, v := range local.QualityMultipliers {
			cfg.QualityMultipliers[k] = v
		}
	}
	if len(local.QualitySizeMultipliers) > 0 {
		cfg.QualitySizeMultipliers = make(map[string]map[string]float64, len(local.QualitySizeMultipliers))
		for quality, sizes := range local.QualitySizeMultipliers {
			inner := make(map[string]float64, len(sizes))
			for size, value := range sizes {
				inner[size] = value
			}
			cfg.QualitySizeMultipliers[quality] = inner
		}
	}
	return cfg
}
