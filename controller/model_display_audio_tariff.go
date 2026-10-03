package controller

import (
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/pricing"
)

// buildLegacyAudioTariffDisplay translates a scalar quota-unit override into
// the direct audio unit used by the relay. It preserves duration minimums and
// increments, returns a copy, and leaves token-only metadata unchanged.
func buildLegacyAudioTariffDisplay(base *AudioDisplayPricing, scalar float64) *AudioDisplayPricing {
	if base == nil {
		return nil
	}
	result := *base
	switch base.InputUnit {
	case "characters", "utf8_bytes":
		result.InputPriceQuantity = 1000000
		result.InputPriceUsd = scalar * result.InputPriceQuantity / ratio.QuotaPerUsd
	case "seconds":
		tokens := base.PromptTokensPerSecond
		if tokens <= 0 {
			tokens = pricing.DefaultAudioPromptTokensPerSecond
		}
		result.InputPriceQuantity = 3600
		result.InputPriceUsd = scalar * tokens * result.InputPriceQuantity / ratio.QuotaPerUsd
		result.UsdPerSecond = scalar * tokens / ratio.QuotaPerUsd
	}
	return &result
}
