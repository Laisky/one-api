package nvidia

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestNovemberHostedModels verifies provider-specific context limits and trial
// discovery without copying model-creator prices or unsupported capabilities.
// It accepts t and returns no values.
func TestNovemberHostedModels(t *testing.T) {
	t.Parallel()
	a := &Adaptor{}
	for id, context := range map[string]int32{
		"mistralai/mistral-large-4-1t-a52b":      262_144,
		"nvidia/nemotron-3.5-bolt-e4b":           65_536,
		"minimaxai/minimax-m3.1-flash":           262_144,
		"deepseek-ai/deepseek-v4.1-flash":        1_048_576,
		"nvidia/nemotron-3.5-super-vl-120b-a12b": 262_144,
		"nvidia/nemotron-3.5-lightning-30b-a3b":  1_048_576,
		"stepfun-ai/step-5-preview":              1_048_576,
		"moonshotai/kimi-k3.1-pro":               1_048_576,
		"meta/muse-glimmer-30b":                  131_072,
	} {
		require.Contains(t, a.GetModelList(), id)
		require.Contains(t, ModelList, id)
		cfg := a.GetDefaultModelPricing()[id]
		require.Equal(t, context, cfg.ContextLength)
		require.Zero(t, a.GetModelRatio(id))
		require.Zero(t, cfg.MaxOutputTokens)
		require.Zero(t, cfg.MaxTokens)
		require.Contains(t, cfg.Description, "Free trial endpoint")
	}
	require.NotContains(t, a.GetModelList(), "moonshotai/kimi-k3_1-pro")
	require.NotContains(t, a.GetDefaultModelPricing()["stepfun-ai/step-5-preview"].SupportedFeatures, "structured_outputs")
	require.NotContains(t, a.GetDefaultModelPricing()["meta/muse-glimmer-30b"].SupportedFeatures, "structured_outputs")
	one, two := novemberModels(), novemberModels()
	one["moonshotai/kimi-k3.1-pro"].SupportedReasoningEfforts[0] = "changed"
	require.Equal(t, "low", two["moonshotai/kimi-k3.1-pro"].SupportedReasoningEfforts[0])
}

// TestNovemberHostedEffortWire verifies documented NVIDIA vocabularies survive
// real conversion, while native boolean/numeric reasoning models retain the
// previous effort omission. It accepts t and returns no values.
func TestNovemberHostedEffortWire(t *testing.T) {
	t.Parallel()
	for id, efforts := range map[string][]string{
		"mistralai/mistral-large-4-1t-a52b":      {"none", "high"},
		"nvidia/nemotron-3.5-super-vl-120b-a12b": {"none", "low", "high"},
		"moonshotai/kimi-k3.1-pro":               {"low", "medium", "high", "max"},
	} {
		require.Equal(t, efforts, ModelRatios[id].SupportedReasoningEfforts)
		for _, effort := range efforts {
			request := &model.GeneralOpenAIRequest{Model: id, ReasoningEffort: &effort, Messages: []model.Message{{Role: "user", Content: "Hello"}}}
			converted, err := (&Adaptor{}).ConvertRequest(nil, 0, request)
			require.NoError(t, err)
			encoded, err := json.Marshal(converted)
			require.NoError(t, err)
			var wire map[string]any
			require.NoError(t, json.Unmarshal(encoded, &wire))
			require.Equal(t, id, wire["model"])
			require.Equal(t, effort, wire["reasoning_effort"])
		}
	}
	effort := "high"
	for _, id := range []string{"nvidia/nemotron-3.5-bolt-e4b", "nvidia/nemotron-3.5-lightning-30b-a3b", "deepseek-ai/deepseek-v4.1-flash", "stepfun-ai/step-5-preview"} {
		request := &model.GeneralOpenAIRequest{Model: id, ReasoningEffort: &effort, ExtraBody: map[string]any{"chat_template_kwargs": map[string]any{"enable_thinking": true}}}
		converted, err := (&Adaptor{}).ConvertRequest(nil, 0, request)
		require.NoError(t, err)
		encoded, err := json.Marshal(converted)
		require.NoError(t, err)
		var wire map[string]any
		require.NoError(t, json.Unmarshal(encoded, &wire))
		require.NotContains(t, wire, "reasoning_effort")
		require.Equal(t, map[string]any{"chat_template_kwargs": map[string]any{"enable_thinking": true}}, wire["extra_body"], "retain native controls for the shared passthrough merger")
	}
}
