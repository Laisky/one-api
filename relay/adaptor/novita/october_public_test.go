package novita_test

import (
	"encoding/json"
	"testing"

	"github.com/Laisky/one-api/relay/adaptor/novita"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/model"
	"github.com/stretchr/testify/require"
)

// TestOctoberNovitaPublishedModels pins published host prices and metadata on
// native and generic discovery using t. It returns nothing and performs no I/O.
func TestOctoberNovitaPublishedModels(t *testing.T) {
	t.Parallel()
	a := &novita.Adaptor{}
	generic := &openai.Adaptor{ChannelType: channeltype.Novita}
	for _, tc := range []struct {
		id                    string
		input, cached, output float64
		context, maxOutput    int32
	}{
		{"deepseek/deepseek-v4-pro-0813-p", 1.32, 0.044, 3.96, 1048576, 393216},
		{"deepseek/deepseek-v4.1-flash-dst", 0.3, 0.006, 1.2, 1048576, 393216},
		{"deepseek/deepseek-v4.1-flash-p", 0.3, 0.006, 1.2, 1048576, 393216},
		{"xiaomimimo/mimo-v2.6-flash", 0.14, 0.0028, 0.28, 1048576, 131072},
		{"xiaomimimo/mimo-v2.6-pro", 0.435, 0.0036, 0.87, 1048576, 131072},
		{"moonshotai/kimi-k3-p", 3, 0.3, 15, 1048576, 1048576},
		{"deepseek/deepseek-v4-flash-0731-p", 0.44, 0.0148, 1.32, 1048576, 393216},
		{"inclusionai/ling-3.0-flash-fin", 0.075, 0.015, 0.22, 262144, 32768},
		{"inclusionai/ling-3.0-flash-sante", 0.075, 0.015, 0.22, 262144, 32768},
	} {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			cfg, exists := a.GetDefaultModelPricing()[tc.id]
			require.True(t, exists)
			require.Contains(t, a.GetModelList(), tc.id)
			require.Contains(t, novita.ModelList, tc.id)
			require.Contains(t, generic.GetModelList(), tc.id)
			require.Equal(t, cfg, generic.GetDefaultModelPricing()[tc.id])
			require.InDelta(t, tc.input, a.GetModelRatio(tc.id)/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, tc.cached, cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, tc.output, a.GetModelRatio(tc.id)*a.GetCompletionRatio(tc.id)/ratio.MilliTokensUsd, 1e-12)
			require.Equal(t, tc.context, cfg.ContextLength)
			require.Equal(t, tc.maxOutput, cfg.MaxOutputTokens)
			require.Equal(t, []string{"text"}, cfg.OutputModalities)
			require.Contains(t, cfg.SupportedFeatures, "tools")
			require.Contains(t, cfg.SupportedFeatures, "reasoning")
			require.Empty(t, cfg.SupportedReasoningEfforts)
			require.Empty(t, cfg.TimeWindows)
			require.Empty(t, cfg.Tiers)
			require.Zero(t, cfg.MaxTokens, "metadata must not become an account restriction")
		})
	}
	require.Equal(t, []string{"text", "image", "video", "audio"}, novita.ModelRatios["xiaomimimo/mimo-v2.6-pro"].InputModalities)
	require.Equal(t, []string{"text", "image"}, novita.ModelRatios["deepseek/deepseek-v4.1-flash-p"].InputModalities)
	require.NotContains(t, novita.ModelRatios["inclusionai/ling-3.0-flash-fin"].SupportedFeatures, "structured_outputs")
	for _, id := range []string{"apodex/apodex-1.1-mini", "inclusionai/ling-3.1-flash", "bunny", "dev/glm46"} {
		require.NotContains(t, a.GetModelList(), id, "null pricing must not become free")
	}
}

// TestOctoberNovitaRequestAndNeighborPreservation checks new model/tool payloads
// without changing existing neighbors. It accepts t and returns no values.
func TestOctoberNovitaRequestAndNeighborPreservation(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"xiaomimimo/mimo-v2.6-pro", "deepseek/deepseek-v4.1-flash-p", "inclusionai/ling-3.0-flash-sante"} {
		var request model.GeneralOpenAIRequest
		body := `{"model":"` + id + `","messages":[{"role":"user","content":"Look up availability."}],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","properties":{}}}}],"tool_choice":"auto"}`
		require.NoError(t, json.Unmarshal([]byte(body), &request))
		before, err := json.Marshal(&request)
		require.NoError(t, err)
		converted, err := (&novita.Adaptor{}).ConvertRequest(nil, 0, &request)
		require.NoError(t, err)
		after, err := json.Marshal(converted)
		require.NoError(t, err)
		require.JSONEq(t, string(before), string(after))
	}
	cfg := novita.ModelRatios["zai-org/glm-5.3-flash"]
	require.InDelta(t, 0.15, cfg.Ratio/ratio.MilliTokensUsd, 1e-12)
	require.InDelta(t, 0.03, cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
	require.InDelta(t, 0.5, cfg.Ratio*cfg.CompletionRatio/ratio.MilliTokensUsd, 1e-12)
}
