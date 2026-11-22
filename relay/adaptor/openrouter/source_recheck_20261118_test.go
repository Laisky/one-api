package openrouter_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openrouter"
	"github.com/Laisky/one-api/relay/billing/ratio"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/Laisky/one-api/relay/quota"
)

// TestOpenRouterLongContextThresholdIsStrictlyGreater pins OpenRouter's
// documented override condition: `min_prompt_tokens` "applies when total prompt
// tokens are strictly greater than this threshold"
// (https://openrouter.ai/docs/guides/overview/models, checked 2026-11-18). The
// gateway's tier test is inclusive (inputTokens >= InputTokenThreshold), so a
// published `min_prompt_tokens: N` must be stored as N+1. The table pins the
// 2026-11-18 /api/v1/models overrides for the reviewed IDs; the sweep rejects any
// tier that copied a round published minimum verbatim. It accepts t and returns
// no values.
func TestOpenRouterLongContextThresholdIsStrictlyGreater(t *testing.T) {
	t.Parallel()
	a := &openrouter.Adaptor{}
	for _, tc := range []struct {
		id                    string
		minPromptTokens       int
		baseInput, tierInput  float64
		baseOutput, tierOuput float64
	}{
		{"openai/gpt-5.6-sol", 272000, 4, 8, 20, 30},
		{"openai/gpt-5.6-sol-pro", 272000, 4, 8, 20, 30},
		{"~openai/gpt-terra-latest", 272000, 1, 2, 5, 7.5},
		{"~x-ai/grok-latest", 200000, 2, 4, 6, 12},
		{"x-ai/grok-4.7", 200000, 2, 4, 6, 12},
	} {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			at := pricing.ResolveEffectivePricingForUsage(tc.id, tc.minPromptTokens, 1, a)
			require.Zero(t, at.AppliedTierThreshold, "a prompt of exactly min_prompt_tokens is billed at the base tariff")
			require.InDelta(t, tc.baseInput, at.InputRatio/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, tc.baseOutput, at.OutputRatio/ratio.MilliTokensUsd, 1e-12)

			above := pricing.ResolveEffectivePricingForUsage(tc.id, tc.minPromptTokens+1, 1, a)
			require.Equal(t, tc.minPromptTokens+1, above.AppliedTierThreshold)
			require.InDelta(t, tc.tierInput, above.InputRatio/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, tc.tierOuput, above.OutputRatio/ratio.MilliTokensUsd, 1e-12)
		})
	}

	for id, cfg := range openrouter.ModelRatios {
		for i, tier := range cfg.Tiers {
			if tier.InputTokenThreshold <= 0 {
				continue
			}
			require.NotZerof(t, tier.InputTokenThreshold%1000,
				"%s tier %d uses %d: OpenRouter min_prompt_tokens is a strict lower bound, store min_prompt_tokens+1",
				id, i, tier.InputTokenThreshold)
		}
	}
}

// TestOpenRouterGrokLatestCacheWritesBillAtInput keeps reported cache writes on
// the `~x-ai/grok-latest` alias billable. Its 2026-11-18 listing quotes
// `input_cache_write: "0"` only because the alias's top provider is Amazon
// Bedrock; the alias redirects to `x-ai/grok-4.7`, whose first-party xAI
// endpoints publish no cache-write price, and OpenRouter's caching guide only
// says Grok writes carry "no cost" (no surcharge). That does not establish that
// write tokens are removed from the prompt charge, so the alias must bill them
// at the input rate exactly like its target instead of for free. It accepts t
// and returns no values.
func TestOpenRouterGrokLatestCacheWritesBillAtInput(t *testing.T) {
	t.Parallel()
	a := &openrouter.Adaptor{}
	alias := openrouter.ModelRatios["~x-ai/grok-latest"]
	require.Zero(t, alias.CacheWrite5mRatio, "unspecified write price falls back to input; -1 would be free")
	require.Zero(t, alias.CacheWrite1hRatio)
	require.Len(t, alias.Tiers, 1)
	require.Zero(t, alias.Tiers[0].CacheWrite5mRatio)
	require.Zero(t, alias.Tiers[0].CacheWrite1hRatio)

	for _, tc := range []struct {
		prompt     int
		inputPrice float64
	}{
		{100000, 2},
		{250000, 4},
	} {
		for _, id := range []string{"~x-ai/grok-latest", "x-ai/grok-4.7"} {
			result := quota.Compute(quota.ComputeInput{
				ModelName: id, ModelRatio: a.GetModelRatio(id), PricingAdaptor: a, GroupRatio: 1,
				Usage: &relaymodel.Usage{PromptTokens: tc.prompt, CacheWrite5mTokens: tc.prompt},
			})
			want := int64(math.Round(float64(tc.prompt) * tc.inputPrice / 1e6 * ratio.QuotaPerUsd))
			require.Equal(t, want, result.TotalQuota, "%s writes must be billed at the input price", id)
		}
	}
}

// TestOpenRouterGLMFlashOutputQuote pins the 2026-11-22 /api/v1/models quote
// for `z-ai/glm-5.3-flash` ($0.0399 input, $0.03 cache read, $0.0784 output
// per million). The independently published `~z-ai/glm-flash-latest` quote
// currently matches the snapshot. The completion ratio must
// be the exact quotient of the published prices rather than a truncated
// 15-digit printout. It accepts t and returns no values.
func TestOpenRouterGLMFlashOutputQuote(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"z-ai/glm-5.3-flash", "~z-ai/glm-flash-latest"} {
		cfg, exists := openrouter.ModelRatios[id]
		require.True(t, exists, id)
		require.InDelta(t, 0.0399, cfg.Ratio/ratio.MilliTokensUsd, 1e-12, id)
		require.InDelta(t, 0.03, cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-12, id)
		require.Equal(t, 0.0784/0.0399, cfg.CompletionRatio, id)
	}
}
