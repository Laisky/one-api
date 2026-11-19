package quota_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	modelcfg "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/quota"
)

// TestGPT6CyberReceiptBilling checks real parsed usage, tier selection and
// unpriced receipts. Parameter t reports failures; the test returns nothing.
func TestGPT6CyberReceiptBilling(t *testing.T) {
	provider := &openai.Adaptor{ChannelType: channeltype.OpenAI}
	receipt := model.ParseResponseUsage([]byte(`{"type":"response.completed","response":{"usage":{"input_tokens":10000,"output_tokens":100,"input_tokens_details":{"cached_tokens":2000,"cache_write_tokens":3000,"cache_write_tokens_details":{"ttl_30m_tokens":3000,"ttl_12h_tokens":0}}}}}`), 0, nil)
	require.NotNil(t, receipt)
	require.Equal(t, 3000, receipt.CacheWrite5mTokens)
	input := quota.ComputeInput{Usage: receipt, ModelName: "gpt-6-cyber", ModelRatio: provider.GetModelRatio("gpt-6-cyber"), GroupRatio: 1, PricingAdaptor: provider}
	result := quota.Compute(input)
	require.False(t, result.UnpricedUsage)
	// USD 0.1 uncached + 0.004 cached + 0.075 writes + 0.01 output.
	require.EqualValues(t, 0.189*ratio.QuotaPerUsd, result.TotalQuota)

	receipt.PromptTokens = 272_000
	require.False(t, quota.Compute(input).UnpricedUsage)
	receipt.PromptTokens = 272_001
	require.True(t, quota.Compute(input).UnpricedUsage, "a missing long-context write tariff is not a short-context price")
	receipt.CacheWrite5mTokens = 0
	long := quota.Compute(input)
	require.False(t, long.UnpricedUsage)
	require.InDelta(t, 40, long.UsedModelRatio/ratio.MilliTokensUsd, 1e-9)
	require.InDelta(t, 150, long.UsedModelRatio*long.UsedCompletionRatio/ratio.MilliTokensUsd, 1e-9)
	for _, tier := range []string{"fast", "priority", "flex", "auto"} {
		receipt.ServiceTier = tier
		require.True(t, quota.Compute(input).UnpricedUsage, tier)
	}
	receipt.ServiceTier = "default"
	receipt.CacheWrite12hTokens = 1
	require.True(t, quota.Compute(input).UnpricedUsage)
}

// TestGPT6CyberExplicitWriteTariff preserves a qualified channel override for
// long writes. Parameter t reports failures; the test returns nothing.
func TestGPT6CyberExplicitWriteTariff(t *testing.T) {
	provider := &openai.Adaptor{ChannelType: channeltype.OpenAI}
	input := quota.ComputeInput{
		Usage:     &model.Usage{PromptTokens: 300_000, CacheWrite5mTokens: 1000},
		ModelName: "gpt-6-cyber", ModelRatio: provider.GetModelRatio("gpt-6-cyber"), GroupRatio: 1, PricingAdaptor: provider,
		ChannelModelConfigs: map[string]modelcfg.ModelConfigLocal{"gpt-6-cyber": {
			Ratio: 20 * ratio.MilliTokensUsd, CompletionRatio: 5,
			Tiers: []modelcfg.ModelRatioTierLocal{{InputTokenThreshold: 272_001,
				Ratio: 40 * ratio.MilliTokensUsd, CompletionRatio: 150.0 / 40,
				CacheWrite5mRatio: 60 * ratio.MilliTokensUsd}},
		}},
	}
	result := quota.Compute(input)
	require.False(t, result.UnpricedUsage)
	require.EqualValues(t, 12.02*ratio.QuotaPerUsd, result.TotalQuota)
}

// TestGPT6CyberUnpricedLongWriteNotBelowGenericRule pins the estimate for a
// long-context Cyber write that slipped past admission (the local estimate
// stayed below 272,001 while the provider counted more). OpenAI publishes no
// long-context Cyber write price, but its prompt-caching guide states a 30m
// write costs 1.25x the uncached input rate on GPT-5.6+, so the estimate must
// not price the write at the cheaper short-context rate the tier resolver
// inherits ($25/M, below even the $40/M long-context input). It stays an
// unpriced, flagged estimate.
func TestGPT6CyberUnpricedLongWriteNotBelowGenericRule(t *testing.T) {
	provider := &openai.Adaptor{ChannelType: channeltype.OpenAI}
	usage := &model.Usage{PromptTokens: 300_000, CompletionTokens: 1_000, CacheWrite5mTokens: 300_000}
	result := quota.Compute(quota.ComputeInput{Usage: usage, ModelName: "gpt-6-cyber",
		ModelRatio: provider.GetModelRatio("gpt-6-cyber"), GroupRatio: 1, PricingAdaptor: provider})
	require.True(t, result.UnpricedUsage, "the write still has no published tariff")
	// 300,000 writes at 1.25 x $40/M = $15.00 plus 1,000 output at $150/M = $0.15.
	require.EqualValues(t, 15.15*ratio.QuotaPerUsd, result.TotalQuota)
}
