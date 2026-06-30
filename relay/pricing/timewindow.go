package pricing

import (
	"maps"
	"sync"
	"time"

	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/relay/adaptor"
)

var timeWindowLocationCache sync.Map

// ApplyTimeWindow applies the first matching time-of-day pricing overlay.
// Parameters: cfg is the resolved base model config and at is the request start time.
// Returns: cfg unchanged when no windows exist or match, otherwise a merged config with TimeWindows cleared.
func ApplyTimeWindow(cfg adaptor.ModelConfig, at time.Time) adaptor.ModelConfig {
	return applyTimeWindow(cfg, at, false)
}

// ApplyTimeWindowRatioOnly applies the first matching overlay for token-only billing.
// Parameters: cfg is the resolved base model config and at is the request start time.
// Returns: cfg unchanged when no windows exist or match, otherwise a scalar/tier merged config with TimeWindows cleared.
func ApplyTimeWindowRatioOnly(cfg adaptor.ModelConfig, at time.Time) adaptor.ModelConfig {
	return applyTimeWindow(cfg, at, true)
}

// ActiveTimeWindowName returns the first window name that matches at.
// Parameters: cfg is the resolved model config and at is the instant to evaluate.
// Returns: the matching window name, or an empty string when no named window is active.
func ActiveTimeWindowName(cfg adaptor.ModelConfig, at time.Time) string {
	if len(cfg.TimeWindows) == 0 {
		return ""
	}
	for _, window := range cfg.TimeWindows {
		matched, err := matchWindow(window, at)
		if err != nil {
			logger.Logger.Debug("skip invalid time pricing window",
				zap.String("window", window.Name),
				zap.Error(err))
			continue
		}
		if matched {
			return window.Name
		}
	}
	return ""
}

// MatchTimeWindow reports whether a time window matches an instant.
// Parameters: window is the pricing window and at is the instant to evaluate.
// Returns: true when date, weekday, and clock ranges all match.
func MatchTimeWindow(window adaptor.TimeWindow, at time.Time) bool {
	matched, err := matchWindow(window, at)
	return err == nil && matched
}

func applyTimeWindow(cfg adaptor.ModelConfig, at time.Time, ratioOnly bool) adaptor.ModelConfig {
	if len(cfg.TimeWindows) == 0 {
		return cfg
	}
	if at.IsZero() {
		at = time.Now()
	}
	for _, window := range cfg.TimeWindows {
		matched, err := matchWindow(window, at)
		if err != nil {
			logger.Logger.Debug("skip invalid time pricing window",
				zap.String("window", window.Name),
				zap.Error(err))
			continue
		}
		if !matched {
			continue
		}
		if ratioOnly {
			return mergePricingRatioOnly(cfg, window.Overlay)
		}
		return mergePricing(cfg, window.Overlay)
	}
	return cfg
}

func matchWindow(window adaptor.TimeWindow, at time.Time) (bool, error) {
	loc, err := loadLocationCached(window.TimeZone)
	if err != nil {
		return false, err
	}
	local := at.In(loc)

	if window.DateFrom != "" || window.DateTo != "" {
		localDate := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
		if window.DateFrom != "" {
			from, err := time.ParseInLocation("2006-01-02", window.DateFrom, loc)
			if err != nil {
				return false, err
			}
			if localDate.Before(from) {
				return false, nil
			}
		}
		if window.DateTo != "" {
			to, err := time.ParseInLocation("2006-01-02", window.DateTo, loc)
			if err != nil {
				return false, err
			}
			if !localDate.Before(to) {
				return false, nil
			}
		}
	}

	if len(window.DaysOfWeek) > 0 {
		weekday := int(local.Weekday())
		found := false
		for _, day := range window.DaysOfWeek {
			if day == weekday {
				found = true
				break
			}
		}
		if !found {
			return false, nil
		}
	}

	if len(window.Ranges) == 0 {
		return false, nil
	}
	tod := local.Hour()*60 + local.Minute()
	for _, clockRange := range window.Ranges {
		start, err := parseClockMinutes(clockRange.Start)
		if err != nil {
			return false, err
		}
		end, err := parseClockMinutes(clockRange.End)
		if err != nil {
			return false, err
		}
		if start == end {
			return true, nil
		}
		if start < end {
			if tod >= start && tod < end {
				return true, nil
			}
			continue
		}
		if tod >= start || tod < end {
			return true, nil
		}
	}
	return false, nil
}

