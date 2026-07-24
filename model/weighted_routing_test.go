package model

import (
	"math"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

// fixedDraw returns a draw function that always yields v, for deterministic
// coverage of the weighted-selection boundaries.
func fixedDraw(v int64) func(int64) int64 {
	return func(n int64) int64 { return v }
}

// uintPtr is a small helper mirroring how callers build *uint weights.
func uintPtr(v uint) *uint { return &v }

func TestWeightedIndex_Deterministic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		weights []uint
		draw    int64
		want    int
	}{
		{name: "empty returns -1", weights: nil, draw: 0, want: -1},
		{name: "single always index 0", weights: []uint{7}, draw: 0, want: 0},
		{name: "boundary 7/3 draw 0 -> first", weights: []uint{7, 3}, draw: 0, want: 0},
		{name: "boundary 7/3 draw 6 -> first", weights: []uint{7, 3}, draw: 6, want: 0},
		{name: "boundary 7/3 draw 7 -> second", weights: []uint{7, 3}, draw: 7, want: 1},
		{name: "boundary 7/3 draw 9 -> second", weights: []uint{7, 3}, draw: 9, want: 1},
		{name: "zero weights excluded, draw 0", weights: []uint{0, 10, 0}, draw: 0, want: 1},
		{name: "zero weights excluded, draw 9", weights: []uint{0, 10, 0}, draw: 9, want: 1},
		{name: "all zero returns -1", weights: []uint{0, 0, 0}, draw: 0, want: -1},
		{name: "leading zeros then weight", weights: []uint{0, 0, 5}, draw: 4, want: 2},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := weightedIndex(tt.weights, fixedDraw(tt.draw))
			require.Equal(t, tt.want, got)
		})
	}
}

// TestWeightedIndex_FullBoundarySweep proves every draw value in [0,total) maps to
// the correct cumulative bucket for the canonical 7/3 split.
func TestWeightedIndex_FullBoundarySweep(t *testing.T) {
	t.Parallel()
	weights := []uint{7, 3}
	for r := int64(0); r < 10; r++ {
		want := 0
		if r >= 7 {
			want = 1
		}
		require.Equalf(t, want, weightedIndex(weights, fixedDraw(r)), "draw=%d", r)
	}
}

func TestWeightedIndex_MaxWeightSucceeds(t *testing.T) {
	t.Parallel()
	// Exactly the cap: still a single valid bucket, no clamping effect.
	got := weightedIndex([]uint{uint(maxRoutingWeight)}, fixedDraw(0))
	require.Equal(t, 0, got)
}

func TestWeightedIndex_OverMaxIsClampedNoPanic(t *testing.T) {
	t.Parallel()
	require.NotPanics(t, func() {
		// A weight above the cap plus a normal weight: the huge one is clamped to
		// maxRoutingWeight, so it dominates but never overflows.
		got := weightedIndex([]uint{uint(maxRoutingWeight) + 1, 1}, rand.Int63n)
		require.GreaterOrEqual(t, got, 0)
		require.Less(t, got, 2)
	})
}

func TestWeightedIndex_SumOverflowNoPanic(t *testing.T) {
	t.Parallel()
	// Many maximal-uint weights: without clamping this overflows int64 and panics
	// in rand.Int63n. clampRoutingWeight must keep the accumulator sane.
	weights := make([]uint, 64)
	for i := range weights {
		weights[i] = math.MaxUint64
	}
	require.NotPanics(t, func() {
		got := weightedIndex(weights, rand.Int63n)
		require.GreaterOrEqual(t, got, 0)
		require.Less(t, got, len(weights))
	})
}

func TestWeightedIndex_MisbehavingDrawStaysInRange(t *testing.T) {
	t.Parallel()
	// A draw that violates the [0,total) contract must not panic and must still
	// return an in-range index (the defensive last-index fallback).
	got := weightedIndex([]uint{1, 1, 1}, fixedDraw(1_000_000))
	require.GreaterOrEqual(t, got, 0)
	require.Less(t, got, 3)
}

func TestClampRoutingWeight(t *testing.T) {
	t.Parallel()
	require.Equal(t, int64(0), clampRoutingWeight(0))
	require.Equal(t, int64(7), clampRoutingWeight(7))
	require.Equal(t, maxRoutingWeight, clampRoutingWeight(uint(maxRoutingWeight)))
	require.Equal(t, maxRoutingWeight, clampRoutingWeight(uint(maxRoutingWeight)+1))
	require.Equal(t, maxRoutingWeight, clampRoutingWeight(math.MaxUint64))
}

