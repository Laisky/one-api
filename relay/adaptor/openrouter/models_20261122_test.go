package openrouter_test

import (
	"encoding/json"
	"testing"

	"github.com/Laisky/one-api/relay/adaptor/openrouter"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/stretchr/testify/require"
)

// TestGPT62OpenRouterPricingBoundary verifies every token bucket across the
// provider's strict 272,000-prompt-token boundary using the production resolver.
// It accepts t and returns no values.
func TestGPT62OpenRouterPricingBoundary(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		id    string
		base  float64
		write float64
	}{
		{"openai/gpt-6.2-astra", 8, 10},
		{"openai/gpt-6.2-astra-pro", 8, 10},
		{"openai/gpt-6.2-sol", 2, 2.5},
		{"openai/gpt-6.2-sol-pro", 2, 2.5},
	} {
		for _, prompt := range []int{1, 271999, 272000, 272001, 1000000} {
			got := pricing.ResolveEffectivePricingForUsage(tc.id, prompt, 128000, &openrouter.Adaptor{})
			input, output, write, threshold := tc.base, tc.base*5, tc.write, 0
			if prompt > 272000 {
				input, output, write, threshold = tc.base*2, tc.base*7.5, tc.write*2, 272001
			}
			require.Equal(t, threshold, got.AppliedTierThreshold, tc.id)
			require.Zero(t, got.AppliedOutputTierThreshold, "output tokens do not activate the prompt tier")
			require.InDelta(t, input, got.InputRatio/ratio.MilliTokensUsd, 1e-12, tc.id)
			require.InDelta(t, output, got.OutputRatio/ratio.MilliTokensUsd, 1e-12, tc.id)
			require.InDelta(t, input*0.05, got.CachedInputRatio/ratio.MilliTokensUsd, 1e-12, tc.id)
			require.InDelta(t, write, got.CacheWrite5mRatio/ratio.MilliTokensUsd, 1e-12, tc.id)
			require.Zero(t, got.CacheWrite1hRatio, "no distinct one-hour write price is published")
		}
		require.NotContains(t, openrouter.ModelRatios[tc.id].SupportedReasoningEfforts, "none")
		require.NotContains(t, (&openrouter.Adaptor{}).GetModelList(), tc.id+":batch")
	}
}

// TestNovember22OpenRouterCapabilities preserves provider-specific modalities,
// reasoning vocabularies and unknown prices. It accepts t and returns no values.
func TestNovember22OpenRouterCapabilities(t *testing.T) {
	t.Parallel()
	neon := openrouter.ModelRatios["google/gemini-4-neon"]
	require.Equal(t, int32(65536), neon.MaxOutputTokens, "ordinary output follows the host FAQ and standard AI Studio endpoint")
	require.ElementsMatch(t, []string{"text", "image", "file", "audio", "video"}, neon.InputModalities)
	require.NotNil(t, neon.Audio)
	require.InDelta(t, 3, neon.Ratio*neon.Audio.PromptRatio/ratio.MilliTokensUsd, 1e-12)
	require.InDelta(t, 3+1.0/12, neon.CacheWrite5mRatio/ratio.MilliTokensUsd, 1e-12)
	require.Equal(t, []string{"xhigh", "high", "medium", "low"}, neon.SupportedReasoningEfforts)
	require.Equal(t, "medium", neon.DefaultReasoningEffort)
	inkling := openrouter.ModelRatios["thinkingmachines/inkling-2-mini"]
	require.Contains(t, inkling.SupportedReasoningEfforts, "none")
	require.Equal(t, int32(65536), inkling.MaxOutputTokens)
	pixtral := openrouter.ModelRatios["mistralai/pixtral-large-2502"]
	require.Zero(t, pixtral.CachedInputRatio, "missing cached-input pricing is unspecified")
	require.NotContains(t, pixtral.SupportedFeatures, "tools")
	require.NotContains(t, pixtral.SupportedSamplingParameters, "tools")
	require.Equal(t, []string{"text", "image"}, pixtral.InputModalities)
}

// TestNovember22OpenRouterRequests preserves supported tool requests and model
// IDs through the existing converter. It accepts t and returns no values.
func TestNovember22OpenRouterRequests(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"openai/gpt-6.2-astra", "openai/gpt-6.2-astra-pro", "openai/gpt-6.2-sol", "openai/gpt-6.2-sol-pro", "google/gemini-4-neon", "thinkingmachines/inkling-2-mini", "radixark/daisyrose-27b"} {
		var request model.GeneralOpenAIRequest
		require.NoError(t, json.Unmarshal([]byte(`{"model":"`+id+`","messages":[{"role":"user","content":"Check availability."}],"reasoning_effort":"medium","tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","properties":{}}}}],"tool_choice":"auto"}`), &request))
		before, err := json.Marshal(&request)
		require.NoError(t, err)
		converted, err := (&openrouter.Adaptor{}).ConvertRequest(nil, 0, &request)
		require.NoError(t, err)
		after, err := json.Marshal(converted)
		require.NoError(t, err)
		require.JSONEq(t, string(before), string(after))
	}
}

// TestNovember22OpenRouterRefreshedParameters verifies removals and additions
// from the current host parameter lists. It accepts t and returns no values;
// absent advertised capabilities do not imply the native model lacks them.
func TestNovember22OpenRouterRefreshedParameters(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		id      string
		removed []string
		added   []string
	}{
		{"deepseek/deepseek-chat-v3-0324", []string{"logprobs", "top_logprobs", "seed"}, nil},
		{"deepseek/deepseek-v4-flash-vision-exp", []string{"logprobs", "top_logprobs"}, nil},
		{"minimax/minimax-m2.7", []string{"response_format", "structured_outputs"}, nil},
		{"meta-llama/llama-3.2-3b-instruct", []string{"min_p"}, nil},
		{"minimax/minimax-m2.5", []string{"logit_bias", "min_p", "repetition_penalty", "seed"}, nil},
		{"moonshotai/kimi-k2.7-code", []string{"min_p"}, nil},
		{"z-ai/glm-5.1", []string{"logit_bias", "min_p"}, nil},
		{"z-ai/glm-5.2", []string{"parallel_tool_calls"}, nil},
		{"qwen/qwen3.8-2.4t-a95b", nil, []string{"logit_bias"}},
		{"qwen/qwen3-coder-30b-a3b-instruct", nil, []string{"frequency_penalty", "response_format", "structured_outputs"}},
	} {
		cfg, exists := openrouter.ModelRatios[tc.id]
		require.True(t, exists, tc.id)
		for _, parameter := range tc.removed {
			require.NotContains(t, cfg.SupportedSamplingParameters, parameter, tc.id)
		}
		for _, parameter := range tc.added {
			require.Contains(t, cfg.SupportedSamplingParameters, parameter, tc.id)
		}
	}
	for _, id := range []string{"deepseek/deepseek-chat-v3-0324", "deepseek/deepseek-v4-flash-vision-exp"} {
		require.NotContains(t, openrouter.ModelRatios[id].SupportedFeatures, "logprobs", id)
	}
	require.Equal(t, []string{"tools", "reasoning"}, openrouter.ModelRatios["minimax/minimax-m2.7"].SupportedFeatures)
	require.Equal(t, []string{"tools", "json_mode", "structured_outputs"}, openrouter.ModelRatios["qwen/qwen3-coder-30b-a3b-instruct"].SupportedFeatures)
}
