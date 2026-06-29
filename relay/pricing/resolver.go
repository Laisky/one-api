package pricing

import (
	"sort"
	"time"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
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
func ResolveModelConfig(modelName string, channelConfigs map[string]model.ModelConfigLocal, provider adaptor.Adaptor, at time.Time) (adaptor.ModelConfig, bool) {
	if channelConfigs != nil {
		if local, ok := channelConfigs[modelName]; ok {
			cfg := convertLocalModelConfig(local)
			return ApplyTimeWindow(cfg, at), true
		}
	}

	if provider != nil {
		if defaults := provider.GetDefaultModelPricing(); defaults != nil {
			if cfg, ok := defaults[modelName]; ok {
				return ApplyTimeWindow(cloneModelConfig(cfg), at), true
			}
		}
	}

	if cfg, ok := GetGlobalModelConfig(modelName); ok {
		return ApplyTimeWindow(cfg, at), true
	}

	return adaptor.ModelConfig{}, false
}

// ResolveAudioPricing resolves audio pricing metadata with three-layer precedence:
// channel overrides (when they include audio pricing), provider defaults, then global pricing.
// It returns nil when no audio metadata is defined in any layer.
func ResolveAudioPricing(modelName string, channelConfigs map[string]model.ModelConfigLocal, provider adaptor.Adaptor, at time.Time) (*adaptor.AudioPricingConfig, bool) {
	if channelConfigs != nil {
		if local, ok := channelConfigs[modelName]; ok {
			cfg := ApplyTimeWindow(convertLocalModelConfig(local), at)
			if cfg.Audio != nil && cfg.Audio.HasData() {
				return cfg.Audio.Clone(), true
			}
		}
	}

	if provider != nil {
		if defaults := provider.GetDefaultModelPricing(); defaults != nil {
			if cfg, ok := defaults[modelName]; ok {
				cfg = ApplyTimeWindow(cloneModelConfig(cfg), at)
				if cfg.Audio != nil && cfg.Audio.HasData() {
					return cfg.Audio.Clone(), true
				}
			}
		}
	}

	if cfg, ok := GetGlobalModelConfig(modelName); ok {
		cfg = ApplyTimeWindow(cfg, at)
		if cfg.Audio != nil && cfg.Audio.HasData() {
			return cfg.Audio.Clone(), true
		}
	}

	return nil, false
}

// ResolveImagePricing resolves image pricing metadata with three-layer precedence:
// channel overrides (when they include image pricing), provider defaults, then global pricing.
// It returns nil when no image metadata is defined in any layer.
func ResolveImagePricing(modelName string, channelConfigs map[string]model.ModelConfigLocal, provider adaptor.Adaptor, at time.Time) (*adaptor.ImagePricingConfig, bool) {
	if channelConfigs != nil {
		if local, ok := channelConfigs[modelName]; ok {
			cfg := ApplyTimeWindow(convertLocalModelConfig(local), at)
			if cfg.Image != nil && cfg.Image.HasData() {
				return cfg.Image.Clone(), true
			}
		}
	}

	if provider != nil {
		if defaults := provider.GetDefaultModelPricing(); defaults != nil {
			if cfg, ok := defaults[modelName]; ok {
				cfg = ApplyTimeWindow(cloneModelConfig(cfg), at)
				if cfg.Image != nil && cfg.Image.HasData() {
					return cfg.Image.Clone(), true
				}
			}
		}
	}

	if cfg, ok := GetGlobalModelConfig(modelName); ok {
		cfg = ApplyTimeWindow(cfg, at)
		if cfg.Image != nil && cfg.Image.HasData() {
			return cfg.Image.Clone(), true
		}
	}

	return nil, false
}

func convertLocalModelConfig(local model.ModelConfigLocal) adaptor.ModelConfig {
	cfg := adaptor.ModelConfig{
		Ratio:             local.Ratio,
		CompletionRatio:   local.CompletionRatio,
		CachedInputRatio:  local.CachedInputRatio,
		CacheWrite5mRatio: local.CacheWrite5mRatio,
		CacheWrite1hRatio: local.CacheWrite1hRatio,
		MaxTokens:         local.MaxTokens,
	}
	if len(local.Tiers) > 0 {
		cfg.Tiers = make([]adaptor.ModelRatioTier, 0, len(local.Tiers))
		for _, t := range local.Tiers {
			cfg.Tiers = append(cfg.Tiers, adaptor.ModelRatioTier{
				Ratio:               t.Ratio,
				CompletionRatio:     t.CompletionRatio,
				CachedInputRatio:    t.CachedInputRatio,
				CacheWrite5mRatio:   t.CacheWrite5mRatio,
				CacheWrite1hRatio:   t.CacheWrite1hRatio,
				InputTokenThreshold: t.InputTokenThreshold,
			})
		}
		sort.Slice(cfg.Tiers, func(i, j int) bool {
			return cfg.Tiers[i].InputTokenThreshold < cfg.Tiers[j].InputTokenThreshold
		})
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
	if local.Embedding != nil {
		cfg.Embedding = convertLocalEmbedding(local.Embedding)
	}
	if len(local.TimeWindows) > 0 {
		cfg.TimeWindows = convertLocalTimeWindows(local.TimeWindows)
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

// convertLocalEmbedding converts the persisted local embedding pricing metadata into adaptor form.
func convertLocalEmbedding(local *model.EmbeddingPricingLocal) *adaptor.EmbeddingPricingConfig {
	if local == nil {
		return nil
	}
	return &adaptor.EmbeddingPricingConfig{
		TextTokenRatio:     local.TextTokenRatio,
		ImageTokenRatio:    local.ImageTokenRatio,
		AudioTokenRatio:    local.AudioTokenRatio,
		VideoTokenRatio:    local.VideoTokenRatio,
		DocumentTokenRatio: local.DocumentTokenRatio,
		UsdPerImage:        local.UsdPerImage,
		UsdPerAudioSecond:  local.UsdPerAudioSecond,
		UsdPerVideoFrame:   local.UsdPerVideoFrame,
		UsdPerDocumentPage: local.UsdPerDocumentPage,
	}
}

// convertLocalTimeWindows converts channel-scoped time-window pricing overlays into adaptor form.
// Parameters: local is the persisted ordered window list.
// Returns: a deep-converted adaptor window list preserving order.
func convertLocalTimeWindows(local []model.TimeWindowLocal) []adaptor.TimeWindow {
	if len(local) == 0 {
		return nil
	}
	windows := make([]adaptor.TimeWindow, 0, len(local))
	for _, window := range local {
		ranges := make([]adaptor.ClockRange, 0, len(window.Ranges))
		for _, clockRange := range window.Ranges {
			ranges = append(ranges, adaptor.ClockRange{
				Start: clockRange.Start,
				End:   clockRange.End,
			})
		}
		windows = append(windows, adaptor.TimeWindow{
			Name:       window.Name,
			TimeZone:   window.TimeZone,
			Ranges:     ranges,
			DaysOfWeek: append([]int(nil), window.DaysOfWeek...),
			DateFrom:   window.DateFrom,
			DateTo:     window.DateTo,
			Overlay:    convertLocalModelConfig(window.Overlay),
		})
	}
	return windows
}

// ResolveModelConfigRatioOnly returns a shallow configuration by applying
// channel overrides first, then adaptor defaults, then global fallbacks.
// It omits media metadata to optimize for token-only billing paths.
func ResolveModelConfigRatioOnly(modelName string, channelConfigs map[string]model.ModelConfigLocal, provider adaptor.Adaptor, at time.Time) (adaptor.ModelConfig, bool) {
	if channelConfigs != nil {
		if local, ok := channelConfigs[modelName]; ok {
			cfg := convertLocalModelConfigRatioOnly(local)
			return ApplyTimeWindowRatioOnly(cfg, at), true
		}
	}

	if provider != nil {
		if defaults := provider.GetDefaultModelPricing(); defaults != nil {
			if cfg, ok := defaults[modelName]; ok {
				clone := cfg
				if len(cfg.Tiers) > 0 {
					clone.Tiers = append([]adaptor.ModelRatioTier(nil), cfg.Tiers...)
				}
				clone.Video = nil
				clone.Audio = nil
				clone.Image = nil
				if cfg.Embedding != nil {
					clone.Embedding = cfg.Embedding.Clone()
				}
				return ApplyTimeWindowRatioOnly(clone, at), true
			}
		}
	}

	if cfg, ok := GetGlobalModelConfigRatioOnly(modelName); ok {
		return ApplyTimeWindowRatioOnly(cfg, at), true
	}

	return adaptor.ModelConfig{}, false
}

func convertLocalModelConfigRatioOnly(local model.ModelConfigLocal) adaptor.ModelConfig {
	cfg := adaptor.ModelConfig{
		Ratio:             local.Ratio,
		CompletionRatio:   local.CompletionRatio,
		CachedInputRatio:  local.CachedInputRatio,
		CacheWrite5mRatio: local.CacheWrite5mRatio,
		CacheWrite1hRatio: local.CacheWrite1hRatio,
		MaxTokens:         local.MaxTokens,
	}
	if len(local.Tiers) > 0 {
		cfg.Tiers = make([]adaptor.ModelRatioTier, 0, len(local.Tiers))
		for _, t := range local.Tiers {
			cfg.Tiers = append(cfg.Tiers, adaptor.ModelRatioTier{
				Ratio:               t.Ratio,
				CompletionRatio:     t.CompletionRatio,
				CachedInputRatio:    t.CachedInputRatio,
				CacheWrite5mRatio:   t.CacheWrite5mRatio,
				CacheWrite1hRatio:   t.CacheWrite1hRatio,
				InputTokenThreshold: t.InputTokenThreshold,
			})
		}
		sort.Slice(cfg.Tiers, func(i, j int) bool {
			return cfg.Tiers[i].InputTokenThreshold < cfg.Tiers[j].InputTokenThreshold
		})
	}
	if len(local.TimeWindows) > 0 {
		cfg.TimeWindows = convertLocalTimeWindows(local.TimeWindows)
	}
	return cfg
}
