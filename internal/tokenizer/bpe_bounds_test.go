package tiktoken

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMergePartCountBounds exercises machine-int boundaries without allocating their implied buffers.
func TestMergePartCountBounds(t *testing.T) {
	for _, length := range []int{0, 1, 4096, math.MaxInt - 2, math.MaxInt - 1} {
		require.Equal(t, length+1, checkedMergePartCount(length))
	}
	for _, length := range []int{math.MaxInt, -1, math.MinInt} {
		require.PanicsWithValue(t, "tokenizer: BPE part count exceeds int range", func() {
			checkedMergePartCount(length)
		}, "length=%d", length)
	}
}

// TestMergePartCountNoAlloc requires the normal dimension check not to allocate request-scoped memory.
func TestMergePartCountNoAlloc(t *testing.T) {
	allocations := testing.AllocsPerRun(1000, func() {
		mergePartCountSink = checkedMergePartCount(128)
	})
	require.Zero(t, allocations)
	require.Equal(t, 129, mergePartCountSink)
}

var mergePartCountSink int
