package deepseek

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/pricing"
)

// TestDeepSeekPricingScheduleBoundaries verifies both billing paths against an
// independent price oracle. It checks every peak boundary, weekends, UTC date
// changes, and both sides of the Pro-to-Flash cutover, including nanoseconds.
func TestDeepSeekPricingScheduleBoundaries(t *testing.T) {
	t.Parallel()
	cutover := time.Date(2026, 9, 14, 4, 0, 0, 0, time.UTC)
	start := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	for name, cfg := range ModelRatios {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			original := cfg.Clone()
			for day := 0; day < 11; day++ {
				for _, hour := range []int{0, 1, 4, 6, 10, 24} {
					boundary := start.AddDate(0, 0, day).Add(time.Duration(hour) * time.Hour)
					for _, delta := range []time.Duration{-time.Nanosecond, 0, time.Nanosecond} {
						at := boundary.Add(delta)
						input, cached, output := 0.15, 0.003, 0.60
						if name == "deepseek-v4-pro" && at.Before(cutover) {
							input, cached, output = 0.66, 0.022, 1.98
						}
						weekday := at.Weekday() != time.Saturday && at.Weekday() != time.Sunday
						peak := weekday && ((at.Hour() >= 1 && at.Hour() < 4) || (at.Hour() >= 6 && at.Hour() < 10))
						if peak {
							input, cached, output = input*2, cached*2, output*2
						}
						for _, apply := range []func(adaptor.ModelConfig, time.Time) adaptor.ModelConfig{pricing.ApplyTimeWindow, pricing.ApplyTimeWindowRatioOnly} {
							resolved := apply(cfg, at)
							require.InDelta(t, input, resolved.Ratio/ratio.MilliTokensUsd, 1e-12, "%s input at %s", name, at)
							require.InDelta(t, cached, resolved.CachedInputRatio/ratio.MilliTokensUsd, 1e-12, "%s cached input at %s", name, at)
							require.InDelta(t, output, resolved.Ratio*resolved.CompletionRatio/ratio.MilliTokensUsd, 1e-12, "%s output at %s", name, at)
							require.Empty(t, resolved.TimeWindows, "a window must cover %s", at)
						}
					}
				}
			}
			require.Equal(t, original, cfg, "resolving prices must not mutate shared model defaults")
		})
	}
}

// TestDeepSeekPricingUsesUTC verifies that caller-local dates cannot turn a UTC
// Monday peak into a Sunday off-peak or shift the Pro redirection instant.
func TestDeepSeekPricingUsesUTC(t *testing.T) {
	t.Parallel()
	for name, cfg := range ModelRatios {
		for _, hour := range []int{3, 4, 6} {
			utc := time.Date(2026, 9, 14, hour, 0, 0, 0, time.UTC)
			local := utc.In(time.FixedZone("UTC-7", -7*60*60))
			require.Equal(t, pricing.ApplyTimeWindow(cfg, utc), pricing.ApplyTimeWindow(cfg, local), name)
			require.Equal(t, pricing.ApplyTimeWindowRatioOnly(cfg, utc), pricing.ApplyTimeWindowRatioOnly(cfg, local), name)
		}
	}
}
