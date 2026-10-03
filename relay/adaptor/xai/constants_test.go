package xai

import (
	"testing"

	"github.com/stretchr/testify/require"

	ratio "github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/pricing"
)

// TestCurrentModelMetadata verifies the current official Grok catalog entries
// and their published context-tier pricing. Parameters: t is the test context.
// Returns: no value; failures identify stale model metadata or prices.
func TestCurrentModelMetadata(t *testing.T) {
	t.Parallel()

	flagship, ok := ModelRatios["grok-4.6"]
	require.True(t, ok)
	require.Equal(t, int32(500_000), flagship.ContextLength)
	require.InDelta(t, 2.0*ratio.MilliTokensUsd, flagship.Ratio, 1e-9)
	require.Len(t, flagship.Tiers, 1)
	require.Equal(t, 200_000, flagship.Tiers[0].InputTokenThreshold)
	require.InDelta(t, 4.0*ratio.MilliTokensUsd, flagship.Tiers[0].Ratio, 1e-9)
	require.InDelta(t, 1.0*ratio.MilliTokensUsd, flagship.Tiers[0].CachedInputRatio, 1e-9)

	image, ok := ModelRatios["grok-imagine-image-2.0"]
	require.True(t, ok)
	require.NotNil(t, image.Image)
	require.InDelta(t, 0.04, image.Image.PricePerImageUsd, 1e-12)
	require.Equal(t, 1.5, image.Image.QualitySizeMultipliers["low"]["2048x2048"])
	require.Equal(t, 2.0, image.Image.QualitySizeMultipliers["medium"]["2048x2048"])

	video, ok := ModelRatios["grok-imagine-video-1.5"]
	require.True(t, ok)
	require.NotNil(t, video.Video)
	require.InDelta(t, 3.125, video.Video.ResolutionMultipliers["1080p"], 1e-12)
}

// TestLongContextPricingBoundaries verifies xAI's 200K input-token surcharge.
// Parameters: t is the test context. Returns: no value; failures identify a
// boundary or tier-resolution regression.
func TestLongContextPricingBoundaries(t *testing.T) {
	t.Parallel()

	config := ModelRatios["grok-4.6"]
	short := pricing.ResolveEffectivePricingFromConfig(199_999, config)
	long := pricing.ResolveEffectivePricingFromConfig(200_000, config)

	require.Equal(t, 0, short.AppliedTierThreshold)
	require.Equal(t, 200_000, long.AppliedTierThreshold)
	require.InDelta(t, 2.0*ratio.MilliTokensUsd, short.InputRatio, 1e-9)
	require.InDelta(t, 4.0*ratio.MilliTokensUsd, long.InputRatio, 1e-9)
	require.InDelta(t, 6.0*ratio.MilliTokensUsd, short.OutputRatio, 1e-9)
	require.InDelta(t, 12.0*ratio.MilliTokensUsd, long.OutputRatio, 1e-9)
}
