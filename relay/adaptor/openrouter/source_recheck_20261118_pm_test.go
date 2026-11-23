package openrouter_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openrouter"
	"github.com/Laisky/one-api/relay/billing/ratio"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/Laisky/one-api/relay/quota"
)

// recheck20261118PMTariff is one OpenRouter /api/v1/models quote from the
// November 18 fetch, refreshed against the 2026-11-22 listing, in USD per million tokens. A zero cached price
// means the listing publishes no input_cache_read price, so cached prompt
// tokens must fall back to the input rate; a zero write price means no
// input_cache_write price is published.
type recheck20261118PMTariff struct {
	id                           string
	input, cached, output, write float64
}

// recheck20261118PMTariffs lists every catalog entry whose listed price moved
// in the November 18 regression set. Published decimals are refreshed against
// https://openrouter.ai/api/v1/models on 2026-11-22; scheduled models use the peak base.
var recheck20261118PMTariffs = []recheck20261118PMTariff{
	{"deepseek/deepseek-r1-0528", 0.571, 0, 2.286, 0},
	{"deepseek/deepseek-v3.1-terminus", 0.27, 0, 1, 0},
	{"deepseek/deepseek-v4.1-flash", 0.0498, 0.000996, 0.1992, 0},
	{"deepseek/deepseek-v4.1-flash-1017", 0.3, 0.006, 1.2, 0},
	{"google/gemma-3-27b-it", 0.08, 0.04, 0.45, 0},
	{"google/gemma-4-31b-it", 0.2, 0, 0.4, 0},
	{"meta/muse-glimmer-30b", 0.35, 0.04, 1.5, 0},
	{"meta-llama/llama-3.1-8b-instruct", 0.02, 0, 0.04, 0},
	{"mistralai/mistral-medium-3", 0.44, 0.044, 2.2, 0},
	{"mistralai/mistral-medium-3.1", 0.44, 0.044, 2.2, 0},
	{"mistralai/mistral-small-3.2-24b-instruct", 0.075, 0, 0.2, 0},
	{"moonshotai/kimi-k2.7-code", 0.6, 0.12, 3, 0},
	{"nvidia/nemotron-3-ultra-550b-a55b", 0.5, 0.1, 2.2, 0},
	{"openai/gpt-oss-20b", 0.017, 0.0085, 0.085, 0},
	{"qwen/qwen3-next-80b-a3b-instruct", 0.1, 0.07, 1.1, 0},
	{"qwen/qwen3.5-27b", 0.26, 0, 2.6, 0},
	{"qwen/qwen3.5-9b", 0.17, 0, 0.25, 0},
	{"qwen/qwen3.8-27b", 0.035, 0.0175, 1.5, 0},
	{"qwen/qwen4-max", 1.425, 0.07125, 4.275, 1.78125},
	{"tencent/hy4-flash", 0.035, 0.0015, 0.1405, 0},
	{"xiaomi/mimo-v2.5-pro", 0.48, 0.096, 1.8, 0},
	{"z-ai/glm-4.6v", 0.3, 0.055, 0.9, 0},
	{"z-ai/glm-4.7-flash", 0.0605, 0, 0.4, 0},
	{"z-ai/glm-5.3-flash", 0.0399, 0.03, 0.0784, 0},
	{"~z-ai/glm-flash-latest", 0.0399, 0.03, 0.0784, 0},
	{"z-ai/glm-5.3-prime", 1.68, 0.336, 5.28, 0},
}

