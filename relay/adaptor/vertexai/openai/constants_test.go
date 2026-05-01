package openai

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestModelRatiosMetadata verifies Vertex OpenAI MaaS models expose explicit OpenRouter metadata.
// Parameter t coordinates test execution. Returns no values.
func TestModelRatiosMetadata(t *testing.T) {
	t.Parallel()

	cfg20b, ok := ModelRatios["openai/gpt-oss-20b-maas"]
	require.True(t, ok)
	require.EqualValues(t, 131072, cfg20b.ContextLength)
	require.EqualValues(t, 131072, cfg20b.MaxOutputTokens)
	require.Equal(t, []string{"text"}, cfg20b.InputModalities)
	require.Equal(t, []string{"text"}, cfg20b.OutputModalities)
	require.Equal(t, []string{"reasoning"}, cfg20b.SupportedFeatures)
	require.Equal(t, []string{"stop", "seed", "max_tokens"}, cfg20b.SupportedSamplingParameters)
	require.Equal(t, "fp4", cfg20b.Quantization)
	require.Equal(t, "openai/gpt-oss-20b", cfg20b.HuggingFaceID)

	cfg120b, ok := ModelRatios["openai/gpt-oss-120b-maas"]
	require.True(t, ok)
	require.EqualValues(t, 131072, cfg120b.ContextLength)
	require.EqualValues(t, 131072, cfg120b.MaxOutputTokens)
	require.Equal(t, []string{"text"}, cfg120b.InputModalities)
	require.Equal(t, []string{"text"}, cfg120b.OutputModalities)
	require.Equal(t, []string{"tools", "json_mode", "structured_outputs", "reasoning"}, cfg120b.SupportedFeatures)
	require.Equal(t, []string{"stop", "seed", "max_tokens"}, cfg120b.SupportedSamplingParameters)
	require.Equal(t, "fp4", cfg120b.Quantization)
	require.Equal(t, "openai/gpt-oss-120b", cfg120b.HuggingFaceID)
}
