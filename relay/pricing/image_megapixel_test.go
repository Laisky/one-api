package pricing

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
)

// flux2MaxTestTariff mirrors DeepInfra's FLUX.2 [max] tariff for unit tests.
// Returns: a fresh tariff.
func flux2MaxTestTariff() *adaptor.ImageMegapixelPricing {
	return &adaptor.ImageMegapixelPricing{
		FirstMegapixelUsd:           0.07,
		AdditionalMegapixelUsd:      0.03,
		ReferenceMegapixelUsd:       0.03,
		ReferenceFallbackMegapixels: 16,
	}
}

// TestQuoteImageMegapixelsCeilRounding pins the per-started-megapixel reading
// with BFL's 1,048,576-pixel megapixel, applied separately to the output and to
// each reference image.
func TestQuoteImageMegapixelsCeilRounding(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                     string
		width, height            int
		references               []ImageDimensions
		wantOutput, wantRefs     float64
		wantUnit                 float64
		wantRefCount, wantUnknow int
	}{
		{name: "exactly_one_megapixel", width: 1024, height: 1024, wantOutput: 1, wantUnit: 0.07},
		{name: "small_output_pays_first_megapixel", width: 256, height: 256, wantOutput: 1, wantUnit: 0.07},
		{name: "one_pixel_over_starts_second_megapixel", width: 1024, height: 1025, wantOutput: 2, wantUnit: 0.10},
		// 1,000,000-pixel megapixels would bill 1024x1024 as 2 MP; BFL's does not.
		{name: "decimal_megapixel_not_used", width: 1000, height: 1048, wantOutput: 1, wantUnit: 0.07},
		{name: "max_size", width: 1440, height: 1440, wantOutput: 2, wantUnit: 0.10},
		{name: "four_megapixels", width: 2048, height: 2048, wantOutput: 4, wantUnit: 0.16},
		{
			name: "references_round_separately", width: 1024, height: 1024,
			// 1.43 MP -> 2 and 0.25 MP -> 1; summed first they would be 2.
			references:   []ImageDimensions{{Width: 1500, Height: 1000, Known: true}, {Width: 512, Height: 512, Known: true}},
			wantOutput:   1,
			wantRefs:     3,
			wantUnit:     0.16,
			wantRefCount: 2,
		},
		{
			name: "unknown_reference_uses_fallback", width: 1024, height: 1024,
			references:   []ImageDimensions{{Known: false}, {Width: 10, Height: 0, Known: true}},
			wantOutput:   1,
			wantRefs:     32,
			wantUnit:     1.03,
			wantRefCount: 2, wantUnknow: 2,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			quote, err := QuoteImageMegapixels(flux2MaxTestTariff(), tc.width, tc.height, tc.references)
			require.NoError(t, err)
			require.Equal(t, tc.wantOutput, quote.OutputMegapixels)
			require.Equal(t, tc.wantRefs, quote.ReferenceMegapixels)
			require.Equal(t, tc.wantRefCount, quote.References)
			require.Equal(t, tc.wantUnknow, quote.UnknownReferences)
			// UnitUsd is the nearest float64 of the exact decimal sum, so it equals
			// the literal price and never accumulates float error.
			require.Equal(t, tc.wantUnit, quote.UnitUsd)
		})
	}
}

// TestQuoteImageMegapixelsOptions covers the exact rounding rule, a custom
// megapixel definition, and output grid snapping.
func TestQuoteImageMegapixelsOptions(t *testing.T) {
	t.Parallel()
	exact := flux2MaxTestTariff()
	exact.Rounding = adaptor.ImageMegapixelRoundingExact
	// 1536x1024 is 1.5 MP: $0.07 + 0.5 x $0.03 + 0.25 MP reference x $0.03.
	quote, err := QuoteImageMegapixels(exact, 1536, 1024, []ImageDimensions{{Width: 512, Height: 512, Known: true}})
	require.NoError(t, err)
	require.Equal(t, 1.5, quote.OutputMegapixels)
	require.Equal(t, 0.25, quote.ReferenceMegapixels)
	require.Equal(t, 0.0925, quote.UnitUsd)
	// The first megapixel stays a minimum under exact rounding.
	quote, err = QuoteImageMegapixels(exact, 512, 512, nil)
	require.NoError(t, err)
	require.Equal(t, 0.07, quote.UnitUsd)

	decimal := flux2MaxTestTariff()
	decimal.PixelsPerMegapixel = 1_000_000
	quote, err = QuoteImageMegapixels(decimal, 1024, 1024, nil)
	require.NoError(t, err)
	require.Equal(t, 2.0, quote.OutputMegapixels)

	snapped := flux2MaxTestTariff()
	snapped.OutputDimensionStep = 32
	// 1000x1048 snaps up to 1024x1056 = 1,081,344 px, which starts a second MP.
	quote, err = QuoteImageMegapixels(snapped, 1000, 1048, nil)
	require.NoError(t, err)
	require.Equal(t, 2.0, quote.OutputMegapixels)
	require.Equal(t, 0.10, quote.UnitUsd)
}

