package cerebras_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/cerebras"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// TestPublicCatalogMetadata verifies the public adaptor's serialized prices and
// capabilities against the 2026-09-24 provider snapshot using t; it returns nothing.
func TestPublicCatalogMetadata(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		id                   string
		input, output        float64
		context, outputLimit int32
		efforts              []string
		defaultEffort        string
		vision, logprobs     bool
	}{
		{"gpt-oss-120b", 0.35, 0.75, 131072, 40960, []string{"low", "medium", "high"}, "medium", false, false},
		{"qwen-3.8-27b", 0.99, 1.49, 65536, 32768, []string{"none", "low", "medium", "high"}, "high", true, true},
	} {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			a := &cerebras.Adaptor{}
			require.Contains(t, a.GetModelList(), tc.id)
			config, exists := a.GetDefaultModelPricing()[tc.id]
			require.True(t, exists)
			encoded, err := json.Marshal(config)
			require.NoError(t, err)
			var got adaptor.ModelConfig
			require.NoError(t, json.Unmarshal(encoded, &got))
			require.InDelta(t, tc.input, a.GetModelRatio(tc.id)/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, tc.output, a.GetModelRatio(tc.id)*a.GetCompletionRatio(tc.id)/ratio.MilliTokensUsd, 1e-12)
			require.Zero(t, got.CachedInputRatio, "no cache discount is published")
			require.Equal(t, tc.context, got.ContextLength)
			require.Equal(t, tc.outputLimit, got.MaxOutputTokens)
			require.Zero(t, got.MaxTokens, "catalog metadata must not introduce an account-tier request cap")
			require.Equal(t, tc.efforts, got.SupportedReasoningEfforts)
			require.Equal(t, tc.defaultEffort, got.DefaultReasoningEffort)
			require.Contains(t, got.SupportedSamplingParameters, "reasoning_effort")
			require.Contains(t, got.SupportedFeatures, "structured_outputs")
			require.NotEmpty(t, got.HuggingFaceID)
			require.Empty(t, got.Quantization, "mixed precision is not one uniform precision label")
			if tc.vision {
				require.Contains(t, got.InputModalities, "image")
			} else {
				require.Equal(t, []string{"text"}, got.InputModalities)
			}
			for _, parameter := range []string{"logprobs", "top_logprobs", "parallel_tool_calls"} {
				if tc.logprobs {
					require.Contains(t, got.SupportedSamplingParameters, parameter)
				} else {
					require.NotContains(t, got.SupportedSamplingParameters, parameter)
				}
			}
		})
	}
}

// TestDedicatedCatalogCompatibility verifies discoverability, historical prices,
// and independent model lists using t; it returns nothing.
func TestDedicatedCatalogCompatibility(t *testing.T) {
	t.Parallel()

	a := &cerebras.Adaptor{}
	for id, prices := range map[string][2]float64{
		"zai-glm-4.7": {2.25, 2.75},
		"gemma-4-31b": {0.99, 1.49},
	} {
		require.Contains(t, a.GetModelList(), id)
		config := a.GetDefaultModelPricing()[id]
		require.InDelta(t, prices[0], config.Ratio/ratio.MilliTokensUsd, 1e-12)
		require.InDelta(t, prices[1], config.Ratio*config.CompletionRatio/ratio.MilliTokensUsd, 1e-12)
		require.Contains(t, config.Description, "not a current enterprise quote")
	}
	gemma := a.GetDefaultModelPricing()["gemma-4-31b"]
	require.Equal(t, []string{"none", "low", "medium", "high"}, gemma.SupportedReasoningEfforts)
	require.Equal(t, "none", gemma.DefaultReasoningEffort)
	first, second := a.GetModelList(), a.GetModelList()
	require.NotEmpty(t, first)
	first[0] = "caller-local-model"
	require.NotContains(t, second, "caller-local-model")
	require.ElementsMatch(t, second, a.GetModelList())
}
