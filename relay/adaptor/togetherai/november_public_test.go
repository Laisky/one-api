package togetherai_test

import (
	"encoding/json"
	"testing"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/togetherai"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/model"
	"github.com/stretchr/testify/require"
)

// TestNovemberTogetherPublishedCatalog pins new model tariffs and host limits through
// native and generic discovery. Fixtures were read from https://docs.together.ai/docs/serverless/models.
func TestNovemberTogetherPublishedCatalog(t *testing.T) {
	t.Parallel()
	a := &togetherai.Adaptor{}
	generic := &openai.Adaptor{ChannelType: channeltype.TogetherAI}
	for _, tc := range []struct {
		id                    string
		input, cached, output float64
		context, maxOutput    int32
	}{
		{"Qwen/Qwen4-Max", 1.425, 0.07125, 4.275, 1000000, 0},
		{"tencent/Hy4", 0.834, 0.042, 2.501, 1048576, 0},
		{"deepseek-ai/DeepSeek-V4.1-Flash-1017", 0.3, 0.006, 1.2, 1000000, 0},
		{"elorian-labs/glimpse-1.0", 0.6, 0.06, 3, 262144, 0},
		{"meta-models/Muse-Spark-Open-816B-A42B", 1.25, 0.15, 4.25, 1048576, 0},
		{"MiniMaxAI/MiniMax-M3.1-Flash", 0.15, 0.015, 0.6, 1048576, 0},
		{"moonshotai/Kimi-K3.1-Pro", 2, 0.2, 10, 1048576, 0},
		{"nvidia/NVIDIA-Nemotron-3.5-Super-VL", 0.15, 0.02, 0.5, 262144, 0},
		{"reflection/Beam-1.0-501B-A23B", 0.4, 0.04, 1.2, 1048576, 0},
		{"ricursive-ai/Ripple-65B-Preview", 0.2, 0.02, 0.6, 131072, 0},
		{"thinkingmachines/Inkling-2-Small", 0.3, 0.03, 1, 1048576, 0},
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

// TestNovemberTogetherRequestPreservation verifies each new model ID and a tool-bearing
// request survive the provider converter without account-level catalogue filtering.
func TestNovemberTogetherRequestPreservation(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"Qwen/Qwen4-Max", "tencent/Hy4", "deepseek-ai/DeepSeek-V4.1-Flash-1017", "elorian-labs/glimpse-1.0", "meta-models/Muse-Spark-Open-816B-A42B", "MiniMaxAI/MiniMax-M3.1-Flash", "moonshotai/Kimi-K3.1-Pro", "nvidia/NVIDIA-Nemotron-3.5-Super-VL", "reflection/Beam-1.0-501B-A23B", "ricursive-ai/Ripple-65B-Preview", "thinkingmachines/Inkling-2-Small"} {
		var request model.GeneralOpenAIRequest
		require.NoError(t, json.Unmarshal([]byte(`{"model":"`+id+`","messages":[{"role":"user","content":"Look up availability."}],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","properties":{}}}}],"tool_choice":"auto"}`), &request))
		before, err := json.Marshal(&request)
		require.NoError(t, err)
		converted, err := (&togetherai.Adaptor{}).ConvertRequest(nil, 0, &request)
		require.NoError(t, err)
		after, err := json.Marshal(converted)
		require.NoError(t, err)
		require.JSONEq(t, string(before), string(after))
	}
}
