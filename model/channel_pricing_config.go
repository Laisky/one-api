package model

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
)

// ModelConfigLocal represents the local definition of ModelConfig to avoid import cycles
// This should match the structure in relay/adaptor/interface.go
type ModelConfigLocal struct {
	Ratio             float64                `json:"ratio"`
	CompletionRatio   float64                `json:"completion_ratio,omitempty"`
	CachedInputRatio  float64                `json:"cached_input_ratio,omitempty"`
	CacheWrite5mRatio float64                `json:"cache_write_5m_ratio,omitempty"`
	CacheWrite1hRatio float64                `json:"cache_write_1h_ratio,omitempty"`
	Tiers             []ModelRatioTierLocal  `json:"tiers,omitempty"`
	MaxTokens         int32                  `json:"max_tokens,omitempty"`
	Video             *VideoPricingLocal     `json:"video,omitempty"`
	Audio             *AudioPricingLocal     `json:"audio,omitempty"`
	Image             *ImagePricingLocal     `json:"image,omitempty"`
	Embedding         *EmbeddingPricingLocal `json:"embedding,omitempty"`
	TimeWindows       []TimeWindowLocal      `json:"time_windows,omitempty"`
}

// TimeWindowLocal mirrors adaptor.TimeWindow for channel JSON persistence.
// Parameters: fields are user-authored JSON schedule bounds and a sparse Overlay.
// Returns: this type is data-only and does not return values.
type TimeWindowLocal struct {
	Name       string            `json:"name,omitempty"`
	TimeZone   string            `json:"timezone,omitempty"`
	Ranges     []ClockRangeLocal `json:"ranges"`
	DaysOfWeek []int             `json:"days_of_week,omitempty"`
	DateFrom   string            `json:"date_from,omitempty"`
	DateTo     string            `json:"date_to,omitempty"`
	Overlay    ModelConfigLocal  `json:"overlay"`
}

// ClockRangeLocal mirrors adaptor.ClockRange for channel JSON persistence.
// Parameters: Start and End use the "15:04" layout in the parent window timezone.
// Returns: this type is data-only and does not return values.
type ClockRangeLocal struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type ModelRatioTierLocal struct {
	Ratio                float64 `json:"ratio"`
	CompletionRatio      float64 `json:"completion_ratio,omitempty"`
	CachedInputRatio     float64 `json:"cached_input_ratio,omitempty"`
	CacheWrite5mRatio    float64 `json:"cache_write_5m_ratio,omitempty"`
	CacheWrite1hRatio    float64 `json:"cache_write_1h_ratio,omitempty"`
	InputTokenThreshold  int     `json:"input_token_threshold"`
	OutputTokenThreshold int     `json:"output_token_threshold,omitempty"`
}

// VideoPricingLocal represents channel-scoped video pricing metadata stored alongside model configs.
type VideoPricingLocal struct {
	PerSecondUsd          float64            `json:"per_second_usd,omitempty"`
	BaseResolution        string             `json:"base_resolution,omitempty"`
	ResolutionMultipliers map[string]float64 `json:"resolution_multipliers,omitempty"`
}

// AudioPricingLocal mirrors adaptor.AudioPricingConfig for persistence without creating import cycles.
type AudioPricingLocal struct {
	PromptRatio               float64 `json:"prompt_ratio,omitempty"`
	CompletionRatio           float64 `json:"completion_ratio,omitempty"`
	PromptTokensPerSecond     float64 `json:"prompt_tokens_per_second,omitempty"`
	CompletionTokensPerSecond float64 `json:"completion_tokens_per_second,omitempty"`
	UsdPerSecond              float64 `json:"usd_per_second,omitempty"`
}

// ImagePricingLocal mirrors adaptor.ImagePricingConfig for persistence.
type ImagePricingLocal struct {
	PricePerImageUsd       float64                       `json:"price_per_image_usd,omitempty"`
	PromptRatio            float64                       `json:"prompt_ratio,omitempty"`
	DefaultSize            string                        `json:"default_size,omitempty"`
	DefaultQuality         string                        `json:"default_quality,omitempty"`
	PromptTokenLimit       int                           `json:"prompt_token_limit,omitempty"`
	MinImages              int                           `json:"min_images,omitempty"`
	MaxImages              int                           `json:"max_images,omitempty"`
	SizeMultipliers        map[string]float64            `json:"size_multipliers,omitempty"`
	QualityMultipliers     map[string]float64            `json:"quality_multipliers,omitempty"`
	QualitySizeMultipliers map[string]map[string]float64 `json:"quality_size_multipliers,omitempty"`
}

