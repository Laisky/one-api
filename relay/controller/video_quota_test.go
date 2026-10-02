package controller

import (
	"github.com/stretchr/testify/require"
	"math"
	"testing"
)

// TestVideoQuotaDecimal verifies exact decimal pricing and rejects unsafe rates.
func TestVideoQuotaDecimal(t *testing.T) {
	for _, tc := range []struct {
		name                              string
		second, multiple, duration, image float64
		count                             int
		group                             float64
		want                              int64
		invalid                           bool
	}{
		{"image rounding", .08, 1, 5, .01, 1, 1, 205000, false},
		{"720p", .08, 1.75, 5, .01, 0, 1, 350000, false},
		{"1080p", .08, 3.125, 5, 0, 0, 1, 625000, false},
		{"fractional unit ceiling", .000001, 1, 1, 0, 0, 1, 1, false},
		{"free group", .08, 1, 5, .01, 1, 0, 0, false},
		{"negative rate", -.08, 1, 5, 0, 0, 1, 0, true},
		{"negative group", .08, 1, 5, 0, 0, -1, 0, true},
		{"nan", math.NaN(), 1, 5, 0, 0, 1, 0, true},
		{"infinity", .08, math.Inf(1), 5, 0, 0, 1, 0, true},
		{"overflow", 1e100, 1, 5, 0, 0, 1, 0, true},
		{"zero duration", .08, 1, 0, 0, 0, 1, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := videoQuota(tc.second, tc.multiple, tc.duration, tc.image, tc.count, tc.group)
			if tc.invalid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
