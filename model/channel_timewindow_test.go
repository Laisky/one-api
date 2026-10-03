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
					CacheWrite5mRatio: 0.125,
					CacheWrite1hRatio: 0.15,
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
	require.InDelta(t, 0.125, cfg.TimeWindows[0].Overlay.CacheWrite5mRatio, 1e-12)
	require.InDelta(t, 0.15, cfg.TimeWindows[0].Overlay.CacheWrite1hRatio, 1e-12)
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

// TestModelPriceConfigsOldBinaryDropsOnlyTimeWindows simulates an old binary
// that does not know the TimeWindows field and verifies additive rollback safety.
// Parameters: t is the current test handle.
// Returns: nothing; assertions fail the test on mismatch.
func TestModelPriceConfigsOldBinaryDropsOnlyTimeWindows(t *testing.T) {
	type oldModelConfigLocal struct {
		Ratio             float64                `json:"ratio"`
		CompletionRatio   float64                `json:"completion_ratio,omitempty"`
		CachedInputRatio  float64                `json:"cached_input_ratio,omitempty"`
		CacheWrite5mRatio float64                `json:"cache_write_5m_ratio,omitempty"`
		CacheWrite1hRatio float64                `json:"cache_write_1h_ratio,omitempty"`
		Tiers             []ModelRatioTierLocal  `json:"tiers,omitempty"`
		MaxTokens         int32                  `json:"max_tokens,omitempty"`
		Video             *VideoPricingLocal     `json:"video,omitempty"`
		Audio             *AudioPricingLocal     `json:"audio,omitempty"`
		Image             *ImagePricingLocal     `json:"image,omitempty"`
		Embedding         *EmbeddingPricingLocal `json:"embedding,omitempty"`
	}

	raw := []byte(`{"ratio":0.03,"completion_ratio":2,"cached_input_ratio":0.01,"max_tokens":8192,"embedding":{"text_token_ratio":0.5},"time_windows":[{"name":"night","ranges":[{"start":"00:00","end":"06:00"}],"overlay":{"ratio":0.01}}]}`)
	var decoded oldModelConfigLocal
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.InDelta(t, 0.03, decoded.Ratio, 1e-12)
	require.InDelta(t, 2.0, decoded.CompletionRatio, 1e-12)
	require.NotNil(t, decoded.Embedding)

	encoded, err := json.Marshal(decoded)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "time_windows")
	require.JSONEq(t, `{"ratio":0.03,"completion_ratio":2,"cached_input_ratio":0.01,"max_tokens":8192,"embedding":{"text_token_ratio":0.5}}`, string(encoded))
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
			name: "negative overlay cache write",
			cfg: windowedConfig(TimeWindowLocal{
				TimeZone: "UTC",
				Ranges:   []ClockRangeLocal{{Start: "00:00", End: "01:00"}},
				Overlay:  ModelConfigLocal{CacheWrite5mRatio: -1},
			}),
		},
		{
			name: "negative overlay tier cache write",
			cfg: windowedConfig(TimeWindowLocal{
				TimeZone: "UTC",
				Ranges:   []ClockRangeLocal{{Start: "00:00", End: "01:00"}},
				Overlay:  ModelConfigLocal{Tiers: []ModelRatioTierLocal{{InputTokenThreshold: 1, CacheWrite1hRatio: -1}}},
			}),
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
