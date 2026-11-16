package openrouter_test

import (
	"testing"
	"time"

	"github.com/Laisky/one-api/relay/adaptor/openrouter"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/stretchr/testify/require"
)

// TestNovemberOpenRouterTerraTier verifies OpenRouter's inclusive 272,000-token
// minimum independently of the native OpenAI threshold and preserves all buckets.
func TestNovemberOpenRouterTerraTier(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"openai/gpt-6.1-terra", "openai/gpt-6.1-terra-pro", "~openai/gpt-terra-latest"} {
		for _, tokens := range []int{271999, 272000, 272001, 1000000} {
			got := pricing.ResolveEffectivePricingForUsage(id, tokens, 1, &openrouter.Adaptor{})
			input, cached, output, write, threshold := 1.0, .05, 5.0, 1.25, 0
			if tokens >= 272000 {
				input, cached, output, write, threshold = 2, .1, 7.5, 2.5, 272000
			}
			require.Equal(t, threshold, got.AppliedTierThreshold, id)
			require.InDelta(t, input, got.InputRatio/ratio.MilliTokensUsd, 1e-12, id)
			require.InDelta(t, cached, got.CachedInputRatio/ratio.MilliTokensUsd, 1e-12, id)
			require.InDelta(t, output, got.OutputRatio/ratio.MilliTokensUsd, 1e-12, id)
			require.InDelta(t, write, got.CacheWrite5mRatio/ratio.MilliTokensUsd, 1e-12, id)
		}
	}
	require.Contains(t, openrouter.ModelRatios["openai/gpt-6.1-terra"].SupportedReasoningEfforts, "none")
	require.NotContains(t, openrouter.ModelRatios["openai/gpt-6.1-terra-pro"].SupportedReasoningEfforts, "none")
}

// TestNovemberOpenRouterClockBoundaries checks weekday, weekend, and midnight
// transitions in the exact host schedules rather than importing native tariffs.
func TestNovemberOpenRouterClockBoundaries(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		id, at                string
		input, cached, output float64
	}{
		{"deepseek/deepseek-v4.1-flash-1017", "2026-11-16T00:59:59Z", .15, .003, .6},
		{"deepseek/deepseek-v4.1-flash-1017", "2026-11-16T01:00:00Z", .3, .006, 1.2},
		{"deepseek/deepseek-v4.1-flash-1017", "2026-11-16T03:59:59Z", .3, .006, 1.2},
		{"deepseek/deepseek-v4.1-flash-1017", "2026-11-16T04:00:00Z", .15, .003, .6},
		{"deepseek/deepseek-v4.1-flash-1017", "2026-11-16T05:59:59Z", .15, .003, .6},
		{"deepseek/deepseek-v4.1-flash-1017", "2026-11-16T06:00:00Z", .3, .006, 1.2},
		{"deepseek/deepseek-v4.1-flash-1017", "2026-11-16T09:59:59Z", .3, .006, 1.2},
		{"deepseek/deepseek-v4.1-flash-1017", "2026-11-16T10:00:00Z", .15, .003, .6},
		{"deepseek/deepseek-v4.1-flash-1017", "2026-11-21T01:00:00Z", .15, .003, .6},
		{"deepseek/deepseek-v4.1-flash-1017", "2026-11-22T06:00:00Z", .15, .003, .6},
		{"minimax/minimax-m3.1-flash", "2026-11-16T00:59:59Z", .075, .0075, .3},
		{"minimax/minimax-m3.1-flash", "2026-11-16T01:00:00Z", .15, .015, .6},
		{"minimax/minimax-m3.1-flash", "2026-11-16T14:59:59Z", .15, .015, .6},
		{"minimax/minimax-m3.1-flash", "2026-11-16T15:00:00Z", .075, .0075, .3},
		{"minimax/minimax-m3.1-flash", "2026-11-16T23:59:59Z", .075, .0075, .3},
		{"minimax/minimax-m3.1-flash", "2026-11-17T00:00:00Z", .075, .0075, .3},
	} {
		at, err := time.Parse(time.RFC3339, tc.at)
		require.NoError(t, err)
		base := openrouter.ModelRatios[tc.id]
		got := pricing.ApplyTimeWindow(base, at)
		require.Equal(t, got, pricing.ApplyTimeWindow(base, at.In(time.FixedZone("UTC+8", 8*3600))))
		require.InDelta(t, tc.input, got.Ratio/ratio.MilliTokensUsd, 1e-12, tc.id+" "+tc.at)
		require.InDelta(t, tc.cached, got.CachedInputRatio/ratio.MilliTokensUsd, 1e-12, tc.id+" "+tc.at)
		require.InDelta(t, tc.output, got.Ratio*got.CompletionRatio/ratio.MilliTokensUsd, 1e-12, tc.id+" "+tc.at)
	}
}

// TestNovemberOpenRouterModalityAndUnknownRates prevents storage-only write
// charges, missing audio multipliers, invented free routers, and batch leakage.
func TestNovemberOpenRouterModalityAndUnknownRates(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		id                    string
		input, audio, storage float64
	}{
		{"google/gemini-4-hydrogen", 1.5, 1.5, 1.0 / 12},
		{"google/gemini-4-carbon", 2, 2, .1875},
		{"google/gemini-4-argon", 2, 2, .1875},
		{"google/gemini-3.8-flash", .825, .825, .5 / 12},
		{"google/gemini-3.1-flash-lite", .25, .5, 1.0 / 12},
		{"google/gemini-2.5-flash", .3, 1, 1.0 / 12},
	} {
		cfg := openrouter.ModelRatios[tc.id]
		require.NotNil(t, cfg.Audio, tc.id)
		require.InDelta(t, tc.audio, cfg.Ratio*cfg.Audio.PromptRatio/ratio.MilliTokensUsd, 1e-10, tc.id)
		require.InDelta(t, tc.input+tc.storage, cfg.CacheWrite5mRatio/ratio.MilliTokensUsd, 1e-10, tc.id)
	}
	for _, id := range []string{"typesafe/jev-router", "nvidia/switchyard", "openrouter/bodybuilder", "google/gemini-4-carbon:batch", "openai/gpt-6.1-terra:batch"} {
		require.NotContains(t, (&openrouter.Adaptor{}).GetModelList(), id)
	}
	require.Positive(t, openrouter.ModelRatios["inclusionai/ling-3.1-flash"].Ratio, "a formerly free model now has a published paid tariff")
	require.Zero(t, openrouter.ModelRatios["inclusionai/ling-3.2-flash:free"].Ratio, "explicitly free is distinct from unknown")
}
