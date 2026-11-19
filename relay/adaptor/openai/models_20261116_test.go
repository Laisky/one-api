package openai_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/pricing"
)

// TestGPT6CyberEffectivePricing checks discovery and Standard billing at the
// input-only long-context boundary. Parameter t reports failures; nothing returns.
func TestGPT6CyberEffectivePricing(t *testing.T) {
	t.Parallel()
	const name = "gpt-6-cyber"
	provider := &openai.Adaptor{ChannelType: channeltype.OpenAI}
	require.Contains(t, provider.GetModelList(), name)
	cfg, ok := provider.GetDefaultModelPricing()[name]
	require.True(t, ok, "published models must not fall back to unknown-model pricing")
	require.EqualValues(t, 1_050_000, cfg.ContextLength)
	require.EqualValues(t, 128_000, cfg.MaxOutputTokens)
	require.Equal(t, []string{"text", "image"}, cfg.InputModalities)
	require.Equal(t, []string{"text"}, cfg.OutputModalities)
	require.Contains(t, cfg.SupportedFeatures, "reasoning")
	require.Contains(t, cfg.SupportedFeatures, "structured_outputs")
	require.Empty(t, cfg.SupportedReasoningEfforts, "the Cyber card does not publish a discrete effort ladder")
	require.NotContains(t, provider.GetModelList(), name+"-2026-11-05")
	for _, input := range []int{272_000, 272_001} {
		for _, output := range []int{0, 128_000} {
			effective := pricing.ResolveEffectivePricingForUsage(name, input, output, provider)
			inRate, outRate, cacheRate := 20.0, 100.0, 2.0
			if input > 272_000 {
				inRate, outRate, cacheRate = 40, 150, 4
			}
			require.InDelta(t, inRate, effective.InputRatio/ratio.MilliTokensUsd, 1e-9)
			require.InDelta(t, outRate, effective.OutputRatio/ratio.MilliTokensUsd, 1e-9)
			require.InDelta(t, cacheRate, effective.CachedInputRatio/ratio.MilliTokensUsd, 1e-9)
			if input <= 272_000 {
				require.InDelta(t, 25, effective.CacheWrite5mRatio/ratio.MilliTokensUsd, 1e-9)
			}
			require.Zero(t, effective.CacheWrite1hRatio, "12h writes must not be mislabeled as Claude 1h writes")
		}
	}
	require.Zero(t, cfg.Tiers[0].CacheWrite5mRatio, "no guessed long-context cache-write tariff")
	require.InDelta(t, 12.5, provider.GetModelRatio("gpt-5.6-cyber")/ratio.MilliTokensUsd, 1e-9)
}

// TestGPTImage26Discovery checks the published image IDs and existing token-only
// billing contract. Parameter t reports failures; nothing returns.
func TestGPTImage26Discovery(t *testing.T) {
	t.Parallel()
	provider := &openai.Adaptor{ChannelType: channeltype.OpenAI}
	for _, name := range []string{"gpt-image-2.6-sunburst", "gpt-image-2.6-flare"} {
		t.Run(name, func(t *testing.T) {
			require.Contains(t, provider.GetModelList(), name)
			cfg, ok := provider.GetDefaultModelPricing()[name]
			require.True(t, ok)
			require.InDelta(t, 5, cfg.Ratio/ratio.MilliTokensUsd, 1e-9)
			require.InDelta(t, 1.25, cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-9)
			require.InDelta(t, 30, cfg.Ratio*cfg.CompletionRatio/ratio.MilliTokensUsd, 1e-9)
			require.NotNil(t, cfg.Image)
			require.InDelta(t, 8, cfg.Ratio*cfg.Image.PromptRatio/ratio.MilliTokensUsd, 1e-9)
			require.Zero(t, cfg.Image.PricePerImageUsd, "actual token usage must not add a fixed render charge")
			require.Equal(t, "auto", cfg.Image.DefaultQuality)
			require.Equal(t, []string{"image"}, cfg.OutputModalities)
			require.Empty(t, cfg.SupportedFeatures, "a native image model must not be advertised as chat")
			require.NotContains(t, provider.GetModelList(), name+"-2026-11-04")
		})
	}
}

// TestNovemberLifecycleGuidance checks superseding published notices while
// retaining historical IDs and prices. Parameter t reports failures; nothing returns.
func TestNovemberLifecycleGuidance(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"gpt-image-2", "gpt-image-2-2026-04-21"} {
		cfg, ok := openai.ModelRatios[name]
		require.True(t, ok)
		require.Contains(t, cfg.Description, "2027-02-10")
		require.Contains(t, cfg.Description, "gpt-image-2.6-sunburst")
		require.InDelta(t, 5, cfg.Ratio/ratio.MilliTokensUsd, 1e-9)
	}
	for name, replacement := range map[string]string{
		"gpt-5": "gpt-6.1-sol", "gpt-5-mini": "gpt-6.1-terra", "gpt-5-nano": "gpt-6-luna",
		"gpt-5-pro": "reasoning.mode=pro", "o3": "gpt-6.1-sol",
	} {
		require.Contains(t, openai.ModelRatios[name].Description, "2026-12-11", name)
		require.Contains(t, openai.ModelRatios[name].Description, replacement, name)
	}
	require.Contains(t, openai.ModelRatios["gpt-image-1.5-2025-12-16"].Description, "2026-12-01")
	require.Contains(t, openai.ModelRatios["gpt-4o-mini-transcribe-2025-12-15"].Description, "2027-02-26")
	require.NotContains(t, openai.ModelRatios["gpt-image-2.5-sunburst"].Description, "announced shutdown")
}