// EmbeddingPricingLocal mirrors adaptor.EmbeddingPricingConfig for persistence.
type EmbeddingPricingLocal struct {
	TextTokenRatio     float64 `json:"text_token_ratio,omitempty"`
	ImageTokenRatio    float64 `json:"image_token_ratio,omitempty"`
	AudioTokenRatio    float64 `json:"audio_token_ratio,omitempty"`
	VideoTokenRatio    float64 `json:"video_token_ratio,omitempty"`
	DocumentTokenRatio float64 `json:"document_token_ratio,omitempty"`
	UsdPerImage        float64 `json:"usd_per_image,omitempty"`
	UsdPerAudioSecond  float64 `json:"usd_per_audio_second,omitempty"`
	UsdPerVideoFrame   float64 `json:"usd_per_video_frame,omitempty"`
	UsdPerDocumentPage float64 `json:"usd_per_document_page,omitempty"`
}

// normalizeModelConfigLocal trims whitespace and validates numeric fields.
func normalizeModelConfigLocal(cfg ModelConfigLocal) (ModelConfigLocal, error) {
	video, err := normalizeVideoPricingLocal(cfg.Video)
	if err != nil {
		return ModelConfigLocal{}, errors.Wrap(err, "normalize video pricing")
	}
	audio, err := normalizeAudioPricingLocal(cfg.Audio)
	if err != nil {
		return ModelConfigLocal{}, errors.Wrap(err, "normalize audio pricing")
	}
	image, err := normalizeImagePricingLocal(cfg.Image)
	if err != nil {
		return ModelConfigLocal{}, errors.Wrap(err, "normalize image pricing")
	}
	timeWindows, err := normalizeTimeWindowsLocal(cfg.TimeWindows)
	if err != nil {
		return ModelConfigLocal{}, errors.Wrap(err, "normalize time windows")
	}

	normalized := ModelConfigLocal{
		Ratio:             cfg.Ratio,
		CompletionRatio:   cfg.CompletionRatio,
		CachedInputRatio:  cfg.CachedInputRatio,
		CacheWrite5mRatio: cfg.CacheWrite5mRatio,
		CacheWrite1hRatio: cfg.CacheWrite1hRatio,
		MaxTokens:         cfg.MaxTokens,
	}
	if len(cfg.Tiers) > 0 {
		normalized.Tiers = append([]ModelRatioTierLocal(nil), cfg.Tiers...)
	}
	if video != nil {
		normalized.Video = video
	}
	if audio != nil {
		normalized.Audio = audio
	}
	if image != nil {
		normalized.Image = image
	}
	if cfg.Embedding != nil {
		normalized.Embedding = cfg.Embedding
	}
	if len(timeWindows) > 0 {
		normalized.TimeWindows = timeWindows
	}
	return normalized, nil
}

// normalizeTimeWindowsLocal trims and validates time-window schedules for channel JSON.
// Parameters: windows is the user-authored list whose order defines precedence.
// Returns: the normalized list, or an error describing the invalid window field.
func normalizeTimeWindowsLocal(windows []TimeWindowLocal) ([]TimeWindowLocal, error) {
	if len(windows) == 0 {
		return nil, nil
	}

	normalized := make([]TimeWindowLocal, 0, len(windows))
	for idx, window := range windows {
		if len(window.Overlay.TimeWindows) > 0 {
			return nil, errors.Errorf("time window %d overlay cannot contain time_windows", idx)
		}
		tz := strings.TrimSpace(window.TimeZone)
		if tz == "" {
			tz = "UTC"
		}
		if _, err := time.LoadLocation(tz); err != nil {
			return nil, errors.Wrapf(err, "load timezone for time window %d", idx)
		}
		if len(window.Ranges) == 0 {
			return nil, errors.Errorf("time window %d must include at least one range", idx)
		}

		ranges := make([]ClockRangeLocal, 0, len(window.Ranges))
		for rangeIdx, clockRange := range window.Ranges {
			start := strings.TrimSpace(clockRange.Start)
			end := strings.TrimSpace(clockRange.End)
			if _, err := time.Parse("15:04", start); err != nil {
				return nil, errors.Wrapf(err, "parse start for time window %d range %d", idx, rangeIdx)
			}
			if _, err := time.Parse("15:04", end); err != nil {
				return nil, errors.Wrapf(err, "parse end for time window %d range %d", idx, rangeIdx)
			}
			ranges = append(ranges, ClockRangeLocal{Start: start, End: end})
		}

		days := append([]int(nil), window.DaysOfWeek...)
		for _, day := range days {
			if day < 0 || day > 6 {
				return nil, errors.Errorf("time window %d day_of_week must be between 0 and 6", idx)
			}
		}

		dateFrom := strings.TrimSpace(window.DateFrom)
		dateTo := strings.TrimSpace(window.DateTo)
		var parsedFrom time.Time
		var hasFrom bool
		if dateFrom != "" {
			parsed, err := time.Parse("2006-01-02", dateFrom)
			if err != nil {
				return nil, errors.Wrapf(err, "parse date_from for time window %d", idx)
			}
			parsedFrom = parsed
			hasFrom = true
		}
		if dateTo != "" {
			parsedTo, err := time.Parse("2006-01-02", dateTo)
			if err != nil {
				return nil, errors.Wrapf(err, "parse date_to for time window %d", idx)
			}
			if hasFrom && !parsedFrom.Before(parsedTo) {
				return nil, errors.Errorf("time window %d date_from must be before date_to", idx)
			}
		}

		overlay, err := normalizeModelConfigLocal(window.Overlay)
		if err != nil {
			return nil, errors.Wrapf(err, "normalize overlay for time window %d", idx)
		}
		if !hasOverlayPricingData(overlay) {
			return nil, errors.Errorf("time window %d overlay must include at least one pricing field", idx)
		}

		normalized = append(normalized, TimeWindowLocal{
			Name:       strings.TrimSpace(window.Name),
			TimeZone:   tz,
			Ranges:     ranges,
			DaysOfWeek: days,
			DateFrom:   dateFrom,
			DateTo:     dateTo,
			Overlay:    overlay,
		})
	}
	return normalized, nil
}

