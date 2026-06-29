package quota

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestComputeTimeWindowDeepSeekFixture verifies request-start time selects DeepSeek-style off-peak billing.
// Parameters: t is the current test handle.
// Returns: nothing; assertions fail the test on mismatch.
func TestComputeTimeWindowDeepSeekFixture(t *testing.T) {
	shanghai := mustLoadLocation(t, "Asia/Shanghai")
	configs := map[string]model.ModelConfigLocal{
		"deepseek-reasoner": {
			Ratio:            1,
			CompletionRatio:  4,
			CachedInputRatio: 0.2,
			TimeWindows: []model.TimeWindowLocal{{
				Name:     "deepseek-offpeak",
				TimeZone: "Asia/Shanghai",
				Ranges:   []model.ClockRangeLocal{{Start: "00:30", End: "08:30"}},
				Overlay: model.ModelConfigLocal{
					Ratio:            0.25,
					CachedInputRatio: 0.05,
				},
			}},
		},
	}
	usage := &relaymodel.Usage{
		PromptTokens:     1000,
		CompletionTokens: 50,
		PromptTokensDetails: &relaymodel.UsagePromptTokensDetails{
			CachedTokens: 100,
		},
	}

	offpeak := Compute(ComputeInput{
		Usage:               usage,
		ModelName:           "deepseek-reasoner",
		ModelRatio:          1,
		GroupRatio:          1,
		ChannelModelConfigs: configs,
		RequestTime:         time.Date(2026, 6, 29, 3, 0, 0, 0, shanghai),
	})
	require.Equal(t, int64(280), offpeak.TotalQuota)
	require.InDelta(t, 0.25, offpeak.UsedModelRatio, 1e-12)
	require.InDelta(t, 4.0, offpeak.UsedCompletionRatio, 1e-12)

	peak := Compute(ComputeInput{
		Usage:               usage,
		ModelName:           "deepseek-reasoner",
		ModelRatio:          1,
		GroupRatio:          1,
		ChannelModelConfigs: configs,
		RequestTime:         time.Date(2026, 6, 29, 12, 0, 0, 0, shanghai),
	})
	require.Equal(t, int64(1120), peak.TotalQuota)
	require.InDelta(t, 1.0, peak.UsedModelRatio, 1e-12)
}

// TestComputeTimeWindowUsesStartTime verifies boundary-crossing streams keep one rate.
// Parameters: t is the current test handle.
// Returns: nothing; assertions fail the test on mismatch.
func TestComputeTimeWindowUsesStartTime(t *testing.T) {
	shanghai := mustLoadLocation(t, "Asia/Shanghai")
	configs := map[string]model.ModelConfigLocal{
		"stream-model": {
			Ratio:           1,
			CompletionRatio: 2,
			TimeWindows: []model.TimeWindowLocal{{
				TimeZone: "Asia/Shanghai",
				Ranges:   []model.ClockRangeLocal{{Start: "00:30", End: "08:30"}},
				Overlay:  model.ModelConfigLocal{Ratio: 0.25},
			}},
		},
	}
	usage := &relaymodel.Usage{PromptTokens: 100, CompletionTokens: 10}
	startInside := time.Date(2026, 6, 29, 8, 29, 0, 0, shanghai)
	startOutside := time.Date(2026, 6, 29, 8, 30, 0, 0, shanghai)

	inside := Compute(ComputeInput{
		Usage:               usage,
		ModelName:           "stream-model",
		ModelRatio:          1,
		GroupRatio:          1,
		ChannelModelConfigs: configs,
		RequestTime:         startInside,
	})
	outside := Compute(ComputeInput{
		Usage:               usage,
		ModelName:           "stream-model",
		ModelRatio:          1,
		GroupRatio:          1,
		ChannelModelConfigs: configs,
		RequestTime:         startOutside,
	})

	require.Equal(t, int64(30), inside.TotalQuota)
	require.Equal(t, int64(120), outside.TotalQuota)
}

// TestComputeTimeWindowFlatOverridePrecedence verifies legacy flat ratios keep base precedence.
// Parameters: t is the current test handle.
// Returns: nothing; assertions fail the test on mismatch.
func TestComputeTimeWindowFlatOverridePrecedence(t *testing.T) {
	configs := map[string]model.ModelConfigLocal{
		"flat-model": {
			Ratio:            1,
			CompletionRatio:  2,
			CachedInputRatio: 0.2,
			TimeWindows: []model.TimeWindowLocal{{
				TimeZone: "UTC",
				Ranges:   []model.ClockRangeLocal{{Start: "00:00", End: "00:00"}},
				Overlay: model.ModelConfigLocal{
					Ratio:            0.25,
					CachedInputRatio: 0.05,
				},
			}},
		},
	}
	result := Compute(ComputeInput{
		Usage:                  &relaymodel.Usage{PromptTokens: 100, CompletionTokens: 10, PromptTokensDetails: &relaymodel.UsagePromptTokensDetails{CachedTokens: 20}},
		ModelName:              "flat-model",
		ModelRatio:             5,
		ChannelModelRatio:      map[string]float64{"flat-model": 5},
		GroupRatio:             1,
		ChannelModelConfigs:    configs,
		ChannelCompletionRatio: map[string]float64{},
		RequestTime:            time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC),
	})

	require.Equal(t, int64(501), result.TotalQuota)
	require.InDelta(t, 5.0, result.UsedModelRatio, 1e-12)
	require.InDelta(t, 2.0, result.UsedCompletionRatio, 1e-12)
}

// TestComputeTimeWindowConfigDerivedRatios verifies model_configs-derived scalar maps still allow windows.
// Parameters: t is the current test handle.
// Returns: nothing; assertions fail the test on mismatch.
func TestComputeTimeWindowConfigDerivedRatios(t *testing.T) {
	configs := map[string]model.ModelConfigLocal{
		"config-derived-model": {
			Ratio:           2,
			CompletionRatio: 3,
			TimeWindows: []model.TimeWindowLocal{{
				TimeZone: "UTC",
				Ranges:   []model.ClockRangeLocal{{Start: "00:00", End: "00:00"}},
				Overlay:  model.ModelConfigLocal{Ratio: 0.5, CompletionRatio: 4},
			}},
		},
	}
	result := Compute(ComputeInput{
		Usage:                  &relaymodel.Usage{PromptTokens: 100, CompletionTokens: 10},
		ModelName:              "config-derived-model",
		ModelRatio:             2,
		ChannelModelRatio:      map[string]float64{"config-derived-model": 2},
		GroupRatio:             1,
		ChannelModelConfigs:    configs,
		ChannelCompletionRatio: map[string]float64{"config-derived-model": 3},
		RequestTime:            time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC),
	})

	require.Equal(t, int64(70), result.TotalQuota)
	require.InDelta(t, 0.5, result.UsedModelRatio, 1e-12)
	require.InDelta(t, 4.0, result.UsedCompletionRatio, 1e-12)
}

// mustLoadLocation loads an IANA timezone for quota tests.
// Parameters: t is the current test handle and name is the IANA timezone name.
// Returns: the loaded location, failing the test if the timezone cannot load.
func mustLoadLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	require.NoError(t, err)
	return loc
}