// TestQuoteImageMegapixelsRejectsUnbillableInput refuses tariffs and sizes
// that cannot be billed rather than charging a default.
func TestQuoteImageMegapixelsRejectsUnbillableInput(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*adaptor.ImageMegapixelPricing){
		"unpriced_first":       func(cfg *adaptor.ImageMegapixelPricing) { cfg.FirstMegapixelUsd = 0 },
		"negative_additional":  func(cfg *adaptor.ImageMegapixelPricing) { cfg.AdditionalMegapixelUsd = -0.01 },
		"nan_reference":        func(cfg *adaptor.ImageMegapixelPricing) { cfg.ReferenceMegapixelUsd = math.NaN() },
		"unknown_rounding":     func(cfg *adaptor.ImageMegapixelPricing) { cfg.Rounding = "floor" },
		"missing_fallback":     func(cfg *adaptor.ImageMegapixelPricing) { cfg.ReferenceFallbackMegapixels = 0 },
		"negative_pixels":      func(cfg *adaptor.ImageMegapixelPricing) { cfg.PixelsPerMegapixel = -1 },
		"negative_output_step": func(cfg *adaptor.ImageMegapixelPricing) { cfg.OutputDimensionStep = -32 },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cfg := flux2MaxTestTariff()
			mutate(cfg)
			require.Error(t, ValidateImageMegapixelPricing(cfg))
			_, err := QuoteImageMegapixels(cfg, 1024, 1024, nil)
			require.Error(t, err)
		})
	}
	require.Error(t, ValidateImageMegapixelPricing(nil))
	for _, size := range [][2]int{{0, 1024}, {1024, -1}, {maxImageMegapixelSide + 1, 1}} {
		_, err := QuoteImageMegapixels(flux2MaxTestTariff(), size[0], size[1], nil)
		require.Error(t, err, "size %v", size)
	}
	// A tariff without a reference price needs no fallback.
	noReference := &adaptor.ImageMegapixelPricing{FirstMegapixelUsd: 0.05}
	quote, err := QuoteImageMegapixels(noReference, 1024, 1024, []ImageDimensions{{Known: false}})
	require.NoError(t, err)
	require.Equal(t, 0.05, quote.UnitUsd)
}

// TestImagePricingConfigMegapixelCloneAndData verifies the opt-in block is
// deep-copied and counted as pricing data.
func TestImagePricingConfigMegapixelCloneAndData(t *testing.T) {
	t.Parallel()
	original := &adaptor.ImagePricingConfig{Megapixel: flux2MaxTestTariff()}
	require.True(t, original.HasData())
	require.True(t, hasImageTariff(original))
	clone := original.Clone()
	clone.Megapixel.FirstMegapixelUsd = 1
	require.Equal(t, 0.07, original.Megapixel.FirstMegapixelUsd, "clone must not alias the tariff")
	require.False(t, (&adaptor.ImagePricingConfig{}).HasData())
}

// TestResolveImagePricingMegapixelLayers verifies the provider megapixel
// tariff survives request-default overrides but never leaks under a complete
// operator image tariff.
func TestResolveImagePricingMegapixelLayers(t *testing.T) {
	const modelName = "image-megapixel-layer-model"
	setTestGlobalModelConfigs(t, map[string]adaptor.ModelConfig{})
	provider := &MockAdaptor{name: "provider", pricing: map[string]adaptor.ModelConfig{
		modelName: {Image: &adaptor.ImagePricingConfig{PricePerImageUsd: 0.07, DefaultSize: "1024x1024", Megapixel: flux2MaxTestTariff()}},
	}}

	cfg, ok := ResolveImagePricing(modelName, nil, provider, time.Now())
	require.True(t, ok)
	require.NotNil(t, cfg.Megapixel)
	require.Equal(t, 0.03, cfg.Megapixel.ReferenceMegapixelUsd)

	defaultsOnly := map[string]model.ModelConfigLocal{modelName: {Image: &model.ImagePricingLocal{DefaultSize: "512x512"}}}
	cfg, ok = ResolveImagePricing(modelName, defaultsOnly, provider, time.Now())
	require.True(t, ok)
	require.NotNil(t, cfg.Megapixel, "a request-default override keeps the provider tariff")
	require.Equal(t, "512x512", cfg.DefaultSize)

	flatOverride := map[string]model.ModelConfigLocal{modelName: {Image: &model.ImagePricingLocal{PricePerImageUsd: 0.2}}}
	cfg, ok = ResolveImagePricing(modelName, flatOverride, provider, time.Now())
	require.True(t, ok)
	require.Nil(t, cfg.Megapixel, "an operator flat tariff is a complete override")
	require.Equal(t, 0.2, cfg.PricePerImageUsd)
	require.Equal(t, "1024x1024", cfg.DefaultSize)
}
