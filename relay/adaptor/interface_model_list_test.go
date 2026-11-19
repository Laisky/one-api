package adaptor

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGetModelListFromPricingIsSortedAndUnique pins that the derived model list
// is deterministic: every pricing key appears exactly once, in ascending order,
// on every call regardless of Go's randomized map iteration.
func TestGetModelListFromPricingIsSortedAndUnique(t *testing.T) {
	pricing := map[string]ModelConfig{}
	for _, name := range []string{
		"gpt-5", "Claude-Opus", "claude-opus", "a", "deepseek-v4", "gpt-4o", "gpt-4o-mini",
		"qwen3-max", "Qwen/Qwen3-14B", "z-model", "0-numeric", "gpt-5.1", "gpt-5-mini",
	} {
		pricing[name] = ModelConfig{Ratio: 1}
	}

	first := GetModelListFromPricing(pricing)
	require.Len(t, first, len(pricing))
	require.True(t, slices.IsSorted(first), "model list must be sorted: %v", first)
	require.Len(t, slices.Compact(slices.Clone(first)), len(first), "model list must not contain duplicates: %v", first)
	for _, name := range first {
		require.Contains(t, pricing, name)
	}

	for range 50 {
		require.Equal(t, first, GetModelListFromPricing(pricing), "model list order must not depend on map iteration")
	}

	first[0] = "mutated"
	require.NotEqual(t, "mutated", GetModelListFromPricing(pricing)[0], "callers must receive a fresh slice")
}

// TestGetModelListFromPricingEmpty pins that an empty or nil table yields an
// empty, non-nil list, preserving the previous JSON shape ([] rather than null).
func TestGetModelListFromPricingEmpty(t *testing.T) {
	require.NotNil(t, GetModelListFromPricing(nil))
	require.Empty(t, GetModelListFromPricing(nil))
	require.NotNil(t, GetModelListFromPricing(map[string]ModelConfig{}))
	require.Empty(t, GetModelListFromPricing(map[string]ModelConfig{}))
}
