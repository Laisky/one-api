package openai

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// TestGetModelListFromPricingIncludesGPT6Astra verifies the new model is
// discoverable through the adapter's generated model list.
func TestGetModelListFromPricingIncludesGPT6Astra(t *testing.T) {
	t.Parallel()

	require.Contains(t, adaptor.GetModelListFromPricing(ModelRatios), "gpt-6-astra")
}

// TestGPT6AstraPricingAndMetadata verifies the documented base pricing,
// long-context surcharge, token limits, modalities, and capability metadata.
func TestGPT6AstraPricingAndMetadata(t *testing.T) {
	t.Parallel()

	cfg, ok := ModelRatios["gpt-6-astra"]
	require.True(t, ok, "ModelRatios must contain gpt-6-astra")

	require.InDelta(t, 10.0*ratio.MilliTokensUsd, cfg.Ratio, 1e-12)
	require.InDelta(t, 5.0, cfg.CompletionRatio, 1e-9)
	require.InDelta(t, 1.0*ratio.MilliTokensUsd, cfg.CachedInputRatio, 1e-12)
	require.InDelta(t, 12.5*ratio.MilliTokensUsd, cfg.CacheWrite5mRatio, 1e-12)
	require.Zero(t, cfg.CacheWrite1hRatio)
	require.Equal(t, int32(1_050_000), cfg.ContextLength)
	require.Equal(t, int32(128_000), cfg.MaxOutputTokens)
	require.Equal(t, []string{"text", "image"}, cfg.InputModalities)
	require.Equal(t, []string{"text"}, cfg.OutputModalities)
	require.Contains(t, cfg.SupportedFeatures, "reasoning")
	require.Contains(t, cfg.SupportedFeatures, "tools")
	require.Contains(t, cfg.SupportedFeatures, "structured_outputs")
	require.Contains(t, cfg.SupportedFeatures, "web_search")

	require.Len(t, cfg.Tiers, 1)
	tier := cfg.Tiers[0]
	require.Equal(t, 272_001, tier.InputTokenThreshold)
	require.InDelta(t, 20.0*ratio.MilliTokensUsd, tier.Ratio, 1e-12)
	require.InDelta(t, 3.75, tier.CompletionRatio, 1e-9)
	require.InDelta(t, 2.0*ratio.MilliTokensUsd, tier.CachedInputRatio, 1e-12)
	require.InDelta(t, 25.0*ratio.MilliTokensUsd, tier.CacheWrite5mRatio, 1e-12)
	require.Zero(t, tier.CacheWrite1hRatio)
}

// TestGPT6AstraReasoningEfforts verifies Astra accepts its documented effort
// ladder while rejecting non-reasoning values that Astra does not support.
func TestGPT6AstraReasoningEfforts(t *testing.T) {
	t.Parallel()

	cfg := ModelRatios["gpt-6-astra"]
	require.Equal(t, gpt6AstraReasoningEfforts, cfg.SupportedReasoningEfforts)
	require.Equal(t, "medium", defaultReasoningEffortForModel("gpt-6-astra"))
	require.True(t, isModelSupportedReasoning("gpt-6-astra"))
	require.False(t, isMediumOnlyReasoningModel("gpt-6-astra"))

	for _, effort := range gpt6AstraReasoningEfforts {
		require.Truef(t, isReasoningEffortAllowedForModel("gpt-6-astra", effort), "effort %q must be allowed", effort)

		requested := effort
		require.Equalf(t, effort, *normalizeReasoningEffortForModel("gpt-6-astra", &requested), "effort %q must pass through", effort)
	}

	for _, effort := range []string{"none", "minimal", "ultra"} {
		require.Falsef(t, isReasoningEffortAllowedForModel("gpt-6-astra", effort), "effort %q must be rejected", effort)

		requested := effort
		require.Equalf(t, "medium", *normalizeReasoningEffortForModel("gpt-6-astra", &requested), "effort %q must fall back to the default", effort)
	}
}
