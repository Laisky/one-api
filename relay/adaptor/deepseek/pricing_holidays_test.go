package deepseek

import (
	"github.com/stretchr/testify/require"
	"testing"
)

// TestDeepSeekHolidayWindowOwnership verifies clipped, exclusive ISO bounds and
// independent allocations using t. It returns nothing and leaves globals untouched.
func TestDeepSeekHolidayWindowOwnership(t *testing.T) {
	t.Parallel()
	first := deepseekPricingWindowsWithHolidays("2026-09-26", "2026-10-03", .15, .003, .6)
	second := deepseekPricingWindowsWithHolidays("2026-09-26", "2026-10-03", .15, .003, .6)
	require.Len(t, first, 4)
	require.Equal(t, "2026-09-26", first[0].DateFrom)
	require.Equal(t, "2026-09-28", first[0].DateTo)
	require.Equal(t, "2026-10-01", first[1].DateFrom)
	require.Equal(t, "2026-10-03", first[1].DateTo)
	require.Equal(t, "deepseek-peak", first[2].Name)
	first[0].Ranges[0].Start = "12:00"
	require.Equal(t, "00:00", second[0].Ranges[0].Start)
	require.Equal(t, "00:00", first[1].Ranges[0].Start)
	require.Len(t, deepseekPricingWindowsWithHolidays("2027-01-01", "2028-01-01", .15, .003, .6), 2, "never repeat the 2026 lunar calendar")
	require.Len(t, deepseekPricingWindowsWithHolidays("2026-09-10", "2026-09-11", .15, .003, .6), 2)
}