// validateModelPriceConfigs validates the structure and values of ModelPriceLocal configurations
func (channel *Channel) validateModelPriceConfigs(configs map[string]ModelConfigLocal) error {
	if configs == nil {
		return nil
	}

	for modelName, config := range configs {
		// Validate model name
		if modelName == "" {
			return errors.New("empty model name found")
		}

		// Validate ratio values
		if config.Ratio < 0 {
			return errors.Errorf("negative ratio for model %s: %f", modelName, config.Ratio)
		}
		if config.CompletionRatio < 0 {
			return errors.Errorf("negative completion ratio for model %s: %f", modelName, config.CompletionRatio)
		}
		if config.CacheWrite5mRatio < 0 {
			return errors.Errorf("negative cache_write_5m_ratio for model %s: %f", modelName, config.CacheWrite5mRatio)
		}
		if config.CacheWrite1hRatio < 0 {
			return errors.Errorf("negative cache_write_1h_ratio for model %s: %f", modelName, config.CacheWrite1hRatio)
		}
		for _, tier := range config.Tiers {
			if tier.InputTokenThreshold < 0 {
				return errors.Errorf("negative input_token_threshold for model %s tier: %d", modelName, tier.InputTokenThreshold)
			}
			if tier.OutputTokenThreshold < 0 {
				return errors.Errorf("negative output_token_threshold for model %s tier: %d", modelName, tier.OutputTokenThreshold)
			}
			if tier.Ratio < 0 {
				return errors.Errorf("negative tier ratio for model %s: %f", modelName, tier.Ratio)
			}
			if tier.CompletionRatio < 0 {
				return errors.Errorf("negative tier completion ratio for model %s: %f", modelName, tier.CompletionRatio)
			}
			if tier.CacheWrite5mRatio < 0 {
				return errors.Errorf("negative tier cache_write_5m_ratio for model %s: %f", modelName, tier.CacheWrite5mRatio)
			}
			if tier.CacheWrite1hRatio < 0 {
				return errors.Errorf("negative tier cache_write_1h_ratio for model %s: %f", modelName, tier.CacheWrite1hRatio)
			}
		}

		// Validate MaxTokens
		if config.MaxTokens < 0 {
			return errors.Errorf("negative MaxTokens for model %s: %d", modelName, config.MaxTokens)
		}

		hasVideoData, err := validateVideoPricingLocal(config.Video, modelName)
		if err != nil {
			return errors.Wrap(err, "validate video pricing")
		}
		hasAudioData, err := validateAudioPricingLocal(config.Audio, modelName)
		if err != nil {
			return errors.Wrap(err, "validate audio pricing")
		}
		hasImageData, err := validateImagePricingLocal(config.Image, modelName)
		if err != nil {
			return errors.Wrap(err, "validate image pricing")
		}
		hasEmbeddingData, err := validateEmbeddingPricingLocal(config.Embedding, modelName)
		if err != nil {
			return errors.Wrap(err, "validate embedding pricing")
		}
		hasTimeWindowData, err := validateTimeWindowsLocal(config.TimeWindows, modelName)
		if err != nil {
			return errors.Wrap(err, "validate time windows")
		}

		// Validate that at least one field has meaningful data
		if config.Ratio == 0 &&
			config.CompletionRatio == 0 &&
			config.CachedInputRatio == 0 &&
			config.CacheWrite5mRatio == 0 &&
			config.CacheWrite1hRatio == 0 &&
			len(config.Tiers) == 0 &&
			config.MaxTokens == 0 &&
			!hasVideoData &&
			!hasAudioData &&
			!hasImageData &&
			!hasEmbeddingData &&
			!hasTimeWindowData {
			return errors.Errorf("model %s has no meaningful configuration data", modelName)
		}
	}

	return nil
}