// FuzzWeightedIndex asserts the invariant on arbitrary weight vectors: the result
// is always -1 or a valid index, it never panics, and -1 happens iff every clamped
// weight is zero.
func FuzzWeightedIndex(f *testing.F) {
	f.Add(uint(0), uint(0), uint(0), int64(0))
	f.Add(uint(7), uint(3), uint(0), int64(5))
	f.Add(uint(math.MaxUint64), uint(1), uint(0), int64(0))
	f.Add(uint(0), uint(0), uint(1), int64(0))

	f.Fuzz(func(t *testing.T, a, b, c uint, draw int64) {
		weights := []uint{a, b, c}
		// Constrain the injected draw to a legal range derived from the same
		// clamp the function uses, so we only assert the production contract.
		var total int64
		for _, w := range weights {
			total += clampRoutingWeight(w)
		}
		var drawFn func(int64) int64
		if total > 0 {
			drawFn = fixedDraw(((draw % total) + total) % total)
		} else {
			drawFn = fixedDraw(0)
		}

		var got int
		require.NotPanics(t, func() { got = weightedIndex(weights, drawFn) })

		if total <= 0 {
			require.Equal(t, -1, got, "all-zero weights must signal uniform fallback")
			return
		}
		require.GreaterOrEqual(t, got, 0)
		require.Less(t, got, len(weights))
		// The selected bucket must carry positive weight.
		require.Positive(t, clampRoutingWeight(weights[got]), "selected index must have non-zero weight")
	})
}

func TestSelectAbilityByWeight_EmptyErrors(t *testing.T) {
	t.Parallel()
	_, err := selectAbilityByWeight(nil)
	require.Error(t, err)
}

func TestSelectAbilityByWeight_NilWeightTreatedAsZero(t *testing.T) {
	t.Parallel()
	// One ability has nil weight (NULL in DB), the other a positive weight.
	// Weighted selection must always choose the positive one.
	abilities := []Ability{
		{ChannelId: 1, Weight: nil},
		{ChannelId: 2, Weight: uintPtr(5)},
	}
	for range 200 {
		got, err := selectAbilityByWeight(abilities)
		require.NoError(t, err)
		require.Equal(t, 2, got.ChannelId)
	}
}

func TestSelectAbilityByWeight_AllZeroUniformCoversAll(t *testing.T) {
	t.Parallel()
	abilities := []Ability{
		{ChannelId: 1, Weight: uintPtr(0)},
		{ChannelId: 2, Weight: nil},
		{ChannelId: 3, Weight: uintPtr(0)},
	}
	seen := map[int]bool{}
	for range 500 {
		got, err := selectAbilityByWeight(abilities)
		require.NoError(t, err)
		seen[got.ChannelId] = true
	}
	require.Len(t, seen, 3, "uniform fallback must be able to pick every candidate")
}

func TestSelectAbilityByWeight_DistributionApproximatelyMatchesWeights(t *testing.T) {
	t.Parallel()
	abilities := []Ability{
		{ChannelId: 1, Weight: uintPtr(7)},
		{ChannelId: 2, Weight: uintPtr(3)},
	}
	counts := map[int]int{}
	const n = 20000
	for range n {
		got, err := selectAbilityByWeight(abilities)
		require.NoError(t, err)
		counts[got.ChannelId]++
	}
	ratio1 := float64(counts[1]) / float64(n)
	require.InDelta(t, 0.70, ratio1, 0.05, "channel 1 should get ~70%% of traffic, got %.3f", ratio1)
}

func TestPickWeightedChannel_EmptyAndAllZeroReturnNil(t *testing.T) {
	t.Parallel()
	require.Nil(t, pickWeightedChannel(nil))
	require.Nil(t, pickWeightedChannel([]*Channel{}))

	channels := []*Channel{
		{Id: 1, Weight: uintPtr(0)},
		{Id: 2, Weight: nil},
	}
	require.Nil(t, pickWeightedChannel(channels), "all-zero weights must signal uniform fallback via nil")
}

func TestPickWeightedChannel_WeightedSelection(t *testing.T) {
	t.Parallel()
	channels := []*Channel{
		{Id: 1, Weight: uintPtr(0)},
		{Id: 2, Weight: uintPtr(9)},
		{Id: 3, Weight: uintPtr(0)},
	}
	for range 200 {
		got := pickWeightedChannel(channels)
		require.NotNil(t, got)
		require.Equal(t, 2, got.Id, "only the positively-weighted channel may be chosen")
	}
}
