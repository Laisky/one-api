package vertexai_test

import (
	"github.com/Laisky/one-api/relay/adaptor/vertexai"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/stretchr/testify/require"
	"testing"
)

// TestOctoberCloudHaikuDiscoveryAndPricing checks actual provider discovery,
// native routing and both sides of the long-prompt tier using t. It returns
// nothing and does not authenticate or invoke a live model.
func TestOctoberCloudHaikuDiscoveryAndPricing(t *testing.T) {
	t.Parallel()
	const id = "claude-haiku-5-5"
	a := &vertexai.Adaptor{}
	require.Contains(t, a.GetModelList(), id)
	cfg, ok := a.GetDefaultModelPricing()[id]
	require.True(t, ok)
	require.EqualValues(t, 1000000, cfg.ContextLength)
	require.EqualValues(t, 128000, cfg.MaxOutputTokens)
	require.Equal(t, "medium", cfg.DefaultReasoningEffort)
	require.Equal(t, []string{"low", "medium", "high", "xhigh", "max"}, cfg.SupportedReasoningEfforts)
	for _, tc := range []struct {
		tokens                            int
		input, cached, five, hour, output float64
	}{
		{99999, .1, .01, .125, .2, .5}, {100000, .1, .01, .125, .2, .5}, {100001, .5, .05, .625, 1, 2.5}, {1000000, .5, .05, .625, 1, 2.5},
	} {
		effective := pricing.ResolveEffectivePricingFromConfig(tc.tokens, cfg)
		require.InDelta(t, tc.input, effective.InputRatio/ratio.MilliTokensUsd, 1e-12)
		require.InDelta(t, tc.cached, effective.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
		require.InDelta(t, tc.five, effective.CacheWrite5mRatio/ratio.MilliTokensUsd, 1e-12)
		require.InDelta(t, tc.hour, effective.CacheWrite1hRatio/ratio.MilliTokensUsd, 1e-12)
		require.InDelta(t, tc.output, effective.OutputRatio/ratio.MilliTokensUsd, 1e-12)
	}
	clone := cfg.Clone()
	clone.Tiers[0].Ratio = 999
	require.NotEqual(t, clone.Tiers[0].Ratio, a.GetDefaultModelPricing()[id].Tiers[0].Ratio)

	m := &meta.Meta{OriginModelName: id, ActualModelName: id, BaseURL: "https://aiplatform.googleapis.com"}
	m.Config.VertexAIProjectID = "catalog-test-project"
	m.Config.Region = "global"
	want := "https://aiplatform.googleapis.com/v1/projects/catalog-test-project/locations/global/publishers/anthropic/models/claude-haiku-5-5:rawPredict"

	url, err := a.GetRequestURL(m)
	require.NoError(t, err)
	require.Equal(t, want, url)
	require.NotContains(t, a.GetModelList(), "claude-haiku-5-5-latest")
	require.NotContains(t, a.GetModelList(), "claude-haiku-5-5@20261007")

	sonnet := a.GetDefaultModelPricing()["claude-sonnet-5-5"]
	require.InDelta(t, .1, sonnet.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
	require.InDelta(t, 2, sonnet.Ratio/ratio.MilliTokensUsd, 1e-12)
	require.InDelta(t, 10, sonnet.Ratio*sonnet.CompletionRatio/ratio.MilliTokensUsd, 1e-12)

}
