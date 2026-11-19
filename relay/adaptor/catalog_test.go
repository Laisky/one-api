package adaptor_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
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

// TestJoinModelCatalogsMergesFamilies checks that every entry of several disjoint
// families survives assembly unchanged and that the result is a fresh map whose
// writes never reach the inputs, using t. It returns nothing.
func TestJoinModelCatalogsMergesFamilies(t *testing.T) {
	t.Parallel()
	chat := map[string]adaptor.ModelConfig{
		"chat-a": {Ratio: 1, CompletionRatio: 4, ContextLength: 128000},
		"chat-b": {Ratio: 0.5, CompletionRatio: 2, InputModalities: []string{"text", "image"}},
	}
	embedding := map[string]adaptor.ModelConfig{"embed-a": {Ratio: 0.1}}
	image := map[string]adaptor.ModelConfig{"image-a": {Ratio: 3, Description: "image family"}}

	got := adaptor.JoinModelCatalogs(chat, nil, embedding, map[string]adaptor.ModelConfig{}, image)
	require.Equal(t, map[string]adaptor.ModelConfig{
		"chat-a":  chat["chat-a"],
		"chat-b":  chat["chat-b"],
		"embed-a": embedding["embed-a"],
		"image-a": image["image-a"],
	}, got)

	delete(got, "chat-a")
	got["new"] = adaptor.ModelConfig{Ratio: 9}
	require.Len(t, chat, 2, "assembly must not alias an input family")
	require.Contains(t, chat, "chat-a")
	require.NotContains(t, image, "new")
}

// TestJoinModelCatalogsDuplicatePanics checks that a model ID defined by two
// families panics with a message naming the ID and the zero-based argument
// indexes of both families, including when nil or empty families sit between
// them, using t. It returns nothing.
func TestJoinModelCatalogsDuplicatePanics(t *testing.T) {
	t.Parallel()
	first := map[string]adaptor.ModelConfig{"shared": {Ratio: 1}}
	other := map[string]adaptor.ModelConfig{"other": {Ratio: 2}}
	repeat := map[string]adaptor.ModelConfig{"shared": {Ratio: 1}}

	require.PanicsWithValue(t,
		"duplicate model ID in catalog: shared (families 0 and 1)",
		func() { adaptor.JoinModelCatalogs(first, repeat) })
	require.PanicsWithValue(t,
		"duplicate model ID in catalog: shared (families 1 and 4)",
		func() {
			adaptor.JoinModelCatalogs(other, first, nil, map[string]adaptor.ModelConfig{}, repeat)
		})
	require.PanicsWithValue(t,
		"duplicate model ID in catalog: shared (families 0 and 1)",
		func() { adaptor.JoinModelCatalogs(first, first) },
		"passing the same family map twice is still a duplicate")
}
