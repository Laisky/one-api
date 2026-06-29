package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestModelPriceConfigsTimeWindowsRoundTrip verifies time windows survive Set/Get normalization.
// Parameters: t is the current test handle.
// Returns: nothing; assertions fail the test on mismatch.
func TestModelPriceConfigsTimeWindowsRoundTrip(t *testing.T) {
	channel := &Channel{}
	err := channel.SetModelPriceConfigs(map[string]ModelConfigLocal{
		"deepseek-reasoner": {
			Ratio:           1,
			CompletionRatio: 4,
			Embedding:       &EmbeddingPricingLocal{TextTokenRatio: 0.7},
			TimeWindows: []TimeWindowLocal{{
				Name:       "deepseek-offpeak",
				TimeZone:   "Asia/Shanghai",
				DaysOfWeek: []int{1, 2},
				DateFrom:   "2026-06-01",
				DateTo:     "2026-07-01",
				Ranges:     []ClockRangeLocal{{Start: "00:30", End: "08:30"}},
				Overlay: ModelConfigLocal{
					Ratio:             0.25,
					CachedInputRatio:  -1,
					CacheWrite5mRatio: -1,
					CacheWrite1hRatio: -1,
				},
			}},
		},
	})
	require.NoError(t, err)

	configs := channel.GetModelPriceConfigs()
	cfg := configs["deepseek-reasoner"]
	require.NotNil(t, cfg.Embedding)
	require.InDelta(t, 0.7, cfg.Embedding.TextTokenRatio, 1e-12)
	require.Len(t, cfg.TimeWindows, 1)
	require.Equal(t, "deepseek-offpeak", cfg.TimeWindows[0].Name)
	require.Equal(t, "Asia/Shanghai", cfg.TimeWindows[0].TimeZone)
	require.Equal(t, []int{1, 2}, cfg.TimeWindows[0].DaysOfWeek)
	require.InDelta(t, 0.25, cfg.TimeWindows[0].Overlay.Ratio, 1e-12)
	require.InDelta(t, -1.0, cfg.TimeWindows[0].Overlay.CachedInputRatio, 1e-12)
	require.InDelta(t, -1.0, cfg.TimeWindows[0].Overlay.CacheWrite5mRatio, 1e-12)
	require.InDelta(t, -1.0, cfg.TimeWindows[0].Overlay.CacheWrite1hRatio, 1e-12)
}

// TestModelPriceConfigsOldJSONCompatibility verifies old rows decode with nil time windows.
// Parameters: t is the current test handle.
// Returns: nothing; assertions fail the test on mismatch.
func TestModelPriceConfigsOldJSONCompatibility(t *testing.T) {
	raw := `{"gpt-4":{"ratio":0.03,"completion_ratio":2}}`
	channel := &Channel{ModelConfigs: &raw}
	configs := channel.GetModelPriceConfigs()
	require.NotNil(t, configs)
	require.Nil(t, configs["gpt-4"].TimeWindows)
}

// TestModelPriceConfigsWindowlessJSONStable verifies adding TimeWindows preserves window-less JSON.
// Parameters: t is the current test handle.
// Returns: nothing; assertions fail the test on mismatch.
func TestModelPriceConfigsWindowlessJSONStable(t *testing.T) {
	cfg := ModelConfigLocal{Ratio: 0.03, CompletionRatio: 2}
	got, err := json.Marshal(cfg)
	require.NoError(t, err)
	require.Equal(t, `{"ratio":0.03,"completion_ratio":2}`, string(got))
	require.JSONEq(t, `{"ratio":0.03,"completion_ratio":2}`, string(got))
	require.NotContains(t, string(got), "time_windows")
}

// TestModelPriceConfigsRejectMalformedTimeWindows verifies save-time validation failures.
// Parameters: t is the current test handle.
// Returns: nothing; assertions fail the test on mismatch.
func TestModelPriceConfigsRejectMalformedTimeWindows(t *testing.T) {
	tests := []struct {
		name string
		cfg  ModelConfigLocal
	}{
		{
			name: "bad timezone",
			cfg:  windowedConfig(TimeWindowLocal{TimeZone: "Mars/Base", Ranges: []ClockRangeLocal{{Start: "00:00", End: "01:00"}}, Overlay: ModelConfigLocal{Ratio: 0.5}}),
		},
		{
			name: "bad clock",
			cfg:  windowedConfig(TimeWindowLocal{TimeZone: "UTC", Ranges: []ClockRangeLocal{{Start: "24:00", End: "01:00"}}, Overlay: ModelConfigLocal{Ratio: 0.5}}),
		},
		{
			name: "empty ranges",
			cfg:  windowedConfig(TimeWindowLocal{TimeZone: "UTC", Overlay: ModelConfigLocal{Ratio: 0.5}}),
		},
		{
			name: "bad day",
			cfg:  windowedConfig(TimeWindowLocal{TimeZone: "UTC", DaysOfWeek: []int{7}, Ranges: []ClockRangeLocal{{Start: "00:00", End: "01:00"}}, Overlay: ModelConfigLocal{Ratio: 0.5}}),
		},
		{
			name: "bad date range",
			cfg:  windowedConfig(TimeWindowLocal{TimeZone: "UTC", DateFrom: "2026-07-01", DateTo: "2026-07-01", Ranges: []ClockRangeLocal{{Start: "00:00", End: "01:00"}}, Overlay: ModelConfigLocal{Ratio: 0.5}}),
		},
		{
			name: "empty overlay",
			cfg:  windowedConfig(TimeWindowLocal{TimeZone: "UTC", Ranges: []ClockRangeLocal{{Start: "00:00", End: "01:00"}}}),
		},
		{
			name: "recursive overlay",
			cfg: windowedConfig(TimeWindowLocal{
				TimeZone: "UTC",
				Ranges:   []ClockRangeLocal{{Start: "00:00", End: "01:00"}},
				Overlay: ModelConfigLocal{
					Ratio:       0.5,
					TimeWindows: []TimeWindowLocal{{TimeZone: "UTC", Ranges: []ClockRangeLocal{{Start: "00:00", End: "01:00"}}, Overlay: ModelConfigLocal{Ratio: 0.1}}},
				},
			}),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			channel := &Channel{}
			err := channel.SetModelPriceConfigs(map[string]ModelConfigLocal{"model": tc.cfg})
			require.Error(t, err)
		})
	}
}

// windowedConfig builds a valid base config with one supplied time window.
// Parameters: window is the time-window configuration to attach.
// Returns: a ModelConfigLocal suitable for validation tests.
func windowedConfig(window TimeWindowLocal) ModelConfigLocal {
	return ModelConfigLocal{
		Ratio:       1,
		TimeWindows: []TimeWindowLocal{window},
	}
}
