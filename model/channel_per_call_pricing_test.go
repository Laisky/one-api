package model

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestProtocolAuditPerCallConfig validates paid and explicitly free overrides at
// both top level and within a time window, including serialization and isolation.
func TestProtocolAuditPerCallConfig(t *testing.T) {
	for _, price := range []float64{0, 12.5} {
		for _, window := range []bool{false, true} {
			cfg := ModelConfigLocal{PerCall: &PerCallPricingLocal{UsdPerThousandCalls: price}}
			if window {
				cfg = ModelConfigLocal{TimeWindows: []TimeWindowLocal{{TimeZone: "UTC", Ranges: []ClockRangeLocal{{Start: "00:00", End: "23:59"}}, Overlay: cfg}}}
			}
			channel := &Channel{}
			require.NoError(t, channel.SetModelPriceConfigs(map[string]ModelConfigLocal{"video": cfg}))
			got := channel.GetModelPriceConfigs()["video"]
			if window {
				got = got.TimeWindows[0].Overlay
			}
			require.NotNil(t, got.PerCall)
			require.Equal(t, price, got.PerCall.UsdPerThousandCalls)
			got.PerCall.UsdPerThousandCalls = 99
			again := channel.GetModelPriceConfigs()["video"]
			if window {
				again = again.TimeWindows[0].Overlay
			}
			require.Equal(t, price, again.PerCall.UsdPerThousandCalls)
		}
	}
	for _, price := range []float64{-1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		cfg := ModelConfigLocal{PerCall: &PerCallPricingLocal{UsdPerThousandCalls: price}}
		require.Error(t, (&Channel{}).validateModelPriceConfigs(map[string]ModelConfigLocal{"video": cfg}))
		require.Error(t, validateTimeWindowOverlayLocal(cfg, "video", 0))
		require.Error(t, (&Channel{}).SetModelPriceConfigs(map[string]ModelConfigLocal{"video": cfg}))
	}
}
