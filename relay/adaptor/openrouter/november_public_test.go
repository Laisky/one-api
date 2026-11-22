package openrouter_test

import (
	"encoding/json"
	"testing"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/openrouter"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/model"
	"github.com/stretchr/testify/require"
)

// TestNovemberOpenRouterPublishedCatalog pins new model tariffs and host limits through
// native and generic discovery. Fixtures were read from https://openrouter.ai/api/v1/models;
// prices and limits were rechecked on 2026-11-22; scheduled models use peak base rates.
func TestNovemberOpenRouterPublishedCatalog(t *testing.T) {
	t.Parallel()
	a := &openrouter.Adaptor{}
	generic := &openai.Adaptor{ChannelType: channeltype.OpenRouter}
	for _, tc := range []struct {
		id                    string
		input, cached, output float64
		context, maxOutput    int32
	}{
		{"openai/gpt-6.2-astra", 8, 0.4, 40, 1050000, 128000},
		{"openai/gpt-6.2-astra-pro", 8, 0.4, 40, 1050000, 128000},
		{"openai/gpt-6.2-sol", 2, 0.1, 10, 1050000, 128000},
		{"openai/gpt-6.2-sol-pro", 2, 0.1, 10, 1050000, 128000},
		{"google/gemini-4-neon", 3, 0.15, 15, 1048576, 65536},
		{"thinkingmachines/inkling-2-mini", 0.075, 0.015, 0.3, 1048576, 65536},
		{"radixark/daisyrose-27b", 0.15, 0.0375, 1.875, 262144, 235929},
		{"mistralai/pixtral-large-2502", 2, 0, 6, 128000, 102400},
		{"qwen/qwen3.5-9b", 0.17, 0, 0.25, 262144, 235929},
		{"tencent/hy4-flash", 0.035, 0.0015, 0.1405, 1048576, 65536},
		{"qwen/qwen4-max", 1.425, 0.07125, 4.275, 1000000, 131072},
		{"inclusionai/ling-3.2-flash:free", 0, 0, 0, 1048576, 262144},
		{"google/gemini-4-hydrogen", 1.5, 0.075, 7.5, 1048576, 943718},
		{"baidu/ernie-5.5-flash-preview:free", 0, 0, 0, 1024000, 921600},
		{"baidu/ernie-light-dt-1.0-intl", 1.13, 0.113, 3.94, 1024000, 131072},
		{"google/gemini-4-carbon", 2, 0.1, 10, 1048576, 943718},
		{"nex-agi/nex-n2.6", 0.12, 0.024, 0.4, 262144, 235929},
		{"thinkingmachines/inkling-2-small", 0.3, 0.03, 1, 1048576, 943718},
		{"thinkingmachines/inkling-2-small:free", 0, 0, 0, 1048576, 131072},
		{"anthropic/claude-fable-5.2", 8, 0.2, 40, 1000000, 128000},
		{"meta/muse-ember", 0.1, 0.004, 0.5, 1048576, 943718},
		{"liquid/lfm-3-1.7b", 0.08, 0.02, 0.2, 262144, 8192},
		{"liquid/lfm-3-1.7b:free", 0, 0, 0, 262144, 8192},
		{"kt/midm-k-3.0-pro", 0.2, 0.04, 0.8, 261795, 235615},
		{"ricursiveai/ripple-65b-preview", 0.1, 0.025, 0.3, 1048576, 131072},
		{"tencent/hy4", 0.417, 0.021, 1.2505, 1048576, 65536},
		{"motif-technologies/motif-3", 0.3, 0.06, 1.2, 262144, 235929},
		{"z-ai/glm-5.3-flashx-legal", 0.37, 0.075, 1.25, 1048576, 131072},
		{"particle-ai/particle-1", 0.07, 0.006, 0.5, 1048576, 131072},
		{"particle-ai/particle-1:free", 0, 0, 0, 1048576, 131072},
		{"openai/gpt-6.1-terra-pro", 1, 0.05, 5, 1050000, 128000},
		{"openai/gpt-6.1-terra", 1, 0.05, 5, 1050000, 128000},
		{"moonshotai/kimi-k3.1-pro", 0.253, 0.18, 8.8, 1048576, 943718},
		{"recursive/flint-v1.0:free", 0, 0, 0, 262144, 235929},
		{"minimax/minimax-m3.1-flash", 0.15, 0.015, 0.6, 1048576, 524288},
		{"deepseek/deepseek-v4.1-flash-1017", 0.3, 0.006, 1.2, 1048576, 393216},
		{"elorian/glimpse", 0.6, 0.06, 3, 262144, 256000},
		{"reflection/beam-1.0-501b-a23b", 0.25, 0.05, 1, 1048576, 131072},
		{"prism-ml/ternary-bonsai-2-9b", 0.08, 0.04, 0.13, 262144, 65536},
		{"moondream/moondream3-preview", 0.3, 0, 2.5, 32000, 4096},
		{"moondream/moondream3.1-9b-a2b", 0.3, 0, 2.5, 32000, 4096},
		{"meta/muse-spark-open", 1.25, 0.15, 4.25, 1048576, 943718},
		{"nvidia/nemotron-3.5-super-vl", 0.15, 0.02, 0.47, 1048576, 943718},
		{"nvidia/nemotron-3.5-super-vl:free", 0, 0, 0, 1000000, 131072},
		{"unbiased/pareto-26.10", 0.8, 0.03, 3.2, 1048576, 131072},
		{"~unbiased/pareto-latest", 0.8, 0.03, 3.2, 1048576, 131072},
		{"unbiased/pareto-26.9", 2.5, 0.25, 7.5, 262144, 131072},
		{"liquid/lfm-2.5-24b-a2b", 0.05, 0.025, 0.7, 128000, 8192},
		{"google/gemini-4-argon", 2, 0.1, 10, 1048576, 943718},
		{"scaledown/summarize", 0.04, 0, 0, 1000000, 4000},
		{"scaledown/extract", 0.04, 0, 0, 1000000, 900000},
		{"inclusionai/ling-3.0-tiny", 0.015, 0.003, 0.045, 262144, 250000},
	} {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			cfg, ok := a.GetDefaultModelPricing()[tc.id]
			require.True(t, ok)
			require.Contains(t, a.GetModelList(), tc.id)
			require.Contains(t, generic.GetModelList(), tc.id)
			require.Equal(t, cfg, generic.GetDefaultModelPricing()[tc.id])
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

// TestNovemberOpenRouterRequestPreservation verifies each new model ID and a tool-bearing
// request survive the provider converter without account-level catalogue filtering.
func TestNovemberOpenRouterRequestPreservation(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"tencent/hy4-flash", "qwen/qwen4-max", "inclusionai/ling-3.2-flash:free", "google/gemini-4-hydrogen", "baidu/ernie-5.5-flash-preview:free", "baidu/ernie-light-dt-1.0-intl", "google/gemini-4-carbon", "nex-agi/nex-n2.6", "thinkingmachines/inkling-2-small", "thinkingmachines/inkling-2-small:free", "anthropic/claude-fable-5.2", "meta/muse-ember", "liquid/lfm-3-1.7b", "liquid/lfm-3-1.7b:free", "kt/midm-k-3.0-pro", "ricursiveai/ripple-65b-preview", "tencent/hy4", "motif-technologies/motif-3", "z-ai/glm-5.3-flashx-legal", "particle-ai/particle-1", "particle-ai/particle-1:free", "openai/gpt-6.1-terra-pro", "openai/gpt-6.1-terra", "moonshotai/kimi-k3.1-pro", "recursive/flint-v1.0:free", "minimax/minimax-m3.1-flash", "deepseek/deepseek-v4.1-flash-1017", "elorian/glimpse", "reflection/beam-1.0-501b-a23b", "prism-ml/ternary-bonsai-2-9b", "moondream/moondream3-preview", "moondream/moondream3.1-9b-a2b", "meta/muse-spark-open", "nvidia/nemotron-3.5-super-vl", "nvidia/nemotron-3.5-super-vl:free", "unbiased/pareto-26.10", "~unbiased/pareto-latest", "unbiased/pareto-26.9", "liquid/lfm-2.5-24b-a2b", "google/gemini-4-argon", "scaledown/summarize", "scaledown/extract", "inclusionai/ling-3.0-tiny"} {
		var request model.GeneralOpenAIRequest
		require.NoError(t, json.Unmarshal([]byte(`{"model":"`+id+`","messages":[{"role":"user","content":"Look up availability."}],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","properties":{}}}}],"tool_choice":"auto"}`), &request))
		before, err := json.Marshal(&request)
		require.NoError(t, err)
		converted, err := (&openrouter.Adaptor{}).ConvertRequest(nil, 0, &request)
		require.NoError(t, err)
		after, err := json.Marshal(converted)
		require.NoError(t, err)
		require.JSONEq(t, string(before), string(after))
	}
}
