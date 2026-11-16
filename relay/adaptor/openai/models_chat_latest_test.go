package openai

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestChatLatestFeatureOwnership checks that the family definition owns all its
// capability slices independently of sibling models and subsequent copies.
// Parameter t owns assertions; the test returns no values.
func TestChatLatestFeatureOwnership(t *testing.T) {
	first := newChatLatestConfig()
	second := newChatLatestConfig()
	original := ModelRatios["chat-latest"].Clone()
	require.Equal(t, original, first)
	require.Equal(t, original, second)
	require.Equal(t, []string{"reasoning", "tools", "json_mode", "structured_outputs", "web_search"}, first.SupportedFeatures)

	first.SupportedFeatures[0] = "mutated-feature"
	first.SupportedReasoningEfforts[0] = "mutated-effort"
	first.SupportedSamplingParameters[0] = "mutated-sampling"
	first.InputModalities[0] = "mutated-input"
	first.OutputModalities[0] = "mutated-output"
	require.Equal(t, original, second, "separate family configurations must not share capability storage")
	require.Equal(t, original, ModelRatios["chat-latest"], "family configuration must not share catalog capability storage")
	require.Equal(t, []string{"reasoning", "tools", "json_mode", "structured_outputs"}, gpt5ReasoningFeatures)
	require.Equal(t, []string{"medium"}, gpt5ChatOnlyEfforts)
}