// validateTimeWindowsLocal validates schedule and overlay semantics for a model's time windows.
// Parameters: windows is the ordered channel override list and modelName names the owning model.
// Returns: true when at least one window is present, or an error for malformed window data.
func validateTimeWindowsLocal(windows []TimeWindowLocal, modelName string) (bool, error) {
	if len(windows) == 0 {
		return false, nil
	}
	for idx, window := range windows {
		tz := strings.TrimSpace(window.TimeZone)
		if tz == "" {
			tz = "UTC"
		}
		if _, err := time.LoadLocation(tz); err != nil {
			return false, errors.Wrapf(err, "invalid timezone for model %s time window %d", modelName, idx)
		}
		if len(window.Ranges) == 0 {
			return false, errors.Errorf("model %s time window %d must include at least one range", modelName, idx)
		}
		for rangeIdx, clockRange := range window.Ranges {
			if _, err := time.Parse("15:04", strings.TrimSpace(clockRange.Start)); err != nil {
				return false, errors.Wrapf(err, "invalid start for model %s time window %d range %d", modelName, idx, rangeIdx)
			}
			if _, err := time.Parse("15:04", strings.TrimSpace(clockRange.End)); err != nil {
				return false, errors.Wrapf(err, "invalid end for model %s time window %d range %d", modelName, idx, rangeIdx)
			}
		}
		for _, day := range window.DaysOfWeek {
			if day < 0 || day > 6 {
				return false, errors.Errorf("model %s time window %d day_of_week must be between 0 and 6", modelName, idx)
			}
		}
		var parsedFrom time.Time
		var hasFrom bool
		if strings.TrimSpace(window.DateFrom) != "" {
			parsed, err := time.Parse("2006-01-02", strings.TrimSpace(window.DateFrom))
			if err != nil {
				return false, errors.Wrapf(err, "invalid date_from for model %s time window %d", modelName, idx)
			}
			parsedFrom = parsed
			hasFrom = true
		}
		if strings.TrimSpace(window.DateTo) != "" {
			parsedTo, err := time.Parse("2006-01-02", strings.TrimSpace(window.DateTo))
			if err != nil {
				return false, errors.Wrapf(err, "invalid date_to for model %s time window %d", modelName, idx)
			}
			if hasFrom && !parsedFrom.Before(parsedTo) {
				return false, errors.Errorf("model %s time window %d date_from must be before date_to", modelName, idx)
			}
		}
		if len(window.Overlay.TimeWindows) > 0 {
			return false, errors.Errorf("model %s time window %d overlay cannot contain time_windows", modelName, idx)
		}
		if err := validateTimeWindowOverlayLocal(window.Overlay, modelName, idx); err != nil {
			return false, errors.Wrap(err, "validate time window overlay")
		}
	}
	return true, nil
}

