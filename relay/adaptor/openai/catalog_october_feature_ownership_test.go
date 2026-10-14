package openai

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
)

// TestOpenAIOctoberFeatureOwnership checks that refreshing chat-latest never
// writes through caller-owned feature storage, even for empty slices with spare
// capacity. Parameter t owns assertions; the test returns no values.
func TestOpenAIOctoberFeatureOwnership(t *testing.T) {
	for _, tc := range []struct {
		name    string
		storage []string
		length  int
	}{
		{name: "nil"},
		{name: "empty", storage: []string{}},
		{name: "empty_shared_capacity", storage: []string{"sibling-feature", "spare"}},
		{name: "populated_shared_capacity", storage: []string{"streaming", "spare"}, length: 1},
		{name: "existing_search_shared_capacity", storage: []string{"web_search", "spare"}, length: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			features := tc.storage[:tc.length]
			originalStorage := slices.Clone(tc.storage)
			originalFeatures := slices.Clone(features)
			before := map[string]adaptor.ModelConfig{
				"chat-latest": {Ratio: 42, SupportedFeatures: features},
				"sibling":    {Ratio: 43, SupportedFeatures: tc.storage},
			}
			after := applyOpenAIModelCatalog202610(before)
			require.Equal(t, originalStorage, tc.storage, "refresh wrote through shared feature storage")
			require.Equal(t, originalFeatures, before["chat-latest"].SupportedFeatures)
			require.Equal(t, before["sibling"], after["sibling"])
			require.Equal(t, 42.0, after["chat-latest"].Ratio)
			want := slices.Clone(originalFeatures)
			if !slices.Contains(want, "web_search") {
				want = append(want, "web_search")
			}
			require.Equal(t, want, after["chat-latest"].SupportedFeatures)
			require.Equal(t, after, applyOpenAIModelCatalog202610(after), "refresh must remain idempotent")
			after["chat-latest"].SupportedFeatures[0] = "mutated-after-refresh"
			require.Equal(t, originalStorage, tc.storage, "result still shares caller-owned feature storage")
			require.Equal(t, originalFeatures, before["chat-latest"].SupportedFeatures)
		})
	}
}
