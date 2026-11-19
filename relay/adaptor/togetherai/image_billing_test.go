package togetherai

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/pricing"
)

// TestTogetherImageTariffsMatchListings pins Together AI's published image
// prices (https://www.together.ai/pricing, retrieved 2026-11-18): per-megapixel
// models carry a pro-rata decimal-megapixel tariff with their default steps,
// and per-image FLUX.2 models carry their lowest-resolution price as a floor.
func TestTogetherImageTariffsMatchListings(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		model        string
		listedUsd    float64
		perMegapixel bool
		defaultSteps int
	}{
		{"black-forest-labs/FLUX.2-max", 0.07, true, 50},
		{"black-forest-labs/FLUX.1-kontext-pro", 0.04, true, 28},
		{"black-forest-labs/FLUX.1-kontext-max", 0.08, true, 28},
		{"black-forest-labs/FLUX.1.1-pro", 0.04, true, 0},
		{"black-forest-labs/FLUX.2-dev", 0.0154, false, 0},
		{"black-forest-labs/FLUX.2-flex", 0.03, false, 0},
		{"black-forest-labs/FLUX.2-pro", 0.03, false, 0},
	} {
		t.Run(tc.model, func(t *testing.T) {
			t.Parallel()
			image := ModelRatios[tc.model].Image
			require.NotNil(t, image)
			require.NotNil(t, image.Megapixel)
			require.NoError(t, pricing.ValidateImageMegapixelPricing(image.Megapixel))
			require.Equal(t, tc.listedUsd, image.Megapixel.FirstMegapixelUsd)
			require.Equal(t, tc.listedUsd, image.Megapixel.AdditionalMegapixelUsd)
			require.EqualValues(t, 1_000_000, image.Megapixel.PixelsPerMegapixel)
			require.Equal(t, adaptor.ImageMegapixelRoundingExact, image.Megapixel.Rounding)
			require.Empty(t, image.SizeMultipliers, "a size table would multiply the megapixel price again")

			rule := ImageBillingRuleFor(tc.model)
			require.Equal(t, tc.perMegapixel, rule.ProRataOutput)
			require.Equal(t, tc.defaultSteps, rule.DefaultSteps)

			// 512x512 costs 0.262144 MP of a true rate, but never less than
			// the listed lowest-resolution price of a per-image model.
			quote, err := pricing.QuoteImageMegapixelsWithOptions(image.Megapixel, 512, 512, nil,
				pricing.ImageMegapixelQuoteOptions{ProRataOutput: rule.ProRataOutput})
			require.NoError(t, err)
			small := tc.listedUsd
			if tc.perMegapixel {
				small = tc.listedUsd * 0.262144
			}
			require.InDelta(t, small, quote.UnitUsd, 1e-12)
		})
	}
}

// TestTogetherImageCatalogPricesForwardedDimensions guards the whole Together
// catalog: Together renders the width and height it receives, so a size table
// may only price sizes Together documents and must never scale a price below
// its listing, and PricePerImageUsd must equal the tariff's default-size price.
func TestTogetherImageCatalogPricesForwardedDimensions(t *testing.T) {
	t.Parallel()
	for name, cfg := range ModelRatios {
		image := cfg.Image
		if image == nil {
			continue
		}
		for size, multiplier := range image.SizeMultipliers {
			require.GreaterOrEqual(t, multiplier, 1.0, "%s %s would bill below its listed price", name, size)
		}
		if image.Megapixel == nil {
			continue
		}
		require.Empty(t, image.SizeMultipliers, "%s mixes a size table with a megapixel tariff", name)
		width, height, ok := strings.Cut(image.DefaultSize, "x")
		require.True(t, ok, "%s default size %q", name, image.DefaultSize)
		w, err := strconv.Atoi(width)
		require.NoError(t, err)
		h, err := strconv.Atoi(height)
		require.NoError(t, err)
		quote, err := pricing.QuoteImageMegapixelsWithOptions(image.Megapixel, w, h, nil,
			pricing.ImageMegapixelQuoteOptions{ProRataOutput: ImageBillingRuleFor(name).ProRataOutput})
		require.NoError(t, err, name)
		require.InDelta(t, image.PricePerImageUsd, quote.UnitUsd, 1e-12, "%s listing price differs from its default-size charge", name)
	}
	for name := range imageBillingRules {
		require.NotNil(t, ModelRatios[name].Image, "%s has a billing rule but no image tariff", name)
		require.NotNil(t, ModelRatios[name].Image.Megapixel, "%s is billed per megapixel", name)
	}
}

// TestGemini3ProImageSizeTiers pins Together's documented Gemini 3 Pro Image
// sizes: 1K and 2K at the listed $0.134 and 4K at about $0.24.
func TestGemini3ProImageSizeTiers(t *testing.T) {
	t.Parallel()
	image := ModelRatios["google/gemini-3-pro-image"].Image
	require.NotNil(t, image)
	require.Equal(t, "1024x1024", image.DefaultSize)
	for size, usd := range map[string]float64{"1024x1024": 0.134, "1376x768": 0.134, "2752x1536": 0.134, "3168x1344": 0.134, "5504x3072": 0.24, "6336x2688": 0.24, "4096x4096": 0.24} {
		require.InDelta(t, usd, image.PricePerImageUsd*image.SizeMultipliers[size], 1e-12, size)
	}
}
