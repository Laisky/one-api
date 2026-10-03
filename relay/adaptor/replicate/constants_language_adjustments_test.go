package replicate_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/replicate"
	ratio "github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/pricing"
)

// TestGPT56SolPromotionWindow verifies that the temporary Replicate discount is
// active through 2026-09-18 and that long-lived base pricing resumes on
// 2026-09-19. The t parameter manages the test lifecycle and assertions. This
// function returns no values.
func TestGPT56SolPromotionWindow(t *testing.T) {
	t.Parallel()

	config, ok := replicate.ModelRatios["openai/gpt-5.6-sol"]
	require.True(t, ok)
	require.Len(t, config.TimeWindows, 1)

	active := pricing.ApplyTimeWindowRatioOnly(config, time.Date(2026, time.September, 18, 23, 59, 0, 0, time.UTC))
	require.InDelta(t, 2.50*ratio.MilliTokensUsd, active.Ratio, 1e-12)
	require.InDelta(t, 0.25*ratio.MilliTokensUsd, active.CachedInputRatio, 1e-12)
	require.InDelta(t, 15.00/2.50, active.CompletionRatio, 1e-12)

	expired := pricing.ApplyTimeWindowRatioOnly(config, time.Date(2026, time.September, 19, 0, 0, 0, 0, time.UTC))
	require.InDelta(t, 5.00*ratio.MilliTokensUsd, expired.Ratio, 1e-12)
	require.InDelta(t, 0.50*ratio.MilliTokensUsd, expired.CachedInputRatio, 1e-12)
	require.InDelta(t, 30.00/5.00, expired.CompletionRatio, 1e-12)
}
