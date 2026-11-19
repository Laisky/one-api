package realtime

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCostPricesExplicitVideoOutput verifies that a priced video output rate
// applies only to the explicitly attributed VIDEO subset, even when it is the
// most expensive output rate, that the rest of the unattributed output keeps
// its cheapest-rate charge, and that an unpriced rate leaves Cost unchanged.
// It accepts t and returns no values.
func TestCostPricesExplicitVideoOutput(t *testing.T) {
	t.Parallel()
	record := Record{Tokens: Tokens{Input: 100, Text: 100, Output: 1400, OutputText: 400,
		OutputUnallocated: 1000, OutputVideo: 600}}
	base := Rates{Text: 1, OutputText: 6, Audio: 4, OutputAudio: 16}
	legacy, err := Cost(record, base)
	require.NoError(t, err)
	require.Equal(t, 100.0+400*6+1000*6, legacy, "without a video rate all unattributed output uses the cheapest rate")

	for _, tc := range []struct {
		name  string
		video float64
		want  float64
	}{
		{"cheaper video", 2, 100 + 400*6 + 600*2 + 400*6},
		{"dearer video", 10, 100 + 400*6 + 600*10 + 400*6},
	} {
		rates := base
		rates.OutputVideo = tc.video
		got, err := Cost(record, rates)
		require.NoError(t, err, tc.name)
		require.Equal(t, tc.want, got, tc.name)
	}

	for _, invalid := range []float64{-1, math.NaN(), math.Inf(1)} {
		rates := base
		rates.OutputVideo = invalid
		_, err := Cost(record, rates)
		require.ErrorIs(t, err, ErrInvalidPrice, "%v", invalid)
	}
}
