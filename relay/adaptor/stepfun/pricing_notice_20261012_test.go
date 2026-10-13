package stepfun_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/stepfun"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/pricing"
)

// TestStep5GrandfatheredCacheNotice checks the account-dependent price notice
// without applying a universal increase to existing accounts. It accepts t.
func TestStep5GrandfatheredCacheNotice(t *testing.T) {
	t.Parallel()
	a := &stepfun.Adaptor{}
	cfg, exists := a.GetDefaultModelPricing()["step-5-preview"]
	require.True(t, exists)
	require.Contains(t, cfg.Description, "2026-10-14 00:30 Asia/Shanghai")
	require.Contains(t, cfg.Description, "2026-10-13 16:30 UTC")
	require.Contains(t, cfg.Description, "prior pay-as-you-go Step 5 usage")
	require.Contains(t, cfg.Description, "configure 0.70")
	require.Empty(t, cfg.TimeWindows, "grandfathering cannot be inferred from the request date")
	for _, at := range []time.Time{
		time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 10, 13, 16, 30, 0, 0, time.UTC),
		time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
	} {
		resolved := pricing.ApplyTimeWindow(cfg, at)
		require.InDelta(t, 0.35, resolved.CachedInputRatio/ratio.MilliTokensRmb, 1e-12)
		require.InDelta(t, 7.0, resolved.Ratio/ratio.MilliTokensRmb, 1e-12)
		require.InDelta(t, 20.0, resolved.Ratio*resolved.CompletionRatio/ratio.MilliTokensRmb, 1e-12)
	}
}
