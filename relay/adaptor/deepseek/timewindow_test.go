package deepseek

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/Laisky/one-api/relay/quota"
)

// shanghai loads the Asia/Shanghai location used by DeepSeek's peak schedule.
func shanghai(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	return loc
}

// TestDeepSeekModelsCarryPeakWindow verifies every DeepSeek V4 model ships the
// peak overlay: base ratios are the published list (= off-peak) price and the
// window doubles input + cache-hit input during peak hours.
func TestDeepSeekModelsCarryPeakWindow(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"deepseek-chat", "deepseek-reasoner", "deepseek-v4-flash", "deepseek-v4-pro"} {
		cfg, ok := ModelRatios[name]
		require.True(t, ok, "model %s missing from ModelRatios", name)
		require.Len(t, cfg.TimeWindows, 1, "model %s should carry exactly one peak window", name)
		window := cfg.TimeWindows[0]
		require.Equal(t, "deepseek-peak", window.Name)
		require.Equal(t, "Asia/Shanghai", window.TimeZone)
		require.Equal(t, deepseekPeakEffectiveFrom, window.DateFrom, "peak surcharge must be gated to the V4 official release")
		require.Len(t, window.Ranges, 2, "peak window should cover exactly the two published peak spans")
		// Overlay must double input + cache-hit input and inherit completion ratio.
		require.InDelta(t, cfg.Ratio*2, window.Overlay.Ratio, 1e-15)
		require.InDelta(t, cfg.CachedInputRatio*2, window.Overlay.CachedInputRatio, 1e-15)
		require.Zero(t, window.Overlay.CompletionRatio, "completion ratio must inherit (0 == inherit)")
	}
}

// TestDeepSeekPeakPricingResolution exercises the full resolver path: once the
// surcharge is effective, a request at a peak instant bills at exactly 2x the
// list price for input, cache-hit input, and output, while off-peak instants
// bill at the unmodified list price.
func TestDeepSeekPeakPricingResolution(t *testing.T) {
	t.Parallel()

	loc := shanghai(t)
	provider := &Adaptor{}

	for _, name := range []string{"deepseek-v4-flash", "deepseek-v4-pro"} {
		base := ModelRatios[name]

		cases := []struct {
			label string
			at    time.Time
			peak  bool
		}{
			{"peak-morning-10:00", time.Date(2026, 7, 20, 10, 0, 0, 0, loc), true},
			{"peak-afternoon-16:00", time.Date(2026, 7, 20, 16, 0, 0, 0, loc), true},
			{"peak-edge-09:00", time.Date(2026, 7, 20, 9, 0, 0, 0, loc), true},
			{"offpeak-night-03:00", time.Date(2026, 7, 20, 3, 0, 0, 0, loc), false},
			{"offpeak-noon-13:00", time.Date(2026, 7, 20, 13, 0, 0, 0, loc), false},
			{"offpeak-evening-20:00", time.Date(2026, 7, 20, 20, 0, 0, 0, loc), false},
			{"offpeak-edge-18:00", time.Date(2026, 7, 20, 18, 0, 0, 0, loc), false},
			{"offpeak-edge-12:00", time.Date(2026, 7, 20, 12, 0, 0, 0, loc), false},
			// Before the effective date the peak hours still bill flat list.
			{"pregate-peak-10:00", time.Date(2026, 7, 10, 10, 0, 0, 0, loc), false},
			{"pregate-peak-16:00", time.Date(2026, 7, 10, 16, 0, 0, 0, loc), false},
		}

		for _, tc := range cases {
			tc := tc
			t.Run(name+"/"+tc.label, func(t *testing.T) {
				t.Parallel()
				cfg, ok := pricing.ResolveModelConfig(name, nil, provider, tc.at)
				require.True(t, ok)

				wantInput := base.Ratio
				wantCached := base.CachedInputRatio
				if tc.peak {
					wantInput = base.Ratio * 2
					wantCached = base.CachedInputRatio * 2
					// A merged (in-window) config clears TimeWindows so it can never re-apply.
					require.Empty(t, cfg.TimeWindows, "merged peak config must not re-expose windows")
				}
				require.InDelta(t, wantInput, cfg.Ratio, 1e-15, "input ratio")
				require.InDelta(t, wantCached, cfg.CachedInputRatio, 1e-15, "cache-hit input ratio")
				// Completion ratio is inherited, so output (= Ratio*CompletionRatio) tracks the surcharge.
				require.InDelta(t, base.CompletionRatio, cfg.CompletionRatio, 1e-15, "completion ratio inherited")
				wantOutput := wantInput * base.CompletionRatio
				require.InDelta(t, wantOutput, cfg.Ratio*cfg.CompletionRatio, 1e-15, "output price")
			})
		}
	}
}