func parseClockMinutes(value string) (int, error) {
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return 0, err
	}
	return parsed.Hour()*60 + parsed.Minute(), nil
}

func loadLocationCached(tz string) (*time.Location, error) {
	if tz == "" {
		tz = "UTC"
	}
	if cached, ok := timeWindowLocationCache.Load(tz); ok {
		return cached.(*time.Location), nil
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, err
	}
	actual, _ := timeWindowLocationCache.LoadOrStore(tz, loc)
	return actual.(*time.Location), nil
}

func mergePricingRatioOnly(base adaptor.ModelConfig, overlay adaptor.ModelConfig) adaptor.ModelConfig {
	merged := base.Clone()
	mergeScalarPricing(&merged, overlay)
	if len(overlay.Tiers) > 0 {
		merged.Tiers = append([]adaptor.ModelRatioTier(nil), overlay.Tiers...)
	}
	merged.Embedding = mergeEmbeddingPricing(merged.Embedding, overlay.Embedding)
	merged.Video = nil
	merged.Audio = nil
	merged.Image = nil
	merged.PerCall = nil
	merged.TimeWindows = nil
	return merged
}

func mergePricing(base adaptor.ModelConfig, overlay adaptor.ModelConfig) adaptor.ModelConfig {
	merged := base.Clone()
	mergeScalarPricing(&merged, overlay)
	if len(overlay.Tiers) > 0 {
		merged.Tiers = append([]adaptor.ModelRatioTier(nil), overlay.Tiers...)
	}
	merged.Video = mergeVideoPricing(merged.Video, overlay.Video)
	merged.Audio = mergeAudioPricing(merged.Audio, overlay.Audio)
	merged.Image = mergeImagePricing(merged.Image, overlay.Image)
	merged.Embedding = mergeEmbeddingPricing(merged.Embedding, overlay.Embedding)
	merged.PerCall = mergePerCallPricing(merged.PerCall, overlay.PerCall)
	merged.TimeWindows = nil
	return merged
}

func mergeScalarPricing(base *adaptor.ModelConfig, overlay adaptor.ModelConfig) {
	if overlay.Ratio != 0 {
		base.Ratio = overlay.Ratio
	}
	if overlay.CompletionRatio != 0 {
		base.CompletionRatio = overlay.CompletionRatio
	}
	if overlay.CachedInputRatio != 0 {
		base.CachedInputRatio = overlay.CachedInputRatio
	}
	if overlay.CacheWrite5mRatio != 0 {
		base.CacheWrite5mRatio = overlay.CacheWrite5mRatio
	}
	if overlay.CacheWrite1hRatio != 0 {
		base.CacheWrite1hRatio = overlay.CacheWrite1hRatio
	}
}

func mergeVideoPricing(base *adaptor.VideoPricingConfig, overlay *adaptor.VideoPricingConfig) *adaptor.VideoPricingConfig {
	if overlay == nil {
		return base
	}
	if base == nil {
		return overlay.Clone()
	}
	merged := base.Clone()
	if overlay.PerSecondUsd != 0 {
		merged.PerSecondUsd = overlay.PerSecondUsd
	}
	if overlay.BaseResolution != "" {
		merged.BaseResolution = overlay.BaseResolution
	}
	merged.ResolutionMultipliers = mergeFloatMap(merged.ResolutionMultipliers, overlay.ResolutionMultipliers)
	return merged
}

func mergeAudioPricing(base *adaptor.AudioPricingConfig, overlay *adaptor.AudioPricingConfig) *adaptor.AudioPricingConfig {
	if overlay == nil {
		return base
	}
	if base == nil {
		return overlay.Clone()
	}
	merged := base.Clone()
	if overlay.PromptRatio != 0 {
		merged.PromptRatio = overlay.PromptRatio
	}
	if overlay.CompletionRatio != 0 {
		merged.CompletionRatio = overlay.CompletionRatio
	}
	if overlay.PromptTokensPerSecond != 0 {
		merged.PromptTokensPerSecond = overlay.PromptTokensPerSecond
	}
	if overlay.CompletionTokensPerSecond != 0 {
		merged.CompletionTokensPerSecond = overlay.CompletionTokensPerSecond
	}
	if overlay.UsdPerSecond != 0 {
		merged.UsdPerSecond = overlay.UsdPerSecond
	}
	return merged
}

