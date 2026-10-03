package zai_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/zai"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

// TestGLM53FlashXPricing verifies independently published USD rates and metadata
// through the adaptor interface using t; it returns nothing.
func TestGLM53FlashXPricing(t *testing.T) {
	t.Parallel()

	const id = "glm-5.3-flashx"
	var a adaptor.Adaptor = &zai.Adaptor{}
	require.Contains(t, a.GetModelList(), id)
	cfg, exists := a.GetDefaultModelPricing()[id]
	require.True(t, exists)
	require.InDelta(t, 0.37, a.GetModelRatio(id)/ratio.MilliTokensUsd, 1e-12)
	require.InDelta(t, 1.25, a.GetModelRatio(id)*a.GetCompletionRatio(id)/ratio.MilliTokensUsd, 1e-12)
	require.InDelta(t, 0.075, cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
	require.Empty(t, cfg.Tiers)
	require.Empty(t, cfg.TimeWindows, "FlashX must not inherit Flash's launch discount")
	require.Zero(t, cfg.CacheWrite5mRatio)
	require.Zero(t, cfg.CacheWrite1hRatio)
	require.EqualValues(t, 1_000_000, cfg.ContextLength)
	require.EqualValues(t, 131_072, cfg.MaxOutputTokens)
	require.ElementsMatch(t, []string{"text", "image", "video", "file"}, cfg.InputModalities)
	require.Equal(t, []string{"text"}, cfg.OutputModalities)
	require.Equal(t, []string{"low", "high", "max"}, cfg.SupportedReasoningEfforts)
	require.Equal(t, "max", cfg.DefaultReasoningEffort)
	require.Contains(t, cfg.SupportedFeatures, "json_mode")
	require.NotContains(t, cfg.SupportedFeatures, "structured_outputs", "JSON mode is not strict json_schema support")
	require.NotEqual(t, a.GetModelRatio("glm-5.3-flash"), a.GetModelRatio(id))
}

// TestGLM53FlashXWireCompatibility verifies the v4 endpoint and preserves the
// exact model ID, image payload, and supported efforts using t; it returns nothing.
func TestGLM53FlashXWireCompatibility(t *testing.T) {
	t.Parallel()

	for _, effort := range []string{"low", "high", "max"} {
		t.Run(effort, func(t *testing.T) {
			t.Parallel()
			const input = `{"model":"glm-5.3-flashx","messages":[{"role":"user","content":[{"type":"text","text":"Describe this image."},{"type":"image_url","image_url":{"url":"https://example.com/image.png"}}]}]}`
			var req model.GeneralOpenAIRequest
			require.NoError(t, json.Unmarshal([]byte(input), &req))
			req.ReasoningEffort = &effort
			a := &zai.Adaptor{}
			converted, err := a.ConvertRequest(nil, 0, &req)
			require.NoError(t, err)
			encoded, err := json.Marshal(converted)
			require.NoError(t, err)
			var want, got map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(input), &want))
			require.NoError(t, json.Unmarshal(encoded, &got))
			require.JSONEq(t, string(want["model"]), string(got["model"]))
			require.JSONEq(t, string(want["messages"]), string(got["messages"]))
			var forwardedEffort string
			require.NoError(t, json.Unmarshal(got["reasoning_effort"], &forwardedEffort))
			require.Equal(t, effort, forwardedEffort)
			url, err := a.GetRequestURL(&meta.Meta{ActualModelName: req.Model, BaseURL: "https://api.z.ai"})
			require.NoError(t, err)
			require.Equal(t, "https://api.z.ai/api/paas/v4/chat/completions", url)
		})
	}
}
