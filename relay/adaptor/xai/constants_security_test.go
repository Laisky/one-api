package xai

import (
	"testing"

	"github.com/stretchr/testify/require"

	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
)

// TestLegacyGrokAliasesHaveExplicitPricing verifies that historical xAI model
// names remain in the adapter pricing table so configured channels do not fall
// through to generic fallback billing for input or output token prices.
func TestLegacyGrokAliasesHaveExplicitPricing(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		expectedRatio           float64
		expectedCompletionRatio float64
	}{
		"grok-3-fast":      {expectedRatio: 3.0 * billingratio.MilliTokensUsd, expectedCompletionRatio: 15.0 / 3.0},
		"grok-3-mini-fast": {expectedRatio: 0.3 * billingratio.MilliTokensUsd, expectedCompletionRatio: 0.5 / 0.3},
		"grok-2-1212":      {expectedRatio: 2.0 * billingratio.MilliTokensUsd, expectedCompletionRatio: 10.0 / 2.0},
		"grok-beta":        {expectedRatio: 2.0 * billingratio.MilliTokensUsd, expectedCompletionRatio: 10.0 / 2.0},
		"grok-2":           {expectedRatio: 2.0 * billingratio.MilliTokensUsd, expectedCompletionRatio: 10.0 / 2.0},
		"grok-2-latest":    {expectedRatio: 2.0 * billingratio.MilliTokensUsd, expectedCompletionRatio: 10.0 / 2.0},
		"grok-vision-beta": {expectedRatio: 2.0 * billingratio.MilliTokensUsd, expectedCompletionRatio: 10.0 / 2.0},
	}

	adaptor := &Adaptor{}
	modelList := adaptor.GetModelList()

	for modelName, expected := range testCases {
		modelName := modelName
		expected := expected
		t.Run(modelName, func(t *testing.T) {
			t.Parallel()

			cfg, exists := ModelRatios[modelName]
			require.True(t, exists, "legacy xAI model %s must have explicit pricing", modelName)
			require.Contains(t, modelList, modelName, "legacy xAI model %s must remain selectable for billing continuity", modelName)
			require.Equal(t, expected.expectedRatio, cfg.Ratio)
			require.Equal(t, expected.expectedCompletionRatio, cfg.CompletionRatio)
			require.Equal(t, expected.expectedRatio, adaptor.GetModelRatio(modelName))
			require.Equal(t, expected.expectedCompletionRatio, adaptor.GetCompletionRatio(modelName))
		})
	}
}

// TestGrok3PricingUsesOutputDividedByInputRatio verifies that Grok 3 billing
// keeps CompletionRatio aligned with output price divided by input price.
func TestGrok3PricingUsesOutputDividedByInputRatio(t *testing.T) {
	t.Parallel()

	cfg, exists := ModelRatios["grok-3"]
	require.True(t, exists, "grok-3 must have explicit pricing")
	require.Equal(t, 1.25*billingratio.MilliTokensUsd, cfg.Ratio)
	require.Equal(t, 2.5/1.25, cfg.CompletionRatio)
}
