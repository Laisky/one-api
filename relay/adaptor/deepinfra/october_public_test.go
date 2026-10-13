package deepinfra_test

import (
	"encoding/json"
	"testing"

	"github.com/Laisky/one-api/relay/adaptor/deepinfra"
	"github.com/Laisky/one-api/relay/adaptor/novita"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/model"
	"github.com/stretchr/testify/require"
)

// TestOctoberDeepInfraPublishedPrices checks the provider's native input, cache,
// and output prices using t. It returns no values and performs no network I/O.
func TestOctoberDeepInfraPublishedPrices(t *testing.T) {
	t.Parallel()
	a := &deepinfra.Adaptor{}
	for _, tc := range []struct {
		id                    string
		input, cached, output float64
	}{
		{"anthropic/claude-sonnet-5-5", 2, 0, 10},
		{"tencent/Hy4-preview", 0.834, 0.834 * 0.05035971, 2.501},
		{"XiaomiMiMo/MiMo-V2.6-Pro", 0.43, 0.43 * 0.00837209, 0.87},
		{"XiaomiMiMo/MiMo-V2.6-Flash", 0.14, 0.0028, 0.28},
		{"nvidia/NVIDIA-Nemotron-3.5-Lightning", 0.06, 0.03, 0.16},
		{"google/gemini-3.5-flash", 1.5, 0, 9},
		{"mistralai/Mistral-Nemo-Instruct-2407", 0.029, 0, 0.03},
		{"Qwen/Qwen3.6-35B-A3B", 0.1, 0.1, 0.95},
		{"google/gemma-4-31B-it", 0.2, 0, 0.4},
		{"google/gemini-3.7-flash", 0.75, 0, 3.75},
	} {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			cfg, exists := a.GetDefaultModelPricing()[tc.id]
			require.True(t, exists)
			require.Contains(t, a.GetModelList(), tc.id)
			require.InDelta(t, tc.input, a.GetModelRatio(tc.id)/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, tc.cached, cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, tc.output, a.GetModelRatio(tc.id)*a.GetCompletionRatio(tc.id)/ratio.MilliTokensUsd, 1e-12)
			require.Empty(t, cfg.Tiers)
			require.Empty(t, cfg.TimeWindows)
		})
	}
	require.InDelta(t, 0.435, novita.ModelRatios["xiaomimimo/mimo-v2.6-pro"].Ratio/ratio.MilliTokensUsd, 1e-12, "Novita is a different host tariff")
}

// TestOctoberDeepInfraDiscountPreservation pins existing discounted prices so
// refreshing undiscounted rows cannot erase or double-apply discounts using t.
func TestOctoberDeepInfraDiscountPreservation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		id                    string
		input, cached, output float64
	}{
		{"zai-org/GLM-5.3-Flash", 0.075, 0.015, 0.25},
		{"deepseek-ai/DeepSeek-V4.1-Flash", 0.14, 0.0042, 0.42},
		{"Qwen/Qwen3.8-27B", 0.15, 0.0375, 1.875},
		{"zai-org/GLM-5.3", 0.5625, 0.12499999875, 2.5},
		{"zai-org/GLM-5.2", 0.5625, 0.105000001875, 1.8},
		{"deepseek-ai/DeepSeek-V4-Flash-Vision-Exp", 0.2156, 0.006859999608, 0.6468},
	} {
		cfg, exists := deepinfra.ModelRatios[tc.id]
		require.True(t, exists)
		require.InDelta(t, tc.input, cfg.Ratio/ratio.MilliTokensUsd, 1e-12)
		require.InDelta(t, tc.cached, cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
		require.InDelta(t, tc.output, cfg.Ratio*cfg.CompletionRatio/ratio.MilliTokensUsd, 1e-12)
	}
}

// TestOctoberDeepInfraNewCapabilities checks bounded published metadata and
// unchanged text/tool conversion using t. It returns no values.
func TestOctoberDeepInfraNewCapabilities(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		id              string
		context, output int32
		inputs          []string
	}{
		{"anthropic/claude-sonnet-5-5", 1000000, 128000, []string{"text", "image"}},
		{"tencent/Hy4-preview", 1048576, 0, []string{"text"}},
		{"XiaomiMiMo/MiMo-V2.6-Pro", 1048576, 0, []string{"text", "image", "video", "audio"}},
		{"XiaomiMiMo/MiMo-V2.6-Flash", 1048576, 0, []string{"text", "image", "video", "audio"}},
	} {
		cfg, exists := deepinfra.ModelRatios[tc.id]
		require.True(t, exists)
		require.Equal(t, tc.context, cfg.ContextLength)
		require.Equal(t, tc.output, cfg.MaxOutputTokens)
		require.Equal(t, tc.inputs, cfg.InputModalities)
		require.Equal(t, []string{"text"}, cfg.OutputModalities)
		require.Empty(t, cfg.SupportedReasoningEfforts)
		require.Empty(t, cfg.SupportedSamplingParameters)
		require.Contains(t, cfg.SupportedFeatures, "tools")
		require.Zero(t, cfg.MaxTokens)
		var request model.GeneralOpenAIRequest
		require.NoError(t, json.Unmarshal([]byte(`{"model":"`+tc.id+`","messages":[{"role":"user","content":"Look up availability."}],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","properties":{}}}}],"tool_choice":"auto"}`), &request))
		before, err := json.Marshal(&request)
		require.NoError(t, err)
		converted, err := (&deepinfra.Adaptor{}).ConvertRequest(nil, 0, &request)
		require.NoError(t, err)
		after, err := json.Marshal(converted)
		require.NoError(t, err)
		require.JSONEq(t, string(before), string(after))
	}
	require.Zero(t, deepinfra.ModelRatios["anthropic/claude-sonnet-5-5"].CachedInputRatio, "unpublished is unspecified, not free or an inferred native discount")
}
