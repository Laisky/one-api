package pricing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
)

// TestApplyTimeWindowOverlayMatchesFirstMatch verifies that merging a window's
// overlay directly yields exactly the configuration ApplyTimeWindow returns
// while that window is the first match, for scalars, free (negative) prices,
// tiers and media blocks, and that neither input is mutated. Parameter t
// reports any divergence a price display would inherit.
func TestApplyTimeWindowOverlayMatchesFirstMatch(t *testing.T) {
	t.Parallel()

	windows := []adaptor.TimeWindow{
		{
			Name:     "night",
			TimeZone: "Asia/Shanghai",
			Ranges:   []adaptor.ClockRange{{Start: "22:00", End: "06:00"}},
			Overlay: adaptor.ModelConfig{
				Ratio:            3,
				CachedInputRatio: -1,
				Tiers:            []adaptor.ModelRatioTier{{Ratio: 6, InputTokenThreshold: 1000}},
				Image:            &adaptor.ImagePricingConfig{PricePerImageUsd: 0.5},
				PerCall:          &adaptor.PerCallPricingConfig{UsdPerThousandCalls: 0.1},
			},
		},
		{
			Name:     "dated",
			TimeZone: "UTC",
			DateFrom: "2026-12-01",
			Ranges:   []adaptor.ClockRange{{Start: "00:00", End: "00:00"}},
			Overlay:  adaptor.ModelConfig{CompletionRatio: 9, CacheWrite5mRatio: 4},
		},
	}
	base := adaptor.ModelConfig{
		Ratio: 2, CompletionRatio: 5, CachedInputRatio: 1, CacheWrite5mRatio: 2.5,
		Tiers:       []adaptor.ModelRatioTier{{Ratio: 4, InputTokenThreshold: 500}},
		Image:       &adaptor.ImagePricingConfig{PricePerImageUsd: 0.2, DefaultSize: "1024x1024"},
		PerCall:     &adaptor.PerCallPricingConfig{UsdPerThousandCalls: 0.3},
		TimeWindows: windows,
	}
	unwindowed := base.Clone()
	unwindowed.TimeWindows = nil

	for index, at := range map[int]time.Time{
		0: time.Date(2026, time.December, 2, 16, 0, 0, 0, time.UTC), // 00:00 Shanghai: night shadows dated
		1: time.Date(2026, time.December, 2, 4, 0, 0, 0, time.UTC),  // 12:00 Shanghai: only dated matches
	} {
		want := ApplyTimeWindow(base, at)
		require.Nil(t, want.TimeWindows, windows[index].Name)
		require.Equal(t, want, ApplyTimeWindowOverlay(unwindowed, windows[index].Overlay), windows[index].Name)
	}
	require.Equal(t, 2.0, base.Ratio, "the base must not be mutated")
	require.Equal(t, 0.2, base.Image.PricePerImageUsd, "the base media block must not be mutated")
	require.Len(t, base.TimeWindows, 2)
}
