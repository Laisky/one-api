package deepseek

import (
	"time"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// deepseekPricingWindows builds the official UTC weekday peak schedule followed
// by an all-day off-peak fallback. First-match ordering is intentional.
// Parameters: date bounds are inclusive/exclusive UTC dates; input, cachedInput,
// and output are off-peak USD prices per million tokens. Peak rates are double.
// Returns: independent windows covering every instant within the date bounds.
func deepseekPricingWindows(dateFrom, dateTo string, input, cachedInput, output float64) []adaptor.TimeWindow {
	return []adaptor.TimeWindow{
		{
			Name: "deepseek-peak", TimeZone: "UTC", DateFrom: dateFrom, DateTo: dateTo,
			DaysOfWeek: []int{int(time.Monday), int(time.Tuesday), int(time.Wednesday), int(time.Thursday), int(time.Friday)},
			Ranges:     []adaptor.ClockRange{{Start: "01:00", End: "04:00"}, {Start: "06:00", End: "10:00"}},
			Overlay: adaptor.ModelConfig{
				Ratio: 2 * input * ratio.MilliTokensUsd, CachedInputRatio: 2 * cachedInput * ratio.MilliTokensUsd,
				CompletionRatio: output / input,
			},
		},
		{
			Name: "deepseek-offpeak", TimeZone: "UTC", DateFrom: dateFrom, DateTo: dateTo,
			Ranges: []adaptor.ClockRange{{Start: "00:00", End: "00:00"}},
			Overlay: adaptor.ModelConfig{
				Ratio: input * ratio.MilliTokensUsd, CachedInputRatio: cachedInput * ratio.MilliTokensUsd,
				CompletionRatio: output / input,
			},
		},
	}
}

// deepseekProPricingWindows retains Pro prices until the announced switch to
// Flash on September 14, 2026 at 12:00 Beijing time (04:00 UTC).
// Parameters: none. Returns: first-match windows, including a one-day bridge
// because DateFrom/DateTo express dates rather than arbitrary timestamps.
func deepseekProPricingWindows() []adaptor.TimeWindow {
	const switchDate = "2026-09-14"
	windows := deepseekPricingWindows("", switchDate, deepseekProInputPrice, deepseekProCachedInputPrice, deepseekProOutputPrice)
	bridge := deepseekPricingWindows(switchDate, "2026-09-15", deepseekProInputPrice, deepseekProCachedInputPrice, deepseekProOutputPrice)
	bridge[0].Name = "deepseek-pro-final-peak"
	bridge[0].Ranges = []adaptor.ClockRange{{Start: "01:00", End: "04:00"}}
	bridge[1].Name = "deepseek-pro-final-offpeak"
	bridge[1].Ranges = []adaptor.ClockRange{{Start: "00:00", End: "01:00"}}
	windows = append(windows, bridge...)
	return append(windows, deepseekPricingWindows(switchDate, "", deepseekFlashInputPrice, deepseekFlashCachedInputPrice, deepseekFlashOutputPrice)...)
}

// deepseekFlashPricingWindows applies the announced Flash price change at
// September 10, 2026, 12:00 Beijing time (04:00 UTC), not UTC midnight.
// Parameters: none. Returns: first-match windows retaining the immediately
// preceding Flash prices until the transition. Earlier price eras are not archived.
func deepseekFlashPricingWindows() []adaptor.TimeWindow {
	const switchDate = "2026-09-10"
	// Preserve the preceding defaults from e000b8eb for in-flight requests whose
	// request start time is before the new prices become effective.
	windows := deepseekPricingWindows("", switchDate, 0.22, 0.007, 0.66)
	bridge := deepseekPricingWindows(switchDate, "2026-09-11", 0.22, 0.007, 0.66)
	bridge[0].Name = "deepseek-flash-final-old-peak"
	bridge[0].Ranges = []adaptor.ClockRange{{Start: "01:00", End: "04:00"}}
	bridge[1].Name = "deepseek-flash-final-old-offpeak"
	bridge[1].Ranges = []adaptor.ClockRange{{Start: "00:00", End: "01:00"}}
	windows = append(windows, bridge...)
	return append(windows, deepseekPricingWindows(switchDate, "", deepseekFlashInputPrice, deepseekFlashCachedInputPrice, deepseekFlashOutputPrice)...)
}
