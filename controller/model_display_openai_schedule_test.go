package controller

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/pricing"
)

// TestGetModelsDisplayPublishesEffectiveOpenAISchedule verifies a response
// rendered before the cutoff already publishes each window's complete tariff
// (effective_pricing) with the base and long-context rates billing applies
// once the window matches, while the top-level quote is still the promotional
// one and saved overrides carry no schedule.
func TestGetModelsDisplayPublishesEffectiveOpenAISchedule(t *testing.T) {
	setupModelsDisplayTestEnv(t)
	const names = "gpt-5.6,gpt-5.6-sol"
	m := ratio.MilliTokensUsd
	native := createDisplayChannel(t, "native-sol-schedule", channeltype.OpenAI, names, "public", nil)
	custom := createDisplayChannel(t, "override-sol-schedule", channeltype.OpenAI, names, "public", map[string]model.ModelConfigLocal{
		"gpt-5.6":     {Ratio: 7 * m, CompletionRatio: 3, CachedInputRatio: 2 * m},
		"gpt-5.6-sol": {Ratio: 7 * m, CompletionRatio: 3, CachedInputRatio: 2 * m},
	})
	cutoff := time.Date(2026, time.December, 1, 0, 0, 0, 0, time.UTC)
	pinModelsDisplayClock(t, cutoff.Add(-time.Nanosecond))
	response := fetchModelsDisplay(t, 0, "")
	for _, name := range []string{"gpt-5.6", "gpt-5.6-sol"} {
		info := response.Data[channeltype.IdToName(native.Type)+":"+native.Name].Models[name]
		overridden := response.Data[channeltype.IdToName(custom.Type)+":"+custom.Name].Models[name]
		require.InDelta(t, 4, info.InputPrice, 1e-9)
		require.Len(t, info.TimeWindows, 1)
		require.InDelta(t, 7, overridden.InputPrice, 1e-9)
		require.InDelta(t, 21, overridden.OutputPrice, 1e-9)
		require.Empty(t, overridden.TimeWindows)

		for _, when := range []time.Time{cutoff.Add(-time.Nanosecond), cutoff, cutoff.Add(time.Hour)} {
			window := info.TimeWindows[0]
			matches := pricing.MatchTimeWindow(adaptor.TimeWindow{
				TimeZone: window.TimeZone, DateFrom: window.DateFrom, DateTo: window.DateTo,
				Ranges: []adaptor.ClockRange{{Start: window.Ranges[0].Start, End: window.Ranges[0].End}},
			}, when)
			require.Equal(t, !when.Before(cutoff), matches)
			if !matches {
				require.InDelta(t, 20, info.OutputPrice, 1e-9)
				continue
			}

			blob, err := json.Marshal(window)
			require.NoError(t, err)
			var projected struct {
				EffectivePricing *struct {
					Input      float64            `json:"input_price"`
					Output     float64            `json:"output_price"`
					Cached     float64            `json:"cached_input_price"`
					CacheWrite float64            `json:"cache_write_5m_price"`
					Tiers      []ModelDisplayTier `json:"tiers"`
				} `json:"effective_pricing"`
			}
			require.NoError(t, json.Unmarshal(blob, &projected))
			require.NotNil(t, projected.EffectivePricing)
			quote := projected.EffectivePricing
			cfg, ok := pricing.ResolveModelConfig(name, nil, displayProvider(t, native), when)
			require.True(t, ok)
			current := pricing.ResolveEffectivePricingFromConfig(272_000, cfg)
			require.InDelta(t, current.InputRatio/m, quote.Input, 1e-9)
			require.InDelta(t, current.OutputRatio/m, quote.Output, 1e-9)
			require.InDelta(t, current.CachedInputRatio/m, quote.Cached, 1e-9)
			require.InDelta(t, current.CacheWrite5mRatio/m, quote.CacheWrite, 1e-9)
			require.Len(t, quote.Tiers, 1)
			tier := quote.Tiers[0]
			require.Equal(t, 272_001, tier.InputTokenThreshold)
			long := pricing.ResolveEffectivePricingFromConfig(tier.InputTokenThreshold, cfg)
			require.InDelta(t, long.InputRatio/m, tier.InputPrice, 1e-9)
			require.InDelta(t, long.OutputRatio/m, tier.OutputPrice, 1e-9)
			require.InDelta(t, long.CachedInputRatio/m, tier.CachedInputPrice, 1e-9)
			require.InDelta(t, long.CacheWrite5mRatio/m, tier.CacheWrite5mPrice, 1e-9)
		}
	}
}