// validateTimeWindowOverlayLocal validates sparse pricing fields allowed inside a time-window overlay.
// Parameters: overlay is the sparse pricing config, modelName names the model, and windowIdx identifies the window.
// Returns: an error when the overlay is empty or contains invalid pricing values.
func validateTimeWindowOverlayLocal(overlay ModelConfigLocal, modelName string, windowIdx int) error {
	if overlay.Ratio < 0 {
		return errors.Errorf("model %s time window %d overlay ratio cannot be negative", modelName, windowIdx)
	}
	if overlay.CompletionRatio < 0 {
		return errors.Errorf("model %s time window %d overlay completion_ratio cannot be negative", modelName, windowIdx)
	}
	if overlay.CacheWrite5mRatio < 0 {
		return errors.Errorf("model %s time window %d overlay cache_write_5m_ratio cannot be negative", modelName, windowIdx)
	}
	if overlay.CacheWrite1hRatio < 0 {
		return errors.Errorf("model %s time window %d overlay cache_write_1h_ratio cannot be negative", modelName, windowIdx)
	}
	for _, tier := range overlay.Tiers {
		if tier.InputTokenThreshold < 0 {
			return errors.Errorf("model %s time window %d overlay tier threshold cannot be negative", modelName, windowIdx)
		}
		if tier.Ratio < 0 {
			return errors.Errorf("model %s time window %d overlay tier ratio cannot be negative", modelName, windowIdx)
		}
		if tier.CompletionRatio < 0 {
			return errors.Errorf("model %s time window %d overlay tier completion_ratio cannot be negative", modelName, windowIdx)
		}
		if tier.CacheWrite5mRatio < 0 {
			return errors.Errorf("model %s time window %d overlay tier cache_write_5m_ratio cannot be negative", modelName, windowIdx)
		}
		if tier.CacheWrite1hRatio < 0 {
			return errors.Errorf("model %s time window %d overlay tier cache_write_1h_ratio cannot be negative", modelName, windowIdx)
		}
	}
	hasVideoData, err := validateVideoPricingLocal(overlay.Video, modelName)
	if err != nil {
		return errors.Wrap(err, "validate overlay video pricing")
	}
	hasAudioData, err := validateAudioPricingLocal(overlay.Audio, modelName)
	if err != nil {
		return errors.Wrap(err, "validate overlay audio pricing")
	}
	hasImageData, err := validateImagePricingLocal(overlay.Image, modelName)
	if err != nil {
		return errors.Wrap(err, "validate overlay image pricing")
	}
	hasEmbeddingData, err := validateEmbeddingPricingLocal(overlay.Embedding, modelName)
	if err != nil {
		return errors.Wrap(err, "validate overlay embedding pricing")
	}
	if !hasOverlayPricingData(overlay) && !hasVideoData && !hasAudioData && !hasImageData && !hasEmbeddingData {
		return errors.Errorf("model %s time window %d overlay must include at least one pricing field", modelName, windowIdx)
	}
	return nil
}

// hasOverlayPricingData reports whether a sparse overlay carries pricing data.
// Parameters: cfg is the normalized local model config.
// Returns: true when token, tier, or nested pricing fields are present.
func hasOverlayPricingData(cfg ModelConfigLocal) bool {
	return cfg.Ratio != 0 ||
		cfg.CompletionRatio != 0 ||
		cfg.CachedInputRatio != 0 ||
		cfg.CacheWrite5mRatio != 0 ||
		cfg.CacheWrite1hRatio != 0 ||
		len(cfg.Tiers) > 0 ||
		hasVideoPricingData(cfg.Video) ||
		hasAudioPricingData(cfg.Audio) ||
		hasImagePricingData(cfg.Image) ||
		hasEmbeddingPricingData(cfg.Embedding)
}

func normalizeVideoPricingLocal(cfg *VideoPricingLocal) (*VideoPricingLocal, error) {
	if cfg == nil {
		return nil, nil
	}

	if cfg.PerSecondUsd < 0 {
		return nil, errors.New("video per_second_usd cannot be negative")
	}

	normalized := &VideoPricingLocal{
		PerSecondUsd: cfg.PerSecondUsd,
	}
	if strings.TrimSpace(cfg.BaseResolution) != "" {
		normalized.BaseResolution = normalizeVideoResolutionKey(cfg.BaseResolution)
	}

	if len(cfg.ResolutionMultipliers) > 0 {
		normalized.ResolutionMultipliers = make(map[string]float64, len(cfg.ResolutionMultipliers))
		for rawKey, value := range cfg.ResolutionMultipliers {
			key := normalizeVideoResolutionKey(rawKey)
			if key == "" {
				return nil, errors.Errorf("video resolution multiplier key cannot be empty for '%s'", rawKey)
			}
			if value <= 0 {
				return nil, errors.Errorf("video resolution multiplier for %s must be positive", rawKey)
			}
			normalized.ResolutionMultipliers[key] = value
		}
	}

	return normalized, nil
}

func validateVideoPricingLocal(cfg *VideoPricingLocal, modelName string) (bool, error) {
	if cfg == nil {
		return false, nil
	}
	if cfg.PerSecondUsd < 0 {
		return false, errors.Errorf("video per_second_usd cannot be negative for model %s", modelName)
	}
	for key, value := range cfg.ResolutionMultipliers {
		if strings.TrimSpace(key) == "" {
			return false, errors.Errorf("video resolution multiplier key cannot be empty for model %s", modelName)
		}
		if value <= 0 {
			return false, errors.Errorf("video resolution multiplier for %s must be positive (model %s)", key, modelName)
		}
	}
	return hasVideoPricingData(cfg), nil
}

