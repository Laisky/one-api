package model

import (
	"github.com/stretchr/testify/require"
	"math"
	"testing"
)

// TestVideoInputImagePriceRoundTrip verifies storage normalizes and retains image fees.
func TestVideoInputImagePriceRoundTrip(t *testing.T) {
	channel := &Channel{}
	input := &VideoPricingLocal{PerSecondUsd: .08, InputImageUsd: .01, BaseResolution: "480p", ResolutionMultipliers: map[string]float64{"720p": 1.75}}
	require.NoError(t, channel.SetModelPriceConfigs(map[string]ModelConfigLocal{"grok-imagine-video-1.5": {Video: input}}))
	output := channel.GetModelPriceConfigs()["grok-imagine-video-1.5"].Video
	require.NotNil(t, output)
	require.Equal(t, input, output)
	for _, price := range []float64{-1, math.NaN(), math.Inf(1)} {
		_, err := normalizeVideoPricingLocal(&VideoPricingLocal{PerSecondUsd: .08, InputImageUsd: price})
		require.Error(t, err)
	}
}
