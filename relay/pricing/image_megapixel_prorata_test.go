package pricing

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
)

// trueRateTestTariff mirrors Together AI's FLUX.2 [max] listing: $0.07 for
// every decimal megapixel (MP = width x height / 1,000,000), billed pro rata.
// Returns: a fresh tariff.
func trueRateTestTariff() *adaptor.ImageMegapixelPricing {
	return &adaptor.ImageMegapixelPricing{
		FirstMegapixelUsd:      0.07,
		AdditionalMegapixelUsd: 0.07,
		PixelsPerMegapixel:     1_000_000,
		Rounding:               adaptor.ImageMegapixelRoundingExact,
	}
}

// TestQuoteImageMegapixelsProRataOutput proves ProRataOutput lifts the one
// megapixel minimum for a host that bills a true rate, leaves larger outputs
// unchanged, and has no effect under ceil rounding or when unset.
func TestQuoteImageMegapixelsProRataOutput(t *testing.T) {
	t.Parallel()
	proRata := ImageMegapixelQuoteOptions{ProRataOutput: true}
	for _, tc := range []struct {
		name          string
		width, height int
		opts          ImageMegapixelQuoteOptions
		megapixels    float64
		unitUsd       float64
	}{
		// 0.262144 MP x $0.07.
		{"small_pro_rata", 512, 512, proRata, 0.262144, 0.01835008},
		// The default keeps the configured one megapixel minimum.
		{"small_minimum_by_default", 512, 512, ImageMegapixelQuoteOptions{}, 0.262144, 0.07},
		// 1.048576 MP x $0.07, identical with or without the option.
		{"one_mebipixel", 1024, 1024, proRata, 1.048576, 0.07340032},
		// 3.6864 MP x $0.07.
		{"large", 1920, 1920, proRata, 3.6864, 0.258048},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			quote, err := QuoteImageMegapixelsWithOptions(trueRateTestTariff(), tc.width, tc.height, nil, tc.opts)
			require.NoError(t, err)
			require.Equal(t, tc.megapixels, quote.OutputMegapixels)
			require.Equal(t, tc.unitUsd, quote.UnitUsd)
		})
	}

	ceil := trueRateTestTariff()
	ceil.Rounding = adaptor.ImageMegapixelRoundingCeil
	quote, err := QuoteImageMegapixelsWithOptions(ceil, 512, 512, nil, proRata)
	require.NoError(t, err)
	require.Equal(t, 0.07, quote.UnitUsd, "ceil rounding bills at least one started megapixel")
}
