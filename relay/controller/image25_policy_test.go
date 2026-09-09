package controller

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
)

// TestGPTImage25ReservationPolicy rejects missing or unrepresentable reserves.
// Parameters: t runs the table. Returns: none; other model policies stay unchanged.
func TestGPTImage25ReservationPolicy(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name               string
		price, tier, group float64
		count              int
	}{
		{"missing", 0, 1, 1, 1},
		{"negative", -1, 1, 1, 1},
		{"nan", math.NaN(), 1, 1, 1},
		{"infinite", math.Inf(1), 1, 1, 1},
		{"zero_tier", 0.1, 0, 1, 1},
		{"zero_group", 0.1, 1, 0, 1},
		{"invalid_count", 0.1, 1, 1, 0},
		{"overflow", math.MaxFloat64, 1, 1, 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Error(t, validateGPTImage25Billing("gpt-image-2.5-sunburst", tc.price, tc.tier, tc.group, tc.count))
		})
	}
	require.NoError(t, validateGPTImage25Billing("gpt-image-2.5-flare", 0.1, 1, 1.5, 10))
	require.NoError(t, validateGPTImage25Billing("dall-e-3", 0, 0, 0, 0))
}

// TestGPTImage25SignedSettlement protects refunds of unused reservations.
// Parameters: t runs debit/refund cases. Returns: none; legacy negative deltas stay clamped.
func TestGPTImage25SignedSettlement(t *testing.T) {
	t.Parallel()
	require.Equal(t, int64(-32_500), imagePostConsumeDelta("gpt-image-2.5-sunburst", 17_500, 50_000))
	require.Zero(t, imagePostConsumeDelta("gpt-image-2.5-flare", 50_000, 50_000))
	require.Equal(t, int64(100_000), imagePostConsumeDelta("gpt-image-2.5-flare", 150_000, 50_000))
	require.Zero(t, imagePostConsumeDelta("dall-e-3", 17_500, 50_000))
}

// TestGPTImage25PriceOnlyOverrideKeepsRequestDefaults checks that configuring a
// reserve does not regress Image API defaults. Parameters: t runs the assertions.
// Returns: none; the original channel configuration must remain unmodified.
func TestGPTImage25PriceOnlyOverrideKeepsRequestDefaults(t *testing.T) {
	t.Parallel()
	input := &adaptor.ImagePricingConfig{PricePerImageUsd: 0.1}
	got := completeGPTImage25Defaults("gpt-image-2.5-sunburst", input)
	require.Equal(t, 0.1, got.PricePerImageUsd)
	require.Equal(t, "auto", got.DefaultSize)
	require.Equal(t, "auto", got.DefaultQuality)
	require.Equal(t, 32000, got.PromptTokenLimit)
	require.Equal(t, 1, got.MinImages)
	require.Equal(t, 10, got.MaxImages)
	require.Empty(t, input.DefaultQuality)
	require.Zero(t, input.MaxImages)
	input.DefaultQuality = "high"
	input.MaxImages = 2
	got = completeGPTImage25Defaults("gpt-image-2.5-flare", input)
	require.Equal(t, "high", got.DefaultQuality)
	require.Equal(t, 2, got.MaxImages)
	require.Same(t, input, completeGPTImage25Defaults("dall-e-3", input))
}
