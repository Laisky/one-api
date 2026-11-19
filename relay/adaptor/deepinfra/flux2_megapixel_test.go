package deepinfra

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/pricing"
)

// TestFlux2MegapixelTariffs pins DeepInfra's published FLUX.2 [max] and [pro]
// megapixel tariffs (https://deepinfra.com/black-forest-labs/FLUX-2-max and
// .../FLUX-2-pro) and BFL's megapixel definition and rounding.
func TestFlux2MegapixelTariffs(t *testing.T) {
	t.Parallel()
	for name, want := range map[string][3]float64{
		"black-forest-labs/FLUX-2-max": {0.07, 0.03, 0.03},
		"black-forest-labs/FLUX-2-pro": {0.03, 0.015, 0.015},
	} {
		cfg := ModelRatios[name].Image
		require.NotNil(t, cfg, name)
		require.NotNil(t, cfg.Megapixel, name)
		tariff := cfg.Megapixel
		require.NoError(t, pricing.ValidateImageMegapixelPricing(tariff), name)
		require.Equal(t, want, [3]float64{tariff.FirstMegapixelUsd, tariff.AdditionalMegapixelUsd, tariff.ReferenceMegapixelUsd}, name)
		require.Equal(t, int64(1024*1024), tariff.PixelsPerMegapixel, name)
		require.Equal(t, adaptor.ImageMegapixelRoundingCeil, tariff.Rounding, name)
		require.Equal(t, 32, tariff.OutputDimensionStep, name)
		require.Equal(t, 16, tariff.ReferenceFallbackMegapixels, name)
		// The flat price is the 1 MP minimum shown in listings and must agree.
		require.Equal(t, tariff.FirstMegapixelUsd, cfg.PricePerImageUsd, name)
		require.Equal(t, "1024x1024", cfg.DefaultSize, name)
		require.Empty(t, cfg.SizeMultipliers, name)
	}
}

// TestOnlyFlux2MaxAndProOptIntoMegapixelPricing keeps the opt-in scoped to the
// models whose DeepInfra pages publish a megapixel tariff; every other image
// model keeps its flat or size-table price.
func TestOnlyFlux2MaxAndProOptIntoMegapixelPricing(t *testing.T) {
	t.Parallel()
	var optedIn []string
	for name, cfg := range ModelRatios {
		if cfg.Image != nil && cfg.Image.Megapixel != nil {
			optedIn = append(optedIn, name)
		}
	}
	require.ElementsMatch(t, []string{"black-forest-labs/FLUX-2-max", "black-forest-labs/FLUX-2-pro"}, optedIn)
}