func hasVideoPricingData(cfg *VideoPricingLocal) bool {
	if cfg == nil {
		return false
	}
	if cfg.PerSecondUsd > 0 {
		return true
	}
	return len(cfg.ResolutionMultipliers) > 0
}

func validateAudioPricingLocal(cfg *AudioPricingLocal, modelName string) (bool, error) {
	if cfg == nil {
		return false, nil
	}
	if cfg.PromptRatio < 0 {
		return false, errors.Errorf("audio prompt_ratio cannot be negative for model %s", modelName)
	}
	if cfg.CompletionRatio < 0 {
		return false, errors.Errorf("audio completion_ratio cannot be negative for model %s", modelName)
	}
	if cfg.PromptTokensPerSecond < 0 {
		return false, errors.Errorf("audio prompt_tokens_per_second cannot be negative for model %s", modelName)
	}
	if cfg.CompletionTokensPerSecond < 0 {
		return false, errors.Errorf("audio completion_tokens_per_second cannot be negative for model %s", modelName)
	}
	if cfg.UsdPerSecond < 0 {
		return false, errors.Errorf("audio usd_per_second cannot be negative for model %s", modelName)
	}
	return hasAudioPricingData(cfg), nil
}

func hasAudioPricingData(cfg *AudioPricingLocal) bool {
	if cfg == nil {
		return false
	}
	return cfg.PromptRatio != 0 || cfg.CompletionRatio != 0 || cfg.PromptTokensPerSecond != 0 ||
		cfg.CompletionTokensPerSecond != 0 || cfg.UsdPerSecond != 0
}

func validateImagePricingLocal(cfg *ImagePricingLocal, modelName string) (bool, error) {
	if cfg == nil {
		return false, nil
	}
	if cfg.PricePerImageUsd < 0 {
		return false, errors.Errorf("image price_per_image_usd cannot be negative for model %s", modelName)
	}
	if cfg.PromptRatio < 0 {
		return false, errors.Errorf("image prompt_ratio cannot be negative for model %s", modelName)
	}
	if cfg.PromptTokenLimit < 0 {
		return false, errors.Errorf("image prompt_token_limit cannot be negative for model %s", modelName)
	}
	if cfg.MinImages < 0 {
		return false, errors.Errorf("image min_images cannot be negative for model %s", modelName)
	}
	if cfg.MaxImages < 0 {
		return false, errors.Errorf("image max_images cannot be negative for model %s", modelName)
	}
	if cfg.MinImages > 0 && cfg.MaxImages > 0 && cfg.MinImages > cfg.MaxImages {
		return false, errors.Errorf("image min_images cannot exceed max_images for model %s", modelName)
	}
	if len(cfg.SizeMultipliers) > 0 {
		for key, value := range cfg.SizeMultipliers {
			if strings.TrimSpace(key) == "" {
				return false, errors.Errorf("image size multiplier key cannot be empty for model %s", modelName)
			}
			if value <= 0 {
				return false, errors.Errorf("image size multiplier for %s (%s) must be positive", modelName, key)
			}
		}
	}
	if len(cfg.QualityMultipliers) > 0 {
		for key, value := range cfg.QualityMultipliers {
			if strings.TrimSpace(key) == "" {
				return false, errors.Errorf("image quality multiplier key cannot be empty for model %s", modelName)
			}
			if value <= 0 {
				return false, errors.Errorf("image quality multiplier for %s (%s) must be positive", modelName, key)
			}
		}
	}
	if len(cfg.QualitySizeMultipliers) > 0 {
		for quality, sizeMap := range cfg.QualitySizeMultipliers {
			if strings.TrimSpace(quality) == "" {
				return false, errors.Errorf("image quality-size multiplier quality cannot be empty for model %s", modelName)
			}
			for size, value := range sizeMap {
				if strings.TrimSpace(size) == "" {
					return false, errors.Errorf("image quality-size multiplier size cannot be empty for model %s quality %s", modelName, quality)
				}
				if value <= 0 {
					return false, errors.Errorf("image quality-size multiplier for %s (%s/%s) must be positive", modelName, quality, size)
				}
			}
		}
	}
	return hasImagePricingData(cfg), nil
}

func hasImagePricingData(cfg *ImagePricingLocal) bool {
	if cfg == nil {
		return false
	}
	if cfg.PricePerImageUsd > 0 || cfg.PromptRatio > 0 || cfg.PromptTokenLimit > 0 {
		return true
	}
	if cfg.MinImages > 0 || cfg.MaxImages > 0 {
		return true
	}
	return len(cfg.SizeMultipliers) > 0 || len(cfg.QualityMultipliers) > 0 || len(cfg.QualitySizeMultipliers) > 0
}

