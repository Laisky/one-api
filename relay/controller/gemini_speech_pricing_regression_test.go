package controller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/gemini"
	"github.com/Laisky/one-api/relay/adaptor/gemini/tts"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/Laisky/one-api/relay/quota"
)

// TestGeminiSpeechEffectiveAudioOverrides covers base, empty, and window-only audio tariffs.
// Parameters: t is the test handle. Returns: none.
func TestGeminiSpeechEffectiveAudioOverrides(t *testing.T) {
	t.Parallel()
	const name = "gemini-3.8-flash-tts"
	before := time.Date(2026, 9, 30, 23, 59, 59, 0, time.UTC)
	after := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	window := []dbmodel.TimeWindowLocal{{
		TimeZone: "UTC", DateFrom: "2026-10-01",
		Ranges:  []dbmodel.ClockRangeLocal{{Start: "00:00", End: "00:00"}},
		Overlay: dbmodel.ModelConfigLocal{Audio: &dbmodel.AudioPricingLocal{PromptRatio: 1, CompletionRatio: 6}},
	}}
	for _, tc := range []struct {
		name     string
		audio    *dbmodel.AudioPricingLocal
		windows  []dbmodel.TimeWindowLocal
		at       time.Time
		explicit bool
		charge   int64
		reserve  int64
	}{
		{"base_audio", &dbmodel.AudioPricingLocal{PromptRatio: 1, CompletionRatio: 6}, nil, before, true, 40, 16504},
		{"audio_prompt_factor", &dbmodel.AudioPricingLocal{PromptRatio: 2, CompletionRatio: 3}, nil, before, true, 40, 16504},
		{"scalar_only", nil, nil, before, false, 48, 16544},
		{"empty_audio_is_not_an_override", &dbmodel.AudioPricingLocal{}, nil, before, false, 48, 16544},
		{"before_audio_window", nil, window, before, false, 48, 16544},
		{"at_audio_window", nil, window, after, true, 40, 16504},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := dbmodel.ModelConfigLocal{Ratio: 2, CompletionRatio: 8, CachedInputRatio: -1, Audio: tc.audio, TimeWindows: tc.windows}
			input := quota.ComputeInput{ModelName: name, GroupRatio: 1, PricingAdaptor: &gemini.Adaptor{}, RequestTime: tc.at,
				ChannelModelConfigs: map[string]dbmodel.ModelConfigLocal{name: cfg}, ChannelModelRatio: map[string]float64{name: 2}}
			input.ModelRatio = pricing.ResolveModelRatioAt(name, input.ChannelModelConfigs, input.ChannelModelRatio, input.PricingAdaptor, tc.at)
			prices, err := newGeminiSpeechPrices(input)
			require.NoError(t, err)
			require.Equal(t, tc.explicit, prices.explicitAudio)
			charge, err := prices.charge(tts.Receipt{PromptTokens: 12, CachedTokens: 4, OutputTokens: 2})
			require.NoError(t, err)
			require.Equal(t, tc.charge, charge)
			hold, err := prices.reserve(10)
			require.NoError(t, err)
			require.Equal(t, tc.reserve, hold)
		})
	}
}
