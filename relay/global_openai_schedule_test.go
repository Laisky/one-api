package relay

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/pricing"
)

// TestNativeOpenAIScheduleDoesNotRepriceGlobalFallback verifies that an
// eligible non-native channel keeps shared fallback rates at the native cutoff.
func TestNativeOpenAIScheduleDoesNotRepriceGlobalFallback(t *testing.T) {
	InitializeGlobalPricing()
	cutoff := time.Date(2026, time.December, 1, 0, 0, 0, 0, time.UTC)
	native := GetAdaptor(apitype.OpenAI)
	native.Init(&meta.Meta{ChannelType: channeltype.OpenAI})
	other := GetAdaptor(apitype.Groq)
	other.Init(&meta.Meta{ChannelType: channeltype.Groq})

	for _, name := range []string{"gpt-5.6", "gpt-5.6-sol"} {
		t.Run(name, func(t *testing.T) {
			require.NotContains(t, other.GetDefaultModelPricing(), name)
			for _, row := range []struct {
				at                    time.Time
				input, cached, output float64
			}{
				{cutoff.Add(-time.Nanosecond), 4, .40, 20},
				{cutoff, 5, .50, 30},
				{cutoff.Add(time.Hour), 5, .50, 30},
			} {
				cfg, ok := pricing.ResolveModelConfig(name, nil, native, row.at)
				require.True(t, ok)
				p := pricing.ResolveEffectivePricingFromConfig(100, cfg)
				require.InDelta(t, row.input, p.InputRatio/ratio.MilliTokensUsd, 1e-9)
				require.InDelta(t, row.cached, p.CachedInputRatio/ratio.MilliTokensUsd, 1e-9)
				require.InDelta(t, row.output, p.OutputRatio/ratio.MilliTokensUsd, 1e-9)

				cfg, ok = pricing.ResolveModelConfig(name, nil, other, row.at)
				require.True(t, ok)
				p = pricing.ResolveEffectivePricingFromConfig(100, cfg)
				require.InDelta(t, 4, p.InputRatio/ratio.MilliTokensUsd, 1e-9)
				require.InDelta(t, .40, p.CachedInputRatio/ratio.MilliTokensUsd, 1e-9)
				require.InDelta(t, 20, p.OutputRatio/ratio.MilliTokensUsd, 1e-9)
			}
		})
	}
}
