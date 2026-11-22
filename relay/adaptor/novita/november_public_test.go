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

// TestNovemberNovitaPublishedCatalog pins new model tariffs and host limits through
// native and generic discovery. Fixtures were read from https://api.novita.ai/v3/openai/models.
func TestNovemberNovitaPublishedCatalog(t *testing.T) {
	t.Parallel()
	a := &novita.Adaptor{}
	generic := &openai.Adaptor{ChannelType: channeltype.Novita}
	for _, tc := range []struct {
		id                    string
		input, cached, output float64
		context, maxOutput    int32
	}{
		{"tencent/hy4-flash", 0.035, 0.0015, 0.1405, 1000000, 64000},
		{"tencent/hy4", 0.417, 0.021, 1.2505, 1000000, 64000},
		{"deepseek/deepseek-v4.1-flash-1017", 0.3, 0.006, 1.2, 1048576, 393216},
		{"zai-org/glm-5.3-flashX", 0.37, 0.075, 1.25, 1048576, 131072},
		{"moonshotai/kimi-k3.1-pro-highspeed", 4, 0.2, 20, 1048576, 1048576},
		{"moonshotai/kimi-k3.1-pro", 2, 0.2, 10, 1048576, 1048576},
		{"minimax/minimax-m3.1-flash", 0.15, 0.015, 0.6, 1000000, 524288},
		{"qwen/qwen4-max", 1.413, 0.071, 4.24, 1000000, 131000},
		{"stepfun/step-5-preview", 1, 0.1, 2.7, 1000000, 64000},
		{"inclusionai/ling-3.1-flash", 0.3, 0.06, 0.9, 1000000, 262144},
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

// TestNovemberNovitaRequestPreservation verifies each new model ID and a tool-bearing
// request survive the provider converter without account-level catalogue filtering.
func TestNovemberNovitaRequestPreservation(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"tencent/hy4-flash", "tencent/hy4", "deepseek/deepseek-v4.1-flash-1017", "zai-org/glm-5.3-flashX", "moonshotai/kimi-k3.1-pro-highspeed", "moonshotai/kimi-k3.1-pro", "minimax/minimax-m3.1-flash", "qwen/qwen4-max", "stepfun/step-5-preview", "inclusionai/ling-3.1-flash"} {
		var request model.GeneralOpenAIRequest
		require.NoError(t, json.Unmarshal([]byte(`{"model":"`+id+`","messages":[{"role":"user","content":"Look up availability."}],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","properties":{}}}}],"tool_choice":"auto"}`), &request))
		before, err := json.Marshal(&request)
		require.NoError(t, err)
		converted, err := (&novita.Adaptor{}).ConvertRequest(nil, 0, &request)
		require.NoError(t, err)
		after, err := json.Marshal(converted)
		require.NoError(t, err)
		require.JSONEq(t, string(before), string(after))
	}
}