// TestDeepSeekPeakMatchesAdaptorWindow sanity-checks the raw window matcher at a
// couple of instants, independent of the merge path.
func TestDeepSeekPeakMatchesAdaptorWindow(t *testing.T) {
	t.Parallel()

	loc := shanghai(t)
	window := ModelRatios["deepseek-v4-pro"].TimeWindows[0]

	matchAt := func(h, m int) bool {
		return pricing.MatchTimeWindow(window, time.Date(2026, 7, 20, h, m, 0, 0, loc))
	}

	// Peak instants (ranges are [start, end)).
	require.True(t, matchAt(9, 0))
	require.True(t, matchAt(11, 59))
	require.True(t, matchAt(14, 0))
	require.True(t, matchAt(17, 59))
	// Off-peak instants.
	require.False(t, matchAt(0, 0))
	require.False(t, matchAt(8, 59))
	require.False(t, matchAt(12, 0))
	require.False(t, matchAt(12, 30))
	require.False(t, matchAt(13, 59))
	require.False(t, matchAt(18, 0))
	require.False(t, matchAt(23, 59))

	// DateFrom gate: the same peak instant does not match before the effective date.
	require.False(t, pricing.MatchTimeWindow(window, time.Date(2026, 7, 10, 10, 0, 0, 0, loc)))
	require.True(t, pricing.MatchTimeWindow(window, time.Date(2026, 7, 15, 10, 0, 0, 0, loc)))
}

// TestDeepSeekIncidentRegression20260708 replays the 2026-07-08 under-billing
// incident through the production quota path. That day a user pushed 15.27M
// prompt tokens (93.8% cache-hit) through deepseek-reasoner (= V4-Flash) during
// Beijing off-peak hours; the inverted peak/valley polarity billed ~51.6k quota
// while DeepSeek's real off-peak charge is the flat list price (~103.1k quota,
// $0.2063 at 500k quota/USD). The corrected table must bill list price at
// off-peak instants (and before the surcharge activates), and exactly 2x at
// post-activation peak instants.
func TestDeepSeekIncidentRegression20260708(t *testing.T) {
	t.Parallel()

	loc := shanghai(t)
	provider := &Adaptor{}
	usage := &relaymodel.Usage{
		PromptTokens:     15265227,
		CompletionTokens: 123823,
		PromptTokensDetails: &relaymodel.UsagePromptTokensDetails{
			CachedTokens: 14325888,
		},
	}

	compute := func(at time.Time) quota.ComputeResult {
		return quota.Compute(quota.ComputeInput{
			Usage:          usage,
			ModelName:      "deepseek-reasoner",
			ModelRatio:     ModelRatios["deepseek-reasoner"].Ratio,
			GroupRatio:     1,
			PricingAdaptor: provider,
			RequestTime:    at,
		})
	}

	// List price = 939,339 miss * $0.14/M + 14,325,888 hit * $0.0028/M
	// + 123,823 out * $0.28/M = $0.20629, i.e. 103,146 quota (ceil).
	const listQuota = int64(103146)

	// The incident window: off-peak instant (03:00 Beijing) after activation.
	require.Equal(t, listQuota, compute(time.Date(2026, 7, 20, 3, 0, 0, 0, loc)).TotalQuota,
		"off-peak must bill the flat list price, not 50% of it")
	// Before the surcharge activates, even peak hours bill flat list.
	require.Equal(t, listQuota, compute(time.Date(2026, 7, 8, 10, 0, 0, 0, loc)).TotalQuota,
		"pre-activation peak hours must bill the flat list price")
	// After activation, peak hours bill exactly 2x every line item.
	require.Equal(t, int64(206291), compute(time.Date(2026, 7, 20, 10, 0, 0, 0, loc)).TotalQuota,
		"post-activation peak hours must bill 2x list")
}
