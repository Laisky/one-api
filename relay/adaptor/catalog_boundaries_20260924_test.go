package adaptor_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/ali"
	"github.com/Laisky/one-api/relay/adaptor/baiduv2"
	"github.com/Laisky/one-api/relay/adaptor/siliconflow"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/pricing"
)

// TestProviderTierEquality checks the actual price resolver on both sides of
// independently documented closed/open provider tier boundaries, using t.
func TestProviderTierEquality(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		config   adaptor.ModelConfig
		boundary int
	}{
		{"Ali strict", ali.ModelRatios["qwen3.5-27b"], 128001},
		{"SiliconFlow inclusive", siliconflow.ModelRatios["Qwen/Qwen3.5-27B"], 128000},
		{"Qianfan strict", baiduv2.ModelRatios["ERNIE-5.1"], 32001},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NotEmpty(t, tc.config.Tiers)
			require.Equal(t, tc.boundary, tc.config.Tiers[0].InputTokenThreshold)
			before := pricing.ResolveEffectivePricingFromConfig(tc.boundary-1, tc.config)
			at := pricing.ResolveEffectivePricingFromConfig(tc.boundary, tc.config)
			require.Equal(t, tc.config.Ratio, before.InputRatio)
			require.Equal(t, tc.config.Tiers[0].Ratio, at.InputRatio)
			require.NotEqual(t, before.InputRatio, at.InputRatio)
		})
	}
}

// TestQianfanHolidayAndClockBoundaries checks real time-window selection for all
// token classes, inclusive start/exclusive end, and both dayparts with t.
func TestQianfanHolidayAndClockBoundaries(t *testing.T) {
	t.Parallel()
	cfg := baiduv2.ModelRatios["DeepSeek-V4.1-Flash"]
	for _, tc := range []struct {
		at                    string
		input, cached, output float64
	}{
		{"2026-09-23T23:59:59+08:00", 1, .02, 4},
		{"2026-09-24T00:00:00+08:00", .6, .012, 2.4},
		{"2026-09-24T07:59:59+08:00", .6, .012, 2.4},
		{"2026-09-24T08:00:00+08:00", 1.2, .024, 4.8},
		{"2026-09-24T22:00:00+08:00", .6, .012, 2.4},
		{"2026-10-07T23:59:59+08:00", .6, .012, 2.4},
		{"2026-10-08T00:00:00+08:00", 1, .02, 4},
		{"2026-10-08T08:00:00+08:00", 2, .04, 8},
	} {
		at, err := time.Parse(time.RFC3339, tc.at)
		require.NoError(t, err)
		got := pricing.ApplyTimeWindow(cfg, at)
		require.InDelta(t, tc.input, got.Ratio/ratio.MilliTokensRmb, 1e-12, tc.at)
		require.InDelta(t, tc.cached, got.CachedInputRatio/ratio.MilliTokensRmb, 1e-12, tc.at)
		require.InDelta(t, tc.output, got.Ratio*got.CompletionRatio/ratio.MilliTokensRmb, 1e-12, tc.at)
	}
}
