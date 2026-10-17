package deepseek

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	modelcfg "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/pricing"
)

// TestDeepSeekPricingScheduleBoundaries verifies both billing paths against an
// independent price oracle. It checks every peak boundary, weekends, UTC date
// changes, and the Flash cutover and canceled Pro cutover, including nanoseconds.
func TestDeepSeekPricingScheduleBoundaries(t *testing.T) {
	t.Parallel()
	flashCutover := time.Date(2026, 9, 10, 4, 0, 0, 0, time.UTC)
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
						if name == "deepseek-v4-pro" {
							input, cached, output = 0.66, 0.022, 1.98
						} else if name != "deepseek-v4-pro" && at.Before(flashCutover) {
							input, cached, output = 0.22, 0.007, 0.66
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
// Monday peak into a Sunday off-peak or introduce the canceled Pro redirection.
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

// TestDeepSeekFlashCutoverHasOnePriceRegime verifies matching cutover-day
// overlays agree with the applicable price era and both production resolvers
// retain boundary prices. Parameters: t is the test handle. Returns: nothing.
func TestDeepSeekFlashCutoverHasOnePriceRegime(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	cutover := start.Add(4 * time.Hour)
	end := start.AddDate(0, 0, 1)
	instants := make([]time.Time, 0, 1460)
	for minute := 0; minute < 24*60; minute++ {
		instants = append(instants, start.Add(time.Duration(minute)*time.Minute))
	}
	for _, hour := range []int{0, 1, 4, 6, 10, 24} {
		for _, delta := range []time.Duration{-time.Nanosecond, 0, time.Nanosecond} {
			instants = append(instants, start.Add(time.Duration(hour)*time.Hour+delta))
		}
	}
	for _, name := range []string{"deepseek-flash", "deepseek-v4-flash", "deepseek-v4-flash-vision-exp"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cfg := ModelRatios[name]
			original := cfg.Clone()
			provider := &Adaptor{}
			for _, at := range instants {
				input, cached, output := .15, .003, .60
				if at.Before(cutover) {
					input, cached, output = .22, .007, .66
				}
				weekday := at.Weekday() != time.Saturday && at.Weekday() != time.Sunday
				peak := weekday && ((at.Hour() >= 1 && at.Hour() < 4) || (at.Hour() >= 6 && at.Hour() < 10))
				if peak {
					input, cached, output = 2*input, 2*cached, 2*output
				}
				// Adjacent dates retain ordinary holiday/peak/fallback priority.
				// On the transition day no conflicting price era may also match.
				if !at.Before(start) && at.Before(end) {
					matched := false
					for _, window := range cfg.TimeWindows {
						if !pricing.MatchTimeWindow(window, at) {
							continue
						}
						matched = true
						require.InDelta(t, input, window.Overlay.Ratio/ratio.MilliTokensUsd, 1e-12,
							"%s conflicting input era at %s", name, at)
						require.InDelta(t, cached, window.Overlay.CachedInputRatio/ratio.MilliTokensUsd, 1e-12,
							"%s conflicting cache era at %s", name, at)
						require.InDelta(t, output, window.Overlay.Ratio*window.Overlay.CompletionRatio/ratio.MilliTokensUsd, 1e-12,
							"%s conflicting output era at %s", name, at)
					}
					require.True(t, matched, "%s has no applicable era at %s", name, at)
				}
				for _, resolve := range []func(string, map[string]modelcfg.ModelConfigLocal, adaptor.Adaptor, time.Time) (adaptor.ModelConfig, bool){
					pricing.ResolveModelConfig,
					pricing.ResolveModelConfigRatioOnly,
				} {
					got, ok := resolve(name, nil, provider, at)
					require.True(t, ok)
					require.InDelta(t, input, got.Ratio/ratio.MilliTokensUsd, 1e-12,
						"%s resolved input at %s", name, at)
					require.InDelta(t, cached, got.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
					require.InDelta(t, output, got.Ratio*got.CompletionRatio/ratio.MilliTokensUsd, 1e-12)
				}
			}
			require.Equal(t, original, cfg, "resolution must not mutate shared defaults")
		})
	}
}
