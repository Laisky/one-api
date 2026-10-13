package deepseek_test

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/deepseek"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

// TestOctoberDeepSeekHolidayRates reproduces weekday holiday overcharging through
// both production resolvers. It accepts t and returns no values or network traffic.
func TestOctoberDeepSeekHolidayRates(t *testing.T) {
	t.Parallel()
	for _, date := range []string{"2026-09-25", "2026-09-26", "2026-09-27", "2026-10-01", "2026-10-02", "2026-10-03", "2026-10-04", "2026-10-05", "2026-10-06", "2026-10-07"} {
		t.Run(date, func(t *testing.T) {
			t.Parallel()
			day, err := time.Parse(time.DateOnly, date)
			require.NoError(t, err)
			for id, cfg := range (&deepseek.Adaptor{}).GetDefaultModelPricing() {
				original := cfg.Clone()
				input, cached, output := .15, .003, .6
				if id == "deepseek-v4-pro" {
					input, cached, output = .66, .022, 1.98
				}
				for _, hour := range []int{0, 1, 4, 6, 10, 23} {
					for _, offset := range []time.Duration{0, time.Nanosecond, time.Minute} {
						at := day.Add(time.Duration(hour)*time.Hour + offset)
						for _, apply := range []func(adaptor.ModelConfig, time.Time) adaptor.ModelConfig{pricing.ApplyTimeWindow, pricing.ApplyTimeWindowRatioOnly} {
							got := apply(cfg, at)
							require.Equal(t, got, apply(cfg, at.In(time.FixedZone("Beijing", 8*3600))))
							require.InDelta(t, input, got.Ratio/ratio.MilliTokensUsd, 1e-12, "%s at %s", id, at)
							require.InDelta(t, cached, got.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
							require.InDelta(t, output, got.Ratio*got.CompletionRatio/ratio.MilliTokensUsd, 1e-12)
						}
					}
				}
				require.Equal(t, original, cfg)
			}
		})
	}
}

// TestOctoberDeepSeekUnaffectedBoundaries verifies holiday end, ordinary peak
// boundaries, make-up weekends and the original Flash switch using t; it returns nothing.
func TestOctoberDeepSeekUnaffectedBoundaries(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		at                   string
		flashInput, proInput float64
	}{
		{"2026-09-10T03:59:59.999999999Z", .44, 1.32},
		{"2026-09-10T04:00:00Z", .15, .66},
		{"2026-09-20T06:00:00Z", .15, .66},
		{"2026-09-24T09:59:59.999999999Z", .3, 1.32},
		{"2026-09-24T10:00:00Z", .15, .66},
		{"2026-09-28T01:00:00Z", .3, 1.32},
		{"2026-09-30T06:00:00Z", .3, 1.32},
		{"2026-10-07T23:59:59.999999999Z", .15, .66},
		{"2026-10-08T00:00:00Z", .15, .66},
		{"2026-10-08T00:59:59.999999999Z", .15, .66},
		{"2026-10-08T01:00:00Z", .3, 1.32},
		{"2026-10-10T06:00:00Z", .15, .66},
		{"2026-10-12T06:00:00Z", .3, 1.32},
	} {
		at, err := time.Parse(time.RFC3339Nano, tc.at)
		require.NoError(t, err)
		for id, cfg := range deepseek.ModelRatios {
			want := tc.flashInput
			if id == "deepseek-v4-pro" {
				want = tc.proInput
			}
			require.InDelta(t, want, pricing.ApplyTimeWindow(cfg, at).Ratio/ratio.MilliTokensUsd, 1e-12, "%s at %s", id, at)
			require.InDelta(t, want, pricing.ApplyTimeWindowRatioOnly(cfg, at).Ratio/ratio.MilliTokensUsd, 1e-12)
		}
	}
}
