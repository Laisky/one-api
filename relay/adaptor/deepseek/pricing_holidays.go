package deepseek

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// deepseekPricingWindowsWithHolidays prepends national holiday off-peak prices
// to an existing UTC schedule. Parameters are inclusive/exclusive ISO date bounds
// and off-peak USD/M input, cached-input and output prices. It returns independent
// windows clipped to the caller's pricing era; bridge windows are built separately.
// Sources, verified 2026-10-12:
//   - https://api-docs.deepseek.com/quick_start/pricing/
//   - https://www.bjdch.gov.cn/zwgk/hygq/202601/t20260123_4459144.html
//
// These are the published 2026 national dates, never a recurring lunar calendar.
// All peak hours are within the same Beijing and UTC calendar day, so full-day
// UTC exceptions are billing-equivalent. Working weekends stay off-peak.
func deepseekPricingWindowsWithHolidays(dateFrom, dateTo string, input, cachedInput, output float64) []adaptor.TimeWindow {
	var windows []adaptor.TimeWindow
	for _, dates := range [][2]string{
		{"2026-01-01", "2026-01-04"},
		{"2026-02-15", "2026-02-24"},
		{"2026-04-04", "2026-04-07"},
		{"2026-05-01", "2026-05-06"},
		{"2026-06-19", "2026-06-22"},
		{"2026-09-25", "2026-09-28"},
		{"2026-10-01", "2026-10-08"},
	} {
		start, end := dates[0], dates[1]
		if dateFrom != "" && start < dateFrom {
			start = dateFrom
		}
		if dateTo != "" && end > dateTo {
			end = dateTo
		}
		if start >= end {
			continue
		}
		windows = append(windows, adaptor.TimeWindow{
			Name: "deepseek-holiday-" + dates[0], TimeZone: "UTC", DateFrom: start, DateTo: end,
			Ranges: []adaptor.ClockRange{{Start: "00:00", End: "00:00"}},
			Overlay: adaptor.ModelConfig{
				Ratio:            input * ratio.MilliTokensUsd,
				CachedInputRatio: cachedInput * ratio.MilliTokensUsd,
				CompletionRatio:  output / input,
			},
		})
	}
	return append(windows, deepseekPricingWindows(dateFrom, dateTo, input, cachedInput, output)...)
}
