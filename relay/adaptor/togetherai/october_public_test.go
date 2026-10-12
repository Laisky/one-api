package togetherai_test

import (
	"encoding/json"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/togetherai"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/model"
	"github.com/stretchr/testify/require"
	"testing"
)

// TestOctoberTogetherPublishedRates independently pins all published current
// chat tariffs through native and generic discovery using t; it returns nothing.
func TestOctoberTogetherPublishedRates(t *testing.T) {
	t.Parallel()
	a := &togetherai.Adaptor{}
	generic := &openai.Adaptor{ChannelType: channeltype.TogetherAI}
	for _, tc := range []struct {
		id                    string
		input, cached, output float64
	}{
		{"thinkingmachines/Inkling", 1, .17, 4.05},
		{"MiniMaxAI/MiniMax-M3", .3, .06, 1.2},
		{"Qwen/Qwen3.8-2.4T-A95B", 2, .5, 6},
		{"Qwen/Qwen3.7-Max", 2.5, .5, 7.5},
		{"Qwen/Qwen3.6-Plus", .5, 0, 3},
		{"Qwen/Qwen3.5-9B", .17, 0, .25},
		{"moonshotai/Kimi-K3", 2.7, .3, 13.5},
		{"zai-org/GLM-5.3", 1.4, .26, 4.4},
		{"zai-org/GLM-5.3-Flash", .15, .03, .5},
		{"zai-org/GLM-5.2", 1.4, .26, 4.4},
		{"openai/gpt-oss-120b", .15, 0, .6},
		{"deepseek-ai/DeepSeek-V4-Pro-0813", 1.32, .13, 3.96},
		{"Qwen/Qwen3.7-Plus", .32, 0, 1.28},
		{"Prism-ML/Ternary-Bonsai-27B", 0, 0, 0},
		{"meta-models/Muse-Glimmer-30B", .35, .04, 1.5},
		{"Qwen/Qwen3.8-Flash", .15, 0, .47},
		{"deepseek-ai/DeepSeek-V4.1-Flash", .3, .006, 1.2},
		{"together/Tev1-4B-experimental", .042, 0, 0},
	} {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			cfg, ok := a.GetDefaultModelPricing()[tc.id]
			require.True(t, ok)
			require.Contains(t, a.GetModelList(), tc.id)
			require.Contains(t, generic.GetModelList(), tc.id)
			require.Equal(t, cfg, generic.GetDefaultModelPricing()[tc.id])
			require.InDelta(t, tc.input, a.GetModelRatio(tc.id)/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, tc.output, a.GetModelRatio(tc.id)*a.GetCompletionRatio(tc.id)/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, tc.cached, cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
			require.Empty(t, cfg.TimeWindows)
			require.Empty(t, cfg.Tiers)
			if tc.id != "together/Tev1-4B-experimental" {
				require.Contains(t, cfg.SupportedFeatures, "tools")
				require.Contains(t, cfg.SupportedFeatures, "structured_outputs")
			}
		})
	}
}

// TestOctoberTogetherSpeechUnits checks character prices and independent audio
// metadata without mistaking duration or model tokens for input characters.
func TestOctoberTogetherSpeechUnits(t *testing.T) {
	t.Parallel()
	for id, price := range map[string]float64{"hexgrad/Kokoro-82M": 4, "canopylabs/orpheus-3b-0.1-ft": 15, "cartesia/sonic": 65, "cartesia/sonic-2": 65, "cartesia/sonic-3": 65} {
		cfg := togetherai.ModelRatios[id]
		require.NotNil(t, cfg.Audio)
		require.Equal(t, "characters", cfg.Audio.InputUnit)
		require.EqualValues(t, 1e6, cfg.Audio.InputPriceQuantity)
		require.Equal(t, price, cfg.Audio.InputPriceUsd)
		require.InDelta(t, price, cfg.Ratio/ratio.MilliTokensUsd, 1e-12)
		require.Equal(t, []string{"text"}, cfg.InputModalities)
		require.Equal(t, []string{"audio"}, cfg.OutputModalities)
		clone := cfg.Clone()
		clone.Audio.InputPriceUsd = 999
		require.Equal(t, price, cfg.Audio.InputPriceUsd)
	}
}

// TestOctoberTogetherMetadataAndRequestPreservation checks host modalities and
// quantization without modifying chat/tool requests; it accepts t and returns nothing.
func TestOctoberTogetherMetadataAndRequestPreservation(t *testing.T) {
	t.Parallel()
	require.Equal(t, "fp4", togetherai.ModelRatios["MiniMaxAI/MiniMax-M3"].Quantization)
	for _, id := range []string{"moonshotai/Kimi-K3", "MiniMaxAI/MiniMax-M3", "Qwen/Qwen3.5-9B"} {
		require.Contains(t, togetherai.ModelRatios[id].InputModalities, "image")
	}
	for _, id := range []string{"Qwen/Qwen3.8-Flash", "moonshotai/Kimi-K3"} {
		var req model.GeneralOpenAIRequest
		require.NoError(t, json.Unmarshal([]byte(`{"model":"`+id+`","messages":[{"role":"user","content":"Look up availability."}],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","properties":{}}}}],"tool_choice":"auto"}`), &req))
		before, err := json.Marshal(&req)
		require.NoError(t, err)
		converted, err := (&togetherai.Adaptor{}).ConvertRequest(nil, 0, &req)
		require.NoError(t, err)
		after, err := json.Marshal(converted)
		require.NoError(t, err)
		require.JSONEq(t, string(before), string(after))
	}
}
