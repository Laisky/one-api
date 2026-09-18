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

// TestAdmissionRejections distinguishes proven rejections from ambiguous work.
// The 4xx statuses below were all observed live on 2026-09-18 with no usage
// receipt in the body; billing any of them would charge for zero evaluation.
func TestAdmissionRejections(t *testing.T) {
	for _, status := range []int{
		400, // max_tokens_exceeded, unknown model, primitive and cap violations
		401, // invalid API key
		403, // absent API key
		404, // base URL missing the /v1 prefix
		405, // wrong method
		413, 422, 429,
		StatusOverloaded,
	} {
		require.True(t, IsAdmissionRejection(status), status)
	}
	for _, status := range []int{200, 201, 302, 500, 502, 503, 504, 599} {
		require.False(t, IsAdmissionRejection(status), status)
	}
	require.Equal(t, 529, StatusOverloaded)
}
