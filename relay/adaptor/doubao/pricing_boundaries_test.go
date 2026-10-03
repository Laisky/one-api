package doubao_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/doubao"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/pricing"
)

// TestSeed16PricingBoundaries exercises the real pricing resolver around the
// existing catalog's (32K,128K] and (128K,256K] bands using t. Thresholds use
// tokens, not thousands of tokens; this correction does not change tariffs.
func TestSeed16PricingBoundaries(t *testing.T) {
	t.Parallel()
	for id, input := range map[string][3]float64{
		"doubao-seed-1.6":        {.8, 1.2, 2.4},
		"doubao-seed-1.6-flash":  {.15, .3, .6},
		"doubao-seed-1.6-vision": {.8, 1.2, 2.4},
		"doubao-seed-1.6-lite":   {.3, .6, 1.2},
	} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			cfg, ok := doubao.ModelRatios[id]
			require.True(t, ok)
			for _, tc := range []struct{ tokens, tier int }{
				{32, 0}, {128, 0}, {32000, 0}, {32001, 1}, {128000, 1}, {128001, 2},
			} {
				got := pricing.ResolveEffectivePricingFromConfig(tc.tokens, cfg)
				require.InDelta(t, input[tc.tier], got.InputRatio/ratio.MilliTokensRmb, 1e-12, "tokens=%d", tc.tokens)
			}
		})
	}
}
