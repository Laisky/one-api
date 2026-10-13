package nvidia_test

import (
	"encoding/json"
	"github.com/Laisky/one-api/relay/adaptor/nvidia"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/stretchr/testify/require"
	"testing"
)

// TestOctoberNvidiaGLM verifies exact trial IDs, model-specific controls and
// payload preservation through public methods using t. It returns nothing.
func TestOctoberNvidiaGLM(t *testing.T) {
	t.Parallel()
	a := &nvidia.Adaptor{}
	for _, id := range []string{"z-ai/glm-5.3", "z-ai/glm-5.3-flash"} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			cfg, ok := a.GetDefaultModelPricing()[id]
			require.True(t, ok)
			require.Contains(t, a.GetModelList(), id)
			require.Contains(t, nvidia.ModelList, id)
			require.Zero(t, a.GetModelRatio(id))
			require.EqualValues(t, 1, a.GetCompletionRatio(id))
			require.EqualValues(t, 1048576, cfg.ContextLength)
			require.Zero(t, cfg.MaxOutputTokens)
			require.Equal(t, []string{"low", "high", "max"}, cfg.SupportedReasoningEfforts)
			require.Equal(t, "max", cfg.DefaultReasoningEffort)
			require.Contains(t, cfg.SupportedFeatures, "tools")
			require.Contains(t, cfg.SupportedFeatures, "reasoning")
			if id == "z-ai/glm-5.3" {
				require.Equal(t, "fp4", cfg.Quantization)
				require.Equal(t, []string{"text"}, cfg.InputModalities)
			} else {
				require.Equal(t, "fp8", cfg.Quantization)
				require.Equal(t, []string{"text", "image"}, cfg.InputModalities)
			}
			for _, effort := range cfg.SupportedReasoningEfforts {
				var req model.GeneralOpenAIRequest
				require.NoError(t, json.Unmarshal([]byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"Inspect"},{"type":"image_url","image_url":{"url":"https://example.test/image.png"}}]}],"tools":[{"type":"function","function":{"name":"record","parameters":{"type":"object"}}}],"tool_choice":"auto"}`), &req))
				req.Model = id
				req.ReasoningEffort = &effort
				before, err := json.Marshal(req)
				require.NoError(t, err)
				out, err := a.ConvertRequest(nil, 0, &req)
				require.NoError(t, err)
				after, err := json.Marshal(out)
				require.NoError(t, err)
				require.JSONEq(t, string(before), string(after))
			}
			url, err := a.GetRequestURL(&meta.Meta{BaseURL: "https://integrate.api.nvidia.com/v1", RequestURLPath: "/v1/chat/completions"})
			require.NoError(t, err)
			require.Equal(t, "https://integrate.api.nvidia.com/v1/chat/completions", url)
		})
	}
	effort := "high"
	legacy := &model.GeneralOpenAIRequest{Model: "nvidia/nemotron-3-super-120b-a12b", ReasoningEffort: &effort}
	_, err := a.ConvertRequest(nil, 0, legacy)
	require.NoError(t, err)
	require.Nil(t, legacy.ReasoningEffort)
	require.Greater(t, a.GetModelRatio("unknown-hosted-model"), 0.0)
	require.NotContains(t, a.GetModelList(), "z-ai/glm-5-3")
}
