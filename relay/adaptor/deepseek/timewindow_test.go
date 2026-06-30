package deepseek

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/pricing"
)

// shanghai loads the Asia/Shanghai location used by DeepSeek's peak schedule.
func shanghai(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	return loc
}

// TestDeepSeekModelsCarryOffPeakWindow verifies every DeepSeek V4 model ships the
// off-peak overlay so the adaptor default actually exposes time-of-day pricing.
func TestDeepSeekModelsCarryOffPeakWindow(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"deepseek-chat", "deepseek-reasoner", "deepseek-v4-flash", "deepseek-v4-pro"} {
		cfg, ok := ModelRatios[name]
		require.True(t, ok, "model %s missing from ModelRatios", name)
		require.Len(t, cfg.TimeWindows, 1, "model %s should carry exactly one off-peak window", name)
		window := cfg.TimeWindows[0]
		require.Equal(t, "deepseek-offpeak", window.Name)
		require.Equal(t, "Asia/Shanghai", window.TimeZone)
		require.Len(t, window.Ranges, 2, "off-peak window should cover the complement of the two peak spans")
		// Overlay must halve input + cache-hit input and inherit completion ratio.
		require.InDelta(t, cfg.Ratio*0.5, window.Overlay.Ratio, 1e-15)
		require.InDelta(t, cfg.CachedInputRatio*0.5, window.Overlay.CachedInputRatio, 1e-15)
		require.Zero(t, window.Overlay.CompletionRatio, "completion ratio must inherit (0 == inherit)")
	}
}

// TestDeepSeekOffPeakPricingResolution exercises the full resolver path: a request
// at a peak instant bills at the base (高峰) price, and an off-peak instant bills at
// exactly 50% for input, cache-hit input, and output.
func TestDeepSeekOffPeakPricingResolution(t *testing.T) {
	t.Parallel()

	loc := shanghai(t)
	provider := &Adaptor{}

	for _, name := range []string{"deepseek-v4-flash", "deepseek-v4-pro"} {
		base := ModelRatios[name]

		cases := []struct {
			label   string
			at      time.Time
			offPeak bool
		}{
			{"peak-morning-10:00", time.Date(2026, 7, 20, 10, 0, 0, 0, loc), false},
			{"peak-afternoon-16:00", time.Date(2026, 7, 20, 16, 0, 0, 0, loc), false},
			{"peak-edge-09:00", time.Date(2026, 7, 20, 9, 0, 0, 0, loc), false},
			{"offpeak-night-03:00", time.Date(2026, 7, 20, 3, 0, 0, 0, loc), true},
			{"offpeak-noon-13:00", time.Date(2026, 7, 20, 13, 0, 0, 0, loc), true},
			{"offpeak-evening-20:00", time.Date(2026, 7, 20, 20, 0, 0, 0, loc), true},
			{"offpeak-edge-18:00", time.Date(2026, 7, 20, 18, 0, 0, 0, loc), true},
		}

		for _, tc := range cases {
			tc := tc
			t.Run(name+"/"+tc.label, func(t *testing.T) {
				t.Parallel()
				cfg, ok := pricing.ResolveModelConfig(name, nil, provider, tc.at)
				require.True(t, ok)

				wantInput := base.Ratio
				wantCached := base.CachedInputRatio
				if tc.offPeak {
					wantInput = base.Ratio * 0.5
					wantCached = base.CachedInputRatio * 0.5
					// A merged (in-window) config clears TimeWindows so it can never re-apply.
					require.Empty(t, cfg.TimeWindows, "merged off-peak config must not re-expose windows")
				}
				require.InDelta(t, wantInput, cfg.Ratio, 1e-15, "input ratio")
				require.InDelta(t, wantCached, cfg.CachedInputRatio, 1e-15, "cache-hit input ratio")
				// Completion ratio is inherited, so output (= Ratio*CompletionRatio) tracks the discount.
				require.InDelta(t, base.CompletionRatio, cfg.CompletionRatio, 1e-15, "completion ratio inherited")
				wantOutput := wantInput * base.CompletionRatio
				require.InDelta(t, wantOutput, cfg.Ratio*cfg.CompletionRatio, 1e-15, "output price")
			})
		}
	}
}

// TestDeepSeekOffPeakMatchesAdaptorWindow sanity-checks the raw window matcher at a
// couple of instants, independent of the merge path.
func TestDeepSeekOffPeakMatchesAdaptorWindow(t *testing.T) {
	t.Parallel()

	loc := shanghai(t)
	window := ModelRatios["deepseek-v4-pro"].TimeWindows[0]

	matchAt := func(h, m int) bool {
		return pricing.MatchTimeWindow(window, time.Date(2026, 7, 20, h, m, 0, 0, loc))
	}

	// Off-peak instants.
	require.True(t, matchAt(0, 0))
	require.True(t, matchAt(8, 59))
	require.True(t, matchAt(12, 30))
	require.True(t, matchAt(23, 59))
	// Peak instants.
	require.False(t, matchAt(9, 0))
	require.False(t, matchAt(11, 59))
	require.False(t, matchAt(14, 0))
	require.False(t, matchAt(17, 59))
}
