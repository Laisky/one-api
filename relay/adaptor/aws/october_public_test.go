package aws_test

import (
	"github.com/Laisky/one-api/relay/adaptor/aws"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/stretchr/testify/require"
	"testing"
)

// TestOctoberBedrockPricesAndEffort checks the AWS cache reduction and exact
// host-specific effort controls through public metadata using t; it returns
// nothing and does not invoke or authenticate to a live AWS endpoint.
func TestOctoberBedrockPricesAndEffort(t *testing.T) {
	t.Parallel()
	a := &aws.Adaptor{}
	for id, prices := range map[string][5]float64{
		"claude-sonnet-5-5": {2, .1, 2.5, 4, 10},
		"claude-opus-5-5":   {4, .2, 5, 8, 20},
	} {
		cfg, ok := a.GetDefaultModelPricing()[id]
		require.True(t, ok)
		require.Contains(t, a.GetModelList(), id)
		require.InDelta(t, prices[0], cfg.Ratio/ratio.MilliTokensUsd, 1e-12)
		require.InDelta(t, prices[1], cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
		require.InDelta(t, prices[2], cfg.CacheWrite5mRatio/ratio.MilliTokensUsd, 1e-12)
		require.InDelta(t, prices[3], cfg.CacheWrite1hRatio/ratio.MilliTokensUsd, 1e-12)
		require.InDelta(t, prices[4], cfg.Ratio*cfg.CompletionRatio/ratio.MilliTokensUsd, 1e-12)
		require.NotContains(t, cfg.SupportedFeatures, "structured_outputs", "AWS model card does not support native structured output")
		require.EqualValues(t, 1000000, cfg.ContextLength)
		require.EqualValues(t, 128000, cfg.MaxOutputTokens)
		require.Equal(t, []string{"low", "medium", "high", "xhigh", "max"}, cfg.SupportedReasoningEfforts)
	}
	opus := a.GetDefaultModelPricing()["claude-opus-5-5"]
	require.Equal(t, "medium", opus.DefaultReasoningEffort)
	require.Zero(t, opus.MaxReasoningTokens)
	clone := opus.Clone()
	clone.SupportedReasoningEfforts[0] = "changed"
	require.Equal(t, "low", a.GetDefaultModelPricing()["claude-opus-5-5"].SupportedReasoningEfforts[0])
	require.Equal(t, "high", a.GetDefaultModelPricing()["claude-sonnet-5-5"].DefaultReasoningEffort)
}
