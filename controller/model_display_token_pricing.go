package controller

import (
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
)

// applyLocalTokenDisplayOverride aligns local token schedules/cache tariffs with
// billing without changing native media/unit tariff display. Parameters: local
// and converted are the saved tariff; prices are current display values;
// hasUnit/hasMedia describe the selected display unit, and convert quotes a ratio.
// Returns: whether to discard an inherited schedule and the effective cache price.
func applyLocalTokenDisplayOverride(local model.ModelConfigLocal, converted adaptor.ModelConfig, inputPrice, cachedPrice float64, hasUnit, hasMedia bool, convert func(float64) float64) (bool, float64) {
	if hasUnit || hasMedia {
		// Preserve the existing explicit native tariff schedule boundary.
		explicitNative := converted.PerCall != nil || converted.PerPage != nil ||
			(converted.Audio != nil && converted.Audio.HasData()) || local.Ratio != 0
		return hasUnit && explicitNative, cachedPrice
	}
	// Token billing selects the local config wholesale. Missing schedules and
	// cache-hit prices are not inherited from the provider's defaults.
	cachedPrice = inputPrice
	if local.CachedInputRatio < 0 {
		cachedPrice = 0
	} else if local.CachedInputRatio > 0 {
		cachedPrice = convert(local.CachedInputRatio)
	}
	return true, cachedPrice
}