// TestOpenRouterRecheck20261118PMPublishedTariffs pins each moved catalog entry
// to the current published decimals through both the raw catalog and adaptor
// ratio accessors, retaining the original regression coverage. It accepts t
// and returns no values.
func TestOpenRouterRecheck20261118PMPublishedTariffs(t *testing.T) {
	t.Parallel()
	a := &openrouter.Adaptor{}
	for _, tc := range recheck20261118PMTariffs {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			cfg, exists := openrouter.ModelRatios[tc.id]
			require.True(t, exists)
			require.InDelta(t, tc.input, cfg.Ratio/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, tc.input, a.GetModelRatio(tc.id)/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, tc.output, cfg.Ratio*cfg.CompletionRatio/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, tc.output, a.GetModelRatio(tc.id)*a.GetCompletionRatio(tc.id)/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, tc.cached, cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, tc.write, cfg.CacheWrite5mRatio/ratio.MilliTokensUsd, 1e-12)
			require.Zero(t, cfg.CacheWrite1hRatio)
			require.Empty(t, cfg.Tiers, "the listing publishes no long-context override for %s", tc.id)
			if tc.id == "deepseek/deepseek-v4.1-flash-1017" {
				require.Len(t, cfg.TimeWindows, 2, "weekday and weekend off-peak windows are published")
			} else {
				require.Empty(t, cfg.TimeWindows, "the listing publishes no time-of-day override for %s", tc.id)
			}
		})
	}
}

// TestOpenRouterRecheck20261118PMUnpublishedCacheReadBillsAtInput settles a
// fully cached prompt for each entry whose 2026-11-18 listing no longer
// publishes an input_cache_read price. Without a published discount the cached
// tokens must cost the full input price; the former catalog applied
// unpublished cache-read prices taken from other endpoints. The control row
// proves the harness still applies a published cache-read price. It accepts t
// and returns no values.
func TestOpenRouterRecheck20261118PMUnpublishedCacheReadBillsAtInput(t *testing.T) {
	t.Parallel()
	a := &openrouter.Adaptor{}
	const promptTokens = 1_000_000
	for _, tc := range []struct {
		id          string
		cachedPrice float64
	}{
		{"deepseek/deepseek-r1-0528", 0.571},
		{"deepseek/deepseek-v3.1-terminus", 0.27},
		{"google/gemma-4-31b-it", 0.2},
		{"meta-llama/llama-3.1-8b-instruct", 0.02},
		{"mistralai/mistral-small-3.2-24b-instruct", 0.075},
		{"z-ai/glm-4.7-flash", 0.0605},
		{"z-ai/glm-5.3-flash", 0.03},
	} {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			result := quota.Compute(quota.ComputeInput{
				ModelName: tc.id, ModelRatio: a.GetModelRatio(tc.id), PricingAdaptor: a, GroupRatio: 1,
				Usage: &relaymodel.Usage{
					PromptTokens:        promptTokens,
					PromptTokensDetails: &relaymodel.UsagePromptTokensDetails{CachedTokens: promptTokens},
				},
			})
			want := float64(promptTokens) * tc.cachedPrice / 1e6 * ratio.QuotaPerUsd
			require.InDelta(t, want, float64(result.TotalQuota), 1, "%s cached prompt tokens", tc.id)
		})
	}
}

// TestOpenRouterRecheck20261118PMAstraAliasTier pins `~openai/gpt-astra-latest`
// on either side of OpenRouter's strict min_prompt_tokens 272,000 bound (stored
// as 272,001). On 2026-11-18 the alias redirects to the newly published
// openai/gpt-6.2-astra and quotes $8/$40 (cache read $0.40, write $10) below the
// bound and $16/$60 (cache read $0.80, write $20) above it. It accepts t and
// returns no values.
func TestOpenRouterRecheck20261118PMAstraAliasTier(t *testing.T) {
	t.Parallel()
	a := &openrouter.Adaptor{}
	for _, tc := range []struct {
		tokens, threshold              int
		input, cached, output, write5m float64
	}{
		{1, 0, 8, 0.4, 40, 10},
		{272000, 0, 8, 0.4, 40, 10},
		{272001, 272001, 16, 0.8, 60, 20},
		{1000000, 272001, 16, 0.8, 60, 20},
	} {
		got := pricing.ResolveEffectivePricingForUsage("~openai/gpt-astra-latest", tc.tokens, 1, a)
		require.Equal(t, tc.threshold, got.AppliedTierThreshold, tc.tokens)
		require.InDelta(t, tc.input, got.InputRatio/ratio.MilliTokensUsd, 1e-12, tc.tokens)
		require.InDelta(t, tc.cached, got.CachedInputRatio/ratio.MilliTokensUsd, 1e-12, tc.tokens)
		require.InDelta(t, tc.output, got.OutputRatio/ratio.MilliTokensUsd, 1e-12, tc.tokens)
		require.InDelta(t, tc.write5m, got.CacheWrite5mRatio/ratio.MilliTokensUsd, 1e-12, tc.tokens)
	}
}