// validateEmbeddingPricingLocal validates channel-scoped embedding pricing metadata.
// Parameters: cfg is the embedding block and modelName names the owning model.
// Returns: true when the block has pricing data, or an error for invalid values.
func validateEmbeddingPricingLocal(cfg *EmbeddingPricingLocal, modelName string) (bool, error) {
	if cfg == nil {
		return false, nil
	}
	if cfg.TextTokenRatio < 0 {
		return false, errors.Errorf("embedding text_token_ratio cannot be negative for model %s", modelName)
	}
	if cfg.ImageTokenRatio < 0 {
		return false, errors.Errorf("embedding image_token_ratio cannot be negative for model %s", modelName)
	}
	if cfg.AudioTokenRatio < 0 {
		return false, errors.Errorf("embedding audio_token_ratio cannot be negative for model %s", modelName)
	}
	if cfg.VideoTokenRatio < 0 {
		return false, errors.Errorf("embedding video_token_ratio cannot be negative for model %s", modelName)
	}
	if cfg.DocumentTokenRatio < 0 {
		return false, errors.Errorf("embedding document_token_ratio cannot be negative for model %s", modelName)
	}
	if cfg.UsdPerImage < 0 {
		return false, errors.Errorf("embedding usd_per_image cannot be negative for model %s", modelName)
	}
	if cfg.UsdPerAudioSecond < 0 {
		return false, errors.Errorf("embedding usd_per_audio_second cannot be negative for model %s", modelName)
	}
	if cfg.UsdPerVideoFrame < 0 {
		return false, errors.Errorf("embedding usd_per_video_frame cannot be negative for model %s", modelName)
	}
	if cfg.UsdPerDocumentPage < 0 {
		return false, errors.Errorf("embedding usd_per_document_page cannot be negative for model %s", modelName)
	}
	return hasEmbeddingPricingData(cfg), nil
}

// hasEmbeddingPricingData reports whether the embedding block contains pricing metadata.
// Parameters: cfg is the optional embedding pricing block.
// Returns: true when any ratio or direct USD field is non-zero.
func hasEmbeddingPricingData(cfg *EmbeddingPricingLocal) bool {
	if cfg == nil {
		return false
	}
	return cfg.TextTokenRatio != 0 || cfg.ImageTokenRatio != 0 || cfg.AudioTokenRatio != 0 ||
		cfg.VideoTokenRatio != 0 || cfg.DocumentTokenRatio != 0 || cfg.UsdPerImage != 0 ||
		cfg.UsdPerAudioSecond != 0 || cfg.UsdPerVideoFrame != 0 || cfg.UsdPerDocumentPage != 0
}

func normalizeVideoResolutionKey(value string) string {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return ""
	}
	parts := strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == 'x' || r == '*' || r == '×'
	})
	if len(parts) != 2 {
		return trimmed
	}
	width, err1 := strconv.Atoi(parts[0])
	height, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || width <= 0 || height <= 0 {
		return trimmed
	}
	if width < height {
		width, height = height, width
	}
	return strconv.Itoa(width) + "x" + strconv.Itoa(height)
}

func normalizeAudioPricingLocal(cfg *AudioPricingLocal) (*AudioPricingLocal, error) {
	if cfg == nil {
		return nil, nil
	}
	if cfg.PromptRatio < 0 {
		return nil, errors.New("audio prompt_ratio cannot be negative")
	}
	if cfg.CompletionRatio < 0 {
		return nil, errors.New("audio completion_ratio cannot be negative")
	}
	if cfg.PromptTokensPerSecond < 0 {
		return nil, errors.New("audio prompt_tokens_per_second cannot be negative")
	}
	if cfg.CompletionTokensPerSecond < 0 {
		return nil, errors.New("audio completion_tokens_per_second cannot be negative")
	}
	if cfg.UsdPerSecond < 0 {
		return nil, errors.New("audio usd_per_second cannot be negative")
	}
	normalized := &AudioPricingLocal{
		PromptRatio:               cfg.PromptRatio,
		CompletionRatio:           cfg.CompletionRatio,
		PromptTokensPerSecond:     cfg.PromptTokensPerSecond,
		CompletionTokensPerSecond: cfg.CompletionTokensPerSecond,
		UsdPerSecond:              cfg.UsdPerSecond,
	}
	return normalized, nil
}

