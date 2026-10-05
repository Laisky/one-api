package model

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPerPagePricingConfig validates paid and explicitly free per-page tariffs
// at top level and within a time window, including serialization and isolation,
// and rejects invalid prices.
func TestPerPagePricingConfig(t *testing.T) {
	for _, price := range []float64{0, 4.5} {
		for _, window := range []bool{false, true} {
			cfg := ModelConfigLocal{PerPage: &PerPagePricingLocal{UsdPerThousandPages: price}}
			if window {
				cfg = ModelConfigLocal{TimeWindows: []TimeWindowLocal{{TimeZone: "UTC", Ranges: []ClockRangeLocal{{Start: "00:00", End: "23:59"}}, Overlay: cfg}}}
			}
			channel := &Channel{}
			require.NoError(t, channel.SetModelPriceConfigs(map[string]ModelConfigLocal{"glm-ocr": cfg}))
			got := channel.GetModelPriceConfigs()["glm-ocr"]
			if window {
				got = got.TimeWindows[0].Overlay
			}
			require.NotNil(t, got.PerPage)
			require.Equal(t, price, got.PerPage.UsdPerThousandPages)
			got.PerPage.UsdPerThousandPages = 99
			again := channel.GetModelPriceConfigs()["glm-ocr"]
			if window {
				again = again.TimeWindows[0].Overlay
			}
			require.Equal(t, price, again.PerPage.UsdPerThousandPages)
		}
	}
	for _, price := range []float64{-1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		cfg := ModelConfigLocal{PerPage: &PerPagePricingLocal{UsdPerThousandPages: price}}
		require.Error(t, (&Channel{}).validateModelPriceConfigs(map[string]ModelConfigLocal{"glm-ocr": cfg}))
		require.Error(t, validateTimeWindowOverlayLocal(cfg, "glm-ocr", 0))
		require.Error(t, (&Channel{}).SetModelPriceConfigs(map[string]ModelConfigLocal{"glm-ocr": cfg}))
	}
}

// TestPerPageAndPerCallAreDisjoint verifies a model cannot carry both flat
// unit tariffs, directly or by combining its base tariff with a window overlay.
func TestPerPageAndPerCallAreDisjoint(t *testing.T) {
	window := func(overlay ModelConfigLocal) []TimeWindowLocal {
		return []TimeWindowLocal{{TimeZone: "UTC", Ranges: []ClockRangeLocal{{Start: "00:00", End: "12:00"}}, Overlay: overlay}}
	}
	for name, cfg := range map[string]ModelConfigLocal{
		"same_level": {PerCall: &PerCallPricingLocal{UsdPerThousandCalls: 1}, PerPage: &PerPagePricingLocal{UsdPerThousandPages: 1}},
		"base_call_window_page": {PerCall: &PerCallPricingLocal{UsdPerThousandCalls: 1},
			TimeWindows: window(ModelConfigLocal{PerPage: &PerPagePricingLocal{UsdPerThousandPages: 1}})},
		"base_page_window_call": {PerPage: &PerPagePricingLocal{UsdPerThousandPages: 1},
			TimeWindows: window(ModelConfigLocal{PerCall: &PerCallPricingLocal{UsdPerThousandCalls: 1}})},
	} {
		t.Run(name, func(t *testing.T) {
			err := (&Channel{}).SetModelPriceConfigs(map[string]ModelConfigLocal{"glm-ocr": cfg})
			require.ErrorContains(t, err, "both per_call and per_page")
		})
	}
}
