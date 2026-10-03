package model

import (
	"github.com/Laisky/errors/v2"
	"math"
)

// validateAudioRates rejects non-finite, negative, or dimensionally inconsistent
// audio tariffs. Explicit units allow zero-cost speech without confusing it with
// absent configuration. It does not mutate its input.
func validateAudioRates(cfg *AudioPricingLocal) error {
	for name, value := range map[string]float64{
		"prompt_ratio": cfg.PromptRatio, "completion_ratio": cfg.CompletionRatio,
		"prompt_tokens_per_second": cfg.PromptTokensPerSecond, "completion_tokens_per_second": cfg.CompletionTokensPerSecond,
		"usd_per_second": cfg.UsdPerSecond, "input_price_usd": cfg.InputPriceUsd, "input_price_quantity": cfg.InputPriceQuantity,
		"minimum_billable_seconds": cfg.MinimumBillableSeconds, "billing_increment_seconds": cfg.BillingIncrementSeconds,
	} {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return errors.Errorf("audio %s must be finite and nonnegative", name)
		}
	}
	switch cfg.InputUnit {
	case "":
		if cfg.InputPriceUsd != 0 || cfg.InputPriceQuantity != 0 {
			return errors.New("audio direct price requires an explicit input_unit")
		}
	case "seconds", "characters", "utf8_bytes":
		if cfg.InputPriceQuantity <= 0 || math.IsNaN(cfg.InputPriceQuantity) || math.IsInf(cfg.InputPriceQuantity, 0) {
			return errors.New("audio input_price_quantity must be finite and positive")
		}
	default:
		return errors.New("audio input_unit must be characters, utf8_bytes, or seconds")
	}

	return nil
}
