package fireworks_test

import (
	"github.com/Laisky/one-api/relay/adaptor/fireworks"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/stretchr/testify/require"
	"testing"
)

// TestOctoberFireworksStandardTariffs verifies ten published Standard tariffs
// through public adaptor methods using t. It returns nothing.
func TestOctoberFireworksStandardTariffs(t *testing.T) {
	t.Parallel()
	for id, prices := range map[string][3]float64{
		"ember-1": {3, .3, 15}, "kimi-k3": {3, .3, 15},
		"deepseek-v4p1-flash": {.3, .006, 1.2}, "glm-5p3-flash": {.15, .03, .5},
		"glm-5p3": {1.4, .26, 4.4}, "qwen3p8-2p4t-a95b": {2, .25, 6},
		"minimax-m3": {.3, .06, 1.2}, "gpt-oss-120b": {.15, .015, .6},
		"nemotron-lightning-3p5-30b-a3b": {.05, .01, .2}, "nemotron-3-ultra-nvfp4": {.6, .12, 2.4},
	} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			name := "accounts/fireworks/models/" + id
			a := &fireworks.Adaptor{}
			cfg, exists := a.GetDefaultModelPricing()[name]
			require.True(t, exists)
			require.Contains(t, a.GetModelList(), name)
			require.InDelta(t, prices[0], a.GetModelRatio(name)/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, prices[1], cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, prices[2], a.GetModelRatio(name)*a.GetCompletionRatio(name)/ratio.MilliTokensUsd, 1e-12)
			require.Empty(t, cfg.TimeWindows, "native provider dayparts are not Fireworks tariffs")
		})
	}
}

// TestOctoberFireworksLegacySnapshotPrices prevents an updated V4.1 rate from
// repricing other dated DeepSeek releases. It accepts t and returns nothing.
func TestOctoberFireworksLegacySnapshotPrices(t *testing.T) {
	t.Parallel()
	a := &fireworks.Adaptor{}
	for id, prices := range map[string][3]float64{
		"deepseek-v4-flash-0731": {.22, .007, .66},
		"deepseek-v4-pro-0813":   {1.32, .044, 3.96},
	} {
		name := "accounts/fireworks/models/" + id
		cfg, ok := a.GetDefaultModelPricing()[name]
		require.True(t, ok)
		require.InDelta(t, prices[0], cfg.Ratio/ratio.MilliTokensUsd, 1e-12)
		require.InDelta(t, prices[1], cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
		require.InDelta(t, prices[2], cfg.Ratio*cfg.CompletionRatio/ratio.MilliTokensUsd, 1e-12)
	}
}
