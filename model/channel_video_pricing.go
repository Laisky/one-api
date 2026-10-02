package model

import (
	"math"
	"strings"

	"github.com/Laisky/errors/v2"
)

// VideoPricingLocal represents channel-scoped video pricing metadata stored alongside model configs.
type VideoPricingLocal struct {
	InputImageUsd         float64            `json:"input_image_usd,omitempty"`
	PerSecondUsd          float64            `json:"per_second_usd,omitempty"`
	BaseResolution        string             `json:"base_resolution,omitempty"`
	ResolutionMultipliers map[string]float64 `json:"resolution_multipliers,omitempty"`
}

// normalizeVideoPricingLocal validates and copies channel video prices and resolution labels.
func normalizeVideoPricingLocal(cfg *VideoPricingLocal) (*VideoPricingLocal, error) {
	if cfg == nil {
		return nil, nil
	}

	if _, err := validateVideoPricingLocal(cfg, ""); err != nil {
		return nil, err
	}

	normalized := &VideoPricingLocal{
		PerSecondUsd:  cfg.PerSecondUsd,
		InputImageUsd: cfg.InputImageUsd,
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
			if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
				return nil, errors.Errorf("video resolution multiplier for %s must be positive", rawKey)
			}
			normalized.ResolutionMultipliers[key] = value
		}
	}

	return normalized, nil
}

// validateVideoPricingLocal checks model-scoped video rates and reports whether any price is set.
func validateVideoPricingLocal(cfg *VideoPricingLocal, modelName string) (bool, error) {
	if cfg == nil {
		return false, nil
	}
	if cfg.PerSecondUsd < 0 {
		return false, errors.Errorf("video per_second_usd cannot be negative for model %s", modelName)
	}
	if math.IsNaN(cfg.PerSecondUsd) || math.IsInf(cfg.PerSecondUsd, 0) ||
		cfg.InputImageUsd < 0 || math.IsNaN(cfg.InputImageUsd) || math.IsInf(cfg.InputImageUsd, 0) {
		return false, errors.Errorf("video rates must be finite and nonnegative for model %s", modelName)
	}
	for key, value := range cfg.ResolutionMultipliers {
		if strings.TrimSpace(key) == "" {
			return false, errors.Errorf("video resolution multiplier key cannot be empty for model %s", modelName)
		}
		if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return false, errors.Errorf("video resolution multiplier for %s must be positive (model %s)", key, modelName)
		}
	}
	return hasVideoPricingData(cfg), nil
}

// hasVideoPricingData reports whether a channel video pricing block carries any rate or multiplier.
func hasVideoPricingData(cfg *VideoPricingLocal) bool {
	if cfg == nil {
		return false
	}
	if cfg.PerSecondUsd > 0 || cfg.InputImageUsd > 0 {
		return true
	}
	return len(cfg.ResolutionMultipliers) > 0
}
