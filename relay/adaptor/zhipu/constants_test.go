package zhipu

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/pricing"
)

// TestCurrentTextModelMetadata verifies current official model identifiers and
// exact context, output, and pricing metadata published by BigModel.
func TestCurrentTextModelMetadata(t *testing.T) {
	t.Parallel()

	glm52, ok := ModelRatios["glm-5.2"]
	require.True(t, ok)
	require.Equal(t, int32(1_000_000), glm52.ContextLength)
	require.Equal(t, int32(131_072), glm52.MaxOutputTokens)
	require.InDelta(t, 8*ratio.MilliTokensRmb, glm52.Ratio, 1e-12)
	require.InDelta(t, 28.0/8.0, glm52.CompletionRatio, 1e-12)
	require.InDelta(t, 2*ratio.MilliTokensRmb, glm52.CachedInputRatio, 1e-12)

	glm51, ok := ModelRatios["glm-5.1"]
	require.True(t, ok)
	require.Equal(t, int32(200_000), glm51.ContextLength)
	require.Equal(t, int32(131_072), glm51.MaxOutputTokens)
	require.Len(t, glm51.Tiers, 1)
	require.Equal(t, 32_000, glm51.Tiers[0].InputTokenThreshold)

	airSnapshot, ok := ModelRatios["glm-4-air-250414"]
	require.True(t, ok)
	require.Equal(t, int32(131_072), airSnapshot.ContextLength)
	require.Equal(t, int32(16_384), airSnapshot.MaxOutputTokens)
}

// TestCurrentVisionModelMetadata verifies exact limits and pricing thresholds
// for the current flagship visual models.
func TestCurrentVisionModelMetadata(t *testing.T) {
	t.Parallel()

	glm5v, ok := ModelRatios["glm-5v-turbo"]
	require.True(t, ok)
	require.Equal(t, int32(200_000), glm5v.ContextLength)
	require.Equal(t, int32(131_072), glm5v.MaxOutputTokens)
	require.Len(t, glm5v.Tiers, 1)
	require.Equal(t, 32_000, glm5v.Tiers[0].InputTokenThreshold)

	glm4v, ok := ModelRatios["glm-4v-plus-0111"]
	require.True(t, ok)
	require.Equal(t, int32(8_192), glm4v.MaxOutputTokens)
}

// TestZhipuTieredPricingResolution verifies BigModel's 32K input and 0.2K
// output boundaries without conflating thousands of tokens with raw tokens.
func TestZhipuTieredPricingResolution(t *testing.T) {
	t.Parallel()

	glm47 := ModelRatios["glm-4.7"]
	short := pricing.ResolveEffectivePricingForUsageFromConfig(31_999, 199, glm47)
	require.InDelta(t, 2*ratio.MilliTokensRmb, short.InputRatio, 1e-12)
	require.InDelta(t, 8*ratio.MilliTokensRmb, short.OutputRatio, 1e-12)

	longOutput := pricing.ResolveEffectivePricingForUsageFromConfig(31_999, 200, glm47)
	require.InDelta(t, 3*ratio.MilliTokensRmb, longOutput.InputRatio, 1e-12)
	require.InDelta(t, 14*ratio.MilliTokensRmb, longOutput.OutputRatio, 1e-12)
	require.Equal(t, 200, longOutput.AppliedOutputTierThreshold)

	longInput := pricing.ResolveEffectivePricingForUsageFromConfig(32_000, 199, glm47)
	require.InDelta(t, 4*ratio.MilliTokensRmb, longInput.InputRatio, 1e-12)
	require.InDelta(t, 16*ratio.MilliTokensRmb, longInput.OutputRatio, 1e-12)
	require.Equal(t, 32_000, longInput.AppliedTierThreshold)
}
