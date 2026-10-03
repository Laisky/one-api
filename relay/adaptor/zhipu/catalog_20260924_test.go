package zhipu

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
)

// TestPublishedCNYRates checks independently quoted model, audio, clone, and
// search tariffs through the public adaptor rather than rounded RMB quota units.
func TestPublishedCNYRates(t *testing.T) {
	t.Parallel()
	a := &Adaptor{}
	cfg := a.GetDefaultModelPricing()["glm-5.3-flashx"]
	require.Contains(t, a.GetModelList(), "glm-5.3-flashx")
	require.InDelta(t, 2*ratio.MilliTokensRmb, cfg.Ratio, 1e-12)
	require.InDelta(t, 7*ratio.MilliTokensRmb, cfg.Ratio*cfg.CompletionRatio, 1e-12)
	require.InDelta(t, .57*ratio.MilliTokensRmb, cfg.CachedInputRatio, 1e-12)
	require.Empty(t, cfg.TimeWindows)
	tts := a.GetDefaultModelPricing()["glm-tts"]
	require.Equal(t, "characters", tts.Audio.InputUnit)
	require.InDelta(t, 2.0/7, tts.Audio.InputPriceUsd, 1e-12)
	require.EqualValues(t, 10000, tts.Audio.InputPriceQuantity)
	require.Zero(t, tts.CompletionRatio)
	asr := a.GetDefaultModelPricing()["glm-asr-2512"]
	require.InDelta(t, 16*ratio.MilliTokensRmb, asr.Ratio, 1e-12)
	require.Zero(t, asr.CompletionRatio)
	clone := a.GetDefaultModelPricing()["glm-tts-clone"]
	require.InDelta(t, 6.0/7*ratio.QuotaPerUsd, clone.Ratio, 1e-9)
	require.InDelta(t, 6.0/7*1000, clone.PerCall.UsdPerThousandCalls, 1e-9)
	for id, cny := range map[string]float64{"search_std": .01, "search_pro": .03, "search_pro_sogou": .05, "search_pro_quark": .05} {
		require.InDelta(t, cny/7, a.DefaultToolingConfig().Pricing[id].UsdPerCall, 1e-12)
	}
}