// TestOpenRouterRoutingAliasesKeepPublishedTariffs checks that routing aliases
// retain their own published host quotes. A pinned model can choose a different
// endpoint and schedule, so equal family names do not imply equal tariffs.
// It accepts t and returns no values.
func TestOpenRouterRoutingAliasesKeepPublishedTariffs(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		id                    string
		input, cached, output float64
		windows               int
	}{
		{"~deepseek/deepseek-flash-latest", 0.036, 0.01, 0.5, 0},
		{"deepseek/deepseek-v4.1-flash-1017", 0.3, 0.006, 1.2, 2},
		{"~z-ai/glm-flash-latest", 0.0399, 0.03, 0.0784, 0},
		{"z-ai/glm-5.3-flash", 0.0399, 0.03, 0.0784, 0},
	} {
		cfg, exists := openrouter.ModelRatios[tc.id]
		require.True(t, exists, tc.id)
		require.InDelta(t, tc.input, cfg.Ratio/ratio.MilliTokensUsd, 1e-12, tc.id)
		require.InDelta(t, tc.cached, cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-12, tc.id)
		require.InDelta(t, tc.output, cfg.Ratio*cfg.CompletionRatio/ratio.MilliTokensUsd, 1e-12, tc.id)
		require.Len(t, cfg.TimeWindows, tc.windows, tc.id)
	}
}

// TestOpenRouterRecheck20261118PMWindowBaseIsPeak sweeps every OpenRouter entry
// with time-of-day windows. OpenRouter's top-level price is whichever UTC
// override is active when the listing is read, so the catalog keeps the highest
// published window as the base: any path that reads the base without applying a
// window must never price below a published rate. It accepts t and returns no
// values.
func TestOpenRouterRecheck20261118PMWindowBaseIsPeak(t *testing.T) {
	t.Parallel()
	windowed := map[string]bool{}
	for id, cfg := range openrouter.ModelRatios {
		if len(cfg.TimeWindows) == 0 {
			continue
		}
		windowed[id] = true
		baseOutput := cfg.Ratio * cfg.CompletionRatio
		for _, window := range cfg.TimeWindows {
			overlay := window.Overlay
			input := cfg.Ratio
			if overlay.Ratio != 0 {
				input = overlay.Ratio
			}
			completion := cfg.CompletionRatio
			if overlay.CompletionRatio != 0 {
				completion = overlay.CompletionRatio
			}
			cached := cfg.CachedInputRatio
			if overlay.CachedInputRatio != 0 {
				cached = overlay.CachedInputRatio
			}
			require.LessOrEqualf(t, input, cfg.Ratio, "%s window %s input", id, window.Name)
			require.LessOrEqualf(t, input*completion, baseOutput*(1+1e-12), "%s window %s output", id, window.Name)
			require.LessOrEqualf(t, cached, cfg.CachedInputRatio, "%s window %s cached input", id, window.Name)
		}
	}
	for _, id := range []string{"minimax/minimax-m3.1-flash", "deepseek/deepseek-v4.1-flash-1017", "tencent/hy4-preview"} {
		require.True(t, windowed[id], "%s still publishes UTC time-of-day overrides", id)
	}
	require.False(t, windowed["tencent/hy3"], "the 2026-11-22 listing publishes a flat Hy3 tariff")

	minimax := openrouter.ModelRatios["minimax/minimax-m3.1-flash"]
	require.InDelta(t, 0.15, minimax.Ratio/ratio.MilliTokensUsd, 1e-12)
	require.InDelta(t, 0.6, minimax.Ratio*minimax.CompletionRatio/ratio.MilliTokensUsd, 1e-12)
	require.InDelta(t, 0.015, minimax.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
}
