package adaptor_test

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/stretchr/testify/require"
	"testing"
)

// TestJoinModelCatalogs checks disjoint assembly, duplicate rejection and explicit
// free prices using t. It returns nothing and never edits supplied family maps.
func TestJoinModelCatalogs(t *testing.T) {
	t.Parallel()
	first := map[string]adaptor.ModelConfig{"a": {Ratio: 2, CompletionRatio: 3}}
	second := map[string]adaptor.ModelConfig{"free": {Ratio: 0, CompletionRatio: 0}}
	got := adaptor.JoinModelCatalogs(nil, first, second)
	require.Len(t, got, 2)
	require.Equal(t, first["a"], got["a"])
	require.Zero(t, got["free"].Ratio)
	require.Zero(t, got["free"].CompletionRatio)
	got["a"] = adaptor.ModelConfig{Ratio: 99}
	require.Equal(t, 2.0, first["a"].Ratio)
	require.Empty(t, adaptor.JoinModelCatalogs())
	require.Panics(t, func() { adaptor.JoinModelCatalogs(first, first) })
}
