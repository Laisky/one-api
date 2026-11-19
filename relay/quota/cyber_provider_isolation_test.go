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

// TestGPT6CyberProviderTariffIsolation preserves explicit hosted tariffs while
// retaining native OpenAI receipt guards. Parameter t reports failures; nothing returns.
func TestGPT6CyberProviderTariffIsolation(t *testing.T) {
	t.Parallel()
	for _, providerCase := range []struct {
		name        string
		channelType int
		guarded     bool
	}{
		{"native_openai", channeltype.OpenAI, true},
		{"default_openai", 0, true},
		{"openai_compatible", channeltype.OpenAICompatible, false},
		{"azure", channeltype.Azure, false},
		{"groq", channeltype.Groq, false},
	} {
		t.Run(providerCase.name, func(t *testing.T) {
			provider := &openai.Adaptor{ChannelType: providerCase.channelType}
			for _, receiptCase := range []struct {
				name  string
				usage model.Usage
				quota int64
			}{
				{"nonstandard_service_tier", model.Usage{
					PromptTokens: 1000, CompletionTokens: 10, ServiceTier: "fast",
				}, 1040},
				{"long_context_cache_write", model.Usage{
					PromptTokens: 300_000, CompletionTokens: 10, CacheWrite5mTokens: 1000,
				}, 300_540},
			} {
				t.Run(receiptCase.name, func(t *testing.T) {
					input := quota.ComputeInput{
						Usage: &receiptCase.usage, ModelName: "gpt-6-cyber",
						ModelRatio: 2 * ratio.MilliTokensUsd, GroupRatio: 1, PricingAdaptor: provider,
						ChannelModelConfigs: map[string]modelcfg.ModelConfigLocal{"gpt-6-cyber": {
							Ratio: 2 * ratio.MilliTokensUsd, CompletionRatio: 4,
							CacheWrite5mRatio: 3 * ratio.MilliTokensUsd,
						}},
					}
					result := quota.Compute(input)
					require.Equal(t, providerCase.guarded, result.UnpricedUsage,
						"native tariff restrictions must not replace a hosted operator's known price")
					require.Equal(t, receiptCase.quota, result.TotalQuota)
				})
			}
		})
	}
}

// TestGPT6CyberMissingProviderRemainsConservative retains receipt guards when
// no provider identity is available. Parameter t reports failures; nothing returns.
func TestGPT6CyberMissingProviderRemainsConservative(t *testing.T) {
	t.Parallel()
	result := quota.Compute(quota.ComputeInput{
		Usage:     &model.Usage{PromptTokens: 1000, CompletionTokens: 10, ServiceTier: "fast"},
		ModelName: "gpt-6-cyber", ModelRatio: 1, GroupRatio: 1,
	})
	require.True(t, result.UnpricedUsage)
}
