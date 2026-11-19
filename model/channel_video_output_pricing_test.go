package model

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestVideoOutputTokenRatioRoundTrip verifies channel storage keeps the realtime
// video output ratio, accepts it as a model's or a time-window overlay's only
// pricing field, rejects invalid values, and leaves configs without it
// byte-identical. It accepts t and returns no values.
func TestVideoOutputTokenRatioRoundTrip(t *testing.T) {
	var authored map[string]ModelConfigLocal
	require.NoError(t, json.Unmarshal([]byte(`{
		"gemini-3.8-live": {"ratio": 0.375, "completion_ratio": 6, "video": {"output_token_ratio": 1.3333333333333333},
			"time_windows": [{"timezone": "UTC", "ranges": [{"start": "00:00", "end": "06:00"}],
				"overlay": {"video": {"output_token_ratio": 0.6666666666666666}}}]},
		"video-only": {"video": {"output_token_ratio": 2}}}`), &authored))
	channel := &Channel{}
	require.NoError(t, channel.SetModelPriceConfigs(authored))
	stored := channel.GetModelPriceConfigs()
	require.Equal(t, 1.3333333333333333, stored["gemini-3.8-live"].Video.OutputTokenRatio)
	require.Equal(t, 0.6666666666666666, stored["gemini-3.8-live"].TimeWindows[0].Overlay.Video.OutputTokenRatio)
	require.Equal(t, 2.0, stored["video-only"].Video.OutputTokenRatio)

	for _, invalid := range []float64{-1, math.NaN(), math.Inf(1)} {
		_, err := validateVideoPricingLocal(&VideoPricingLocal{OutputTokenRatio: invalid}, "m")
		require.Error(t, err, "%v", invalid)
	}

	// A config written before the field existed keeps its exact stored form.
	legacy := map[string]ModelConfigLocal{"live": {Ratio: 0.375, CompletionRatio: 6,
		Audio: &AudioPricingLocal{PromptRatio: 4, CompletionRatio: 4}, Image: &ImagePricingLocal{PromptRatio: 1.25}}}
	require.NoError(t, channel.SetModelPriceConfigs(legacy))
	require.NotContains(t, *channel.ModelConfigs, "output_token_ratio")
	require.NotContains(t, *channel.ModelConfigs, `"video"`)
}
