package deepinfra_test

import (
	"encoding/json"
	"testing"

	"github.com/Laisky/one-api/relay"
	"github.com/Laisky/one-api/relay/adaptor/deepinfra"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/model"
	"github.com/stretchr/testify/require"
)

// TestNovemberDeepInfraPublishedCatalog pins new model tariffs and host limits through
// native and registered adaptor discovery. Fixtures were read from https://api.deepinfra.com/models/list;
// output limits and cached decimals were rechecked on 2026-11-22 against
// https://api.deepinfra.com/models/<model> (max_output_tokens) and https://deepinfra.com/<model>.
func TestNovemberDeepInfraPublishedCatalog(t *testing.T) {
	t.Parallel()
	a := &deepinfra.Adaptor{}
	registered := relay.GetAdaptor(apitype.DeepInfra)
	require.IsType(t, &deepinfra.Adaptor{}, registered)
	for _, tc := range []struct {
		id                    string
		input, cached, output float64
		context, maxOutput    int32
	}{
		{"RadixArk/DaisyRose-27B", 0.15, 0.0375, 1.875, 262144, 0},
		{"tencent/Hy4", 0.834, 0.04199999814, 2.501, 1048576, 131072},
		{"deepseek-ai/DeepSeek-V4.1-Flash-1017", 0.098, 0.00294, 0.294, 1048576, 393216},
		{"Qwen/Qwen4-Max", 1.413, 0.071, 4.24, 1000000, 0},
		{"moonshotai/Kimi-K3.1-Pro", 1.9, 0.19, 9.5, 1048576, 943718},
		{"nvidia/NVIDIA-Nemotron-3.5-Super-VL", 0.15, 0.02, 0.5, 262144, 0},
		{"thinkingmachines/Inkling-2-Small", 0.285, 0.0285, 0.95, 1048576, 0},
	} {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			cfg, ok := a.GetDefaultModelPricing()[tc.id]
			require.True(t, ok)
			require.Contains(t, a.GetModelList(), tc.id)
			require.Contains(t, registered.GetModelList(), tc.id)
			require.Equal(t, cfg, registered.GetDefaultModelPricing()[tc.id])
			require.InDelta(t, tc.input, a.GetModelRatio(tc.id)/ratio.MilliTokensUsd, 1e-10)
			require.InDelta(t, tc.cached, cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-10)
			require.InDelta(t, tc.output, a.GetModelRatio(tc.id)*a.GetCompletionRatio(tc.id)/ratio.MilliTokensUsd, 1e-10)
			require.Equal(t, tc.context, cfg.ContextLength)
			require.Equal(t, tc.maxOutput, cfg.MaxOutputTokens)
			require.Equal(t, []string{"text"}, cfg.OutputModalities)
			require.Zero(t, cfg.MaxTokens, "published context must not impose an invented per-request cap")
		})
	}
}

// TestNovemberDeepInfraRequestPreservation verifies each new model ID and a tool-bearing
// request survive the provider converter without account-level catalogue filtering.
func TestNovemberDeepInfraRequestPreservation(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"RadixArk/DaisyRose-27B", "tencent/Hy4", "deepseek-ai/DeepSeek-V4.1-Flash-1017", "Qwen/Qwen4-Max", "moonshotai/Kimi-K3.1-Pro", "nvidia/NVIDIA-Nemotron-3.5-Super-VL", "thinkingmachines/Inkling-2-Small"} {
		var request model.GeneralOpenAIRequest
		require.NoError(t, json.Unmarshal([]byte(`{"model":"`+id+`","messages":[{"role":"user","content":"Look up availability."}],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","properties":{}}}}],"tool_choice":"auto"}`), &request))
		before, err := json.Marshal(&request)
		require.NoError(t, err)
		converted, err := (&deepinfra.Adaptor{}).ConvertRequest(nil, 0, &request)
		require.NoError(t, err)
		after, err := json.Marshal(converted)
		require.NoError(t, err)
		require.JSONEq(t, string(before), string(after))
	}
}
