package replicate_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/replicate"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/pricing"
)

// TestGemini31LongContextTariff verifies the provider's strict greater-than-200k
// boundary through the production pricing resolver. It takes t and returns no values.
func TestGemini31LongContextTariff(t *testing.T) {
	t.Parallel()
	a := &replicate.Adaptor{}
	for _, tc := range []struct {
		tokens        int
		input, output float64
	}{
		{199_999, 2, 12}, {200_000, 2, 12}, {200_001, 4, 18}, {900_000, 4, 18},
	} {
		resolved := pricing.ResolveEffectivePricing("google/gemini-3.1-pro", tc.tokens, a)
		require.InDelta(t, tc.input, resolved.InputRatio/ratio.MilliTokensUsd, 1e-12)
		require.InDelta(t, tc.output, resolved.OutputRatio/ratio.MilliTokensUsd, 1e-12)
	}
	for _, id := range []string{"openai/gpt-5.6-sol", "openai/gpt-5.6-terra", "openai/gpt-5.6-luna"} {
		cfg := a.GetDefaultModelPricing()[id]
		require.Contains(t, a.GetModelList(), id)
		require.Zero(t, cfg.ContextLength, "do not copy an unconfirmed sibling-model context")
		require.Zero(t, cfg.MaxOutputTokens, "do not copy an unconfirmed sibling-model output limit")
	}
}