func mergeImagePricing(base *adaptor.ImagePricingConfig, overlay *adaptor.ImagePricingConfig) *adaptor.ImagePricingConfig {
	if overlay == nil {
		return base
	}
	if base == nil {
		return overlay.Clone()
	}
	merged := base.Clone()
	if overlay.PricePerImageUsd != 0 {
		merged.PricePerImageUsd = overlay.PricePerImageUsd
	}
	if overlay.PromptRatio != 0 {
		merged.PromptRatio = overlay.PromptRatio
	}
	if overlay.DefaultSize != "" {
		merged.DefaultSize = overlay.DefaultSize
	}
	if overlay.DefaultQuality != "" {
		merged.DefaultQuality = overlay.DefaultQuality
	}
	if overlay.PromptTokenLimit != 0 {
		merged.PromptTokenLimit = overlay.PromptTokenLimit
	}
	if overlay.MinImages != 0 {
		merged.MinImages = overlay.MinImages
	}
	if overlay.MaxImages != 0 {
		merged.MaxImages = overlay.MaxImages
	}
	merged.SizeMultipliers = mergeFloatMap(merged.SizeMultipliers, overlay.SizeMultipliers)
	merged.QualityMultipliers = mergeFloatMap(merged.QualityMultipliers, overlay.QualityMultipliers)
	merged.QualitySizeMultipliers = mergeNestedFloatMap(merged.QualitySizeMultipliers, overlay.QualitySizeMultipliers)
	return merged
}

func mergeEmbeddingPricing(base *adaptor.EmbeddingPricingConfig, overlay *adaptor.EmbeddingPricingConfig) *adaptor.EmbeddingPricingConfig {
	if overlay == nil {
		return base
	}
	if base == nil {
		return overlay.Clone()
	}
	merged := base.Clone()
	if overlay.TextTokenRatio != 0 {
		merged.TextTokenRatio = overlay.TextTokenRatio
	}
	if overlay.ImageTokenRatio != 0 {
		merged.ImageTokenRatio = overlay.ImageTokenRatio
	}
	if overlay.AudioTokenRatio != 0 {
		merged.AudioTokenRatio = overlay.AudioTokenRatio
	}
	if overlay.VideoTokenRatio != 0 {
		merged.VideoTokenRatio = overlay.VideoTokenRatio
	}
	if overlay.DocumentTokenRatio != 0 {
		merged.DocumentTokenRatio = overlay.DocumentTokenRatio
	}
	if overlay.UsdPerImage != 0 {
		merged.UsdPerImage = overlay.UsdPerImage
	}
	if overlay.UsdPerAudioSecond != 0 {
		merged.UsdPerAudioSecond = overlay.UsdPerAudioSecond
	}
	if overlay.UsdPerVideoFrame != 0 {
		merged.UsdPerVideoFrame = overlay.UsdPerVideoFrame
	}
	if overlay.UsdPerDocumentPage != 0 {
		merged.UsdPerDocumentPage = overlay.UsdPerDocumentPage
	}
	return merged
}

func mergePerCallPricing(base *adaptor.PerCallPricingConfig, overlay *adaptor.PerCallPricingConfig) *adaptor.PerCallPricingConfig {
	if overlay == nil {
		return base
	}
	if base == nil {
		return overlay.Clone()
	}
	merged := base.Clone()
	if overlay.UsdPerThousandCalls != 0 {
		merged.UsdPerThousandCalls = overlay.UsdPerThousandCalls
	}
	return merged
}

func mergeFloatMap(base map[string]float64, overlay map[string]float64) map[string]float64 {
	if len(overlay) == 0 {
		return base
	}
	merged := make(map[string]float64, len(base)+len(overlay))
	maps.Copy(merged, base)
	maps.Copy(merged, overlay)
	return merged
}

func mergeNestedFloatMap(base map[string]map[string]float64, overlay map[string]map[string]float64) map[string]map[string]float64 {
	if len(overlay) == 0 {
		return base
	}
	merged := make(map[string]map[string]float64, len(base)+len(overlay))
	for quality, sizes := range base {
		inner := make(map[string]float64, len(sizes))
		maps.Copy(inner, sizes)
		merged[quality] = inner
	}
	for quality, sizes := range overlay {
		inner := merged[quality]
		if inner == nil {
			inner = make(map[string]float64, len(sizes))
		}
		maps.Copy(inner, sizes)
		merged[quality] = inner
	}
	return merged
}