func normalizeImagePricingLocal(cfg *ImagePricingLocal) (*ImagePricingLocal, error) {
	if cfg == nil {
		return nil, nil
	}
	if cfg.PricePerImageUsd < 0 {
		return nil, errors.New("image price_per_image_usd cannot be negative")
	}
	if cfg.PromptRatio < 0 {
		return nil, errors.New("image prompt_ratio cannot be negative")
	}
	if cfg.PromptTokenLimit < 0 {
		return nil, errors.New("image prompt_token_limit cannot be negative")
	}
	if cfg.MinImages < 0 {
		return nil, errors.New("image min_images cannot be negative")
	}
	if cfg.MaxImages < 0 {
		return nil, errors.New("image max_images cannot be negative")
	}
	if cfg.MinImages > 0 && cfg.MaxImages > 0 && cfg.MinImages > cfg.MaxImages {
		return nil, errors.New("image min_images cannot exceed max_images")
	}
	normalized := &ImagePricingLocal{
		PricePerImageUsd: cfg.PricePerImageUsd,
		PromptRatio:      cfg.PromptRatio,
		PromptTokenLimit: cfg.PromptTokenLimit,
		MinImages:        cfg.MinImages,
		MaxImages:        cfg.MaxImages,
	}
	if trimmed := strings.TrimSpace(cfg.DefaultSize); trimmed != "" {
		normalized.DefaultSize = normalizeImageSizeKey(trimmed)
	}
	if trimmedQuality := strings.TrimSpace(cfg.DefaultQuality); trimmedQuality != "" {
		normalized.DefaultQuality = normalizeImageQualityKey(trimmedQuality)
	}
	if len(cfg.SizeMultipliers) > 0 {
		normalized.SizeMultipliers = make(map[string]float64, len(cfg.SizeMultipliers))
		for raw, value := range cfg.SizeMultipliers {
			key := normalizeImageSizeKey(raw)
			if key == "" {
				return nil, errors.Errorf("image size multiplier key cannot be empty (input: %q)", raw)
			}
			if value <= 0 {
				return nil, errors.Errorf("image size multiplier for %s must be positive", raw)
			}
			normalized.SizeMultipliers[key] = value
		}
	}
	if len(cfg.QualityMultipliers) > 0 {
		normalized.QualityMultipliers = make(map[string]float64, len(cfg.QualityMultipliers))
		for raw, value := range cfg.QualityMultipliers {
			key := normalizeImageQualityKey(raw)
			if key == "" {
				return nil, errors.Errorf("image quality multiplier key cannot be empty (input: %q)", raw)
			}
			if value <= 0 {
				return nil, errors.Errorf("image quality multiplier for %s must be positive", raw)
			}
			normalized.QualityMultipliers[key] = value
		}
	}
	if len(cfg.QualitySizeMultipliers) > 0 {
		normalized.QualitySizeMultipliers = make(map[string]map[string]float64, len(cfg.QualitySizeMultipliers))
		for rawQuality, sizeMap := range cfg.QualitySizeMultipliers {
			qualityKey := normalizeImageQualityKey(rawQuality)
			if qualityKey == "" {
				return nil, errors.Errorf("image quality-size multiplier quality cannot be empty (input: %q)", rawQuality)
			}
			if len(sizeMap) == 0 {
				continue
			}
			normalizedSizes := make(map[string]float64, len(sizeMap))
			for rawSize, value := range sizeMap {
				sizeKey := normalizeImageSizeKey(rawSize)
				if sizeKey == "" {
					return nil, errors.Errorf("image quality-size multiplier size cannot be empty (quality: %s)", rawQuality)
				}
				if value <= 0 {
					return nil, errors.Errorf("image quality-size multiplier for %s/%s must be positive", rawQuality, rawSize)
				}
				normalizedSizes[sizeKey] = value
			}
			if len(normalizedSizes) > 0 {
				normalized.QualitySizeMultipliers[qualityKey] = normalizedSizes
			}
		}
	}
	return normalized, nil
}

func normalizeImageSizeKey(value string) string {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	trimmed = strings.ReplaceAll(trimmed, "×", "x")
	trimmed = strings.ReplaceAll(trimmed, "*", "x")
	trimmed = strings.ReplaceAll(trimmed, " ", "")
	return trimmed
}

func normalizeImageQualityKey(value string) string {
	return strings.TrimSpace(strings.ToLower(value))
}

func floatFromAny(value any) (float64, bool) {
	switch v := value.(type) {
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return 0, false
		}
		return f, true
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case int32:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint64:
		return float64(v), true
	case uint32:
		return float64(v), true
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return 0, false
		}
		f, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}
