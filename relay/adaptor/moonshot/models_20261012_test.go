package moonshot

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/model"
)

// TestMoonshotOctoberLifecycle checks all retained retired IDs and historical
// tariffs using t, without deleting configurations or inventing new aliases.
func TestMoonshotOctoberLifecycle(t *testing.T) {
	t.Parallel()
	a := &Adaptor{}
	for id, prices := range map[string][2]float64{
		"kimi-k2.5":                      {4, 21},
		"moonshot-v1-8k":                 {2, 10},
		"moonshot-v1-32k":                {5, 20},
		"moonshot-v1-128k":               {10, 30},
		"moonshot-v1-8k-vision-preview":   {2, 10},
		"moonshot-v1-32k-vision-preview":  {5, 20},
		"moonshot-v1-128k-vision-preview": {10, 30},
	} {
		cfg, ok := a.GetDefaultModelPricing()[id]
		require.True(t, ok)
		require.Contains(t, a.GetModelList(), id)
		require.Contains(t, cfg.Description, "Retired on 2026-08-31")
		require.Contains(t, cfg.Description, "historical pricing")
		require.InDelta(t, prices[0], cfg.Ratio/ratio.MilliTokensRmb, 1e-12)
		require.InDelta(t, prices[1], cfg.Ratio*cfg.CompletionRatio/ratio.MilliTokensRmb, 1e-12)
	}
	for _, id := range []string{"kimi-k3", "kimi-k2.7-code", "kimi-k2.7-code-highspeed", "kimi-k2.6"} {
		require.NotContains(t, a.GetDefaultModelPricing()[id].Description, "Retired on")
	}
}

// TestKimiK3PublishedControlsReachRequest verifies the newly advertised output
// control and existing effort survive conversion using t. It returns nothing.
func TestKimiK3PublishedControlsReachRequest(t *testing.T) {
	t.Parallel()
	a := &Adaptor{}
	cfg := a.GetDefaultModelPricing()["kimi-k3"]
	require.Contains(t, cfg.SupportedSamplingParameters, "max_completion_tokens")
	require.Contains(t, cfg.SupportedSamplingParameters, "reasoning_effort")
	require.Contains(t, cfg.Description, "5m/1h")
	require.Contains(t, cfg.Description, "not yet represented")
	effort := "max"
	req := &model.GeneralOpenAIRequest{Model: "kimi-k3", MaxCompletionTokens: 777, ReasoningEffort: &effort}
	converted, err := a.ConvertRequest(nil, 0, req)
	require.NoError(t, err)
	wire, ok := converted.(*model.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Equal(t, "kimi-k3", wire.Model)
	require.EqualValues(t, 777, wire.MaxCompletionTokens)
	require.Equal(t, "max", *wire.ReasoningEffort)
	require.InDelta(t, 20.0, cfg.Ratio/ratio.MilliTokensRmb, 1e-12)
	require.InDelta(t, 2.0, cfg.CachedInputRatio/ratio.MilliTokensRmb, 1e-12)
}
