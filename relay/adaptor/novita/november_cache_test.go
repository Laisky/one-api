package novita_test

import (
	"testing"

	"github.com/Laisky/one-api/relay/adaptor/novita"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/stretchr/testify/require"
)

// TestNovemberNovitaCacheTariffs checks exact decimal host quotes for both TTLs
// and keeps unknown router prices distinct from the newly priced Ling model.
func TestNovemberNovitaCacheTariffs(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		id                 string
		cached, five, hour float64
	}{
		{"moonshotai/kimi-k3.1-pro", .2, 2, 4},
		{"moonshotai/kimi-k3.1-pro-highspeed", .2, 4, 8},
		{"qwen/qwen4-max", .071, 1.767, 2.827},
		{"moonshotai/kimi-k3", .3, 3, 6},
	} {
		got := pricing.ResolveEffectivePricingForUsage(tc.id, 500000, 1, &novita.Adaptor{})
		require.InDelta(t, tc.cached, got.CachedInputRatio/ratio.MilliTokensUsd, 1e-12, tc.id)
		require.InDelta(t, tc.five, got.CacheWrite5mRatio/ratio.MilliTokensUsd, 1e-12, tc.id)
		require.InDelta(t, tc.hour, got.CacheWrite1hRatio/ratio.MilliTokensUsd, 1e-12, tc.id)
	}
	for _, id := range []string{"apodex/apodex-1.1-mini", "bunny", "nvrouter/auto", "nvrouter/fusion", "dev/glm46"} {
		require.NotContains(t, (&novita.Adaptor{}).GetModelList(), id)
	}
	require.Positive(t, novita.ModelRatios["inclusionai/ling-3.1-flash"].Ratio)
	require.Empty(t, novita.ModelRatios["moonshotai/kimi-k3.1-pro"].SupportedReasoningEfforts, "features do not establish an effort vocabulary")
}
