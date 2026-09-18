package typesafe

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestInputQuota uses exact decimal expectations rather than binary-float rounding.
func TestInputQuota(t *testing.T) {
	for _, tc := range []struct {
		tokens       int
		input, group float64
		want         int64
	}{
		{1000, 0.021, 1, 21}, {312, 0.021, 1, 7}, {312, 0.021, 2, 14},
		{1, 0.021, 1, 1}, {0, 0.021, 1, 0}, {312, 0.021, 0, 0},
		{312, 0, 1, 0}, {100, 0.1, 1, 10}, {AdmissionInputTokens, 0.021, 1, 1377},
	} {
		got, err := InputQuota(tc.tokens, tc.input, tc.group)
		require.NoError(t, err)
		require.Equal(t, tc.want, got)
	}
	for _, value := range []float64{-1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		_, err := InputQuota(1, value, 1)
		require.Error(t, err)
		_, err = InputQuota(1, 1, value)
		require.Error(t, err)
	}
	_, err := InputQuota(-1, 1, 1)
	require.Error(t, err)
	_, err = InputQuota(AdmissionInputTokens, math.MaxFloat64, 1)
	require.Error(t, err)
}

// TestAdmissionRejections distinguishes documented rejections from ambiguous work.
func TestAdmissionRejections(t *testing.T) {
	for _, status := range []int{401, 422, 429, 529} {
		require.True(t, IsAdmissionRejection(status))
	}
	for _, status := range []int{200, 302, 400, 408, 500, 502, 503, 504} {
		require.False(t, IsAdmissionRejection(status))
	}
}
