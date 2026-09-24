package deepseek

import (
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

// TestProContinuesAfterSeptember14 checks the revised official service notice
// through both price resolvers. Pro never inherits the cheaper Flash tariff.
// Source: https://api-docs.deepseek.com/updates/ (verified 2026-09-24).
func TestProContinuesAfterSeptember14(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		at                    string
		input, cached, output float64
	}{
		{"2026-09-14T03:59:59.999999999Z", 1.32, .044, 3.96},
		{"2026-09-14T04:00:00Z", .66, .022, 1.98},
		{"2026-09-14T06:00:00Z", 1.32, .044, 3.96},
		{"2026-09-24T12:00:00Z", .66, .022, 1.98},
		{"2026-09-26T06:00:00Z", .66, .022, 1.98},
	} {
		at, err := time.Parse(time.RFC3339Nano, tc.at)
		require.NoError(t, err)
		for _, got := range []float64{
			pricing.ApplyTimeWindow(ModelRatios["deepseek-v4-pro"], at).Ratio,
			pricing.ApplyTimeWindowRatioOnly(ModelRatios["deepseek-v4-pro"], at).Ratio,
		} {
			require.InDelta(t, tc.input, got/ratio.MilliTokensUsd, 1e-12, tc.at)
		}
		got := pricing.ApplyTimeWindow(ModelRatios["deepseek-v4-pro"], at)
		require.InDelta(t, tc.cached, got.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
		require.InDelta(t, tc.output, got.Ratio*got.CompletionRatio/ratio.MilliTokensUsd, 1e-12)
	}
}
