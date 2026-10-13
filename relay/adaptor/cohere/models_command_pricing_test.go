package cohere_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/cohere"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// TestCommandResearchAPITariffs checks discovery and explicit free API pricing
// through the public adaptor methods using t. It returns no values.
func TestCommandResearchAPITariffs(t *testing.T) {
	t.Parallel()
	a := &cohere.Adaptor{}
	for _, id := range []string{
		"command-a-plus-05-2026",
		"command-a-reasoning-08-2025",
		"command-a-translate-08-2025",
		"command-a-vision-07-2025",
	} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			cfg, exists := a.GetDefaultModelPricing()[id]
			require.True(t, exists)
			require.Contains(t, a.GetModelList(), id)
			require.Zero(t, cfg.Ratio)
			require.EqualValues(t, 1, cfg.CompletionRatio)
			require.Zero(t, a.GetModelRatio(id), "an explicit free price must not become an unknown-model fallback")
			require.Zero(t, a.GetModelRatio(id)*a.GetCompletionRatio(id))
			require.Zero(t, cfg.CachedInputRatio)
			require.Empty(t, cfg.Tiers)
			require.Empty(t, cfg.TimeWindows)
			require.Contains(t, cfg.Description, "free until rate limits")
			require.Contains(t, cfg.Description, "configured separately")
			require.NotContains(t, cfg.Description, "estimate")
		})
	}
}

// TestCommandResearchPricingDoesNotChangePaidModels pins neighboring paid
// tariffs using t so a research-model update cannot make all Command IDs free.
// It returns no values.
func TestCommandResearchPricingDoesNotChangePaidModels(t *testing.T) {
	t.Parallel()
	a := &cohere.Adaptor{}
	for id, prices := range map[string][2]float64{
		"command-a-03-2025":      {2.5, 10},
		"command-r-plus-08-2024": {2.5, 10},
		"command-r-08-2024":      {0.15, 0.60},
		"command-r7b-12-2024":    {0.0375, 0.15},
		"c4ai-aya-expanse-32b":   {0.5, 1.5},
		"command":                {1, 2},
	} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			require.InDelta(t, prices[0], a.GetModelRatio(id)/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, prices[1], a.GetModelRatio(id)*a.GetCompletionRatio(id)/ratio.MilliTokensUsd, 1e-12)
		})
	}
}
