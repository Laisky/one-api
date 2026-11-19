package quota_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/aiproxy"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/quota"
)

// TestModelBillingPolicyReceiptScope pins which settling providers apply each
// catalog billing policy to provider receipts. Native-tariff-only policies
// yield to a known hosted provider; other policies follow the model on every
// provider, and an unknown provider stays conservative. Parameter t reports
// failures; nothing returns.
func TestModelBillingPolicyReceiptScope(t *testing.T) {
	t.Parallel()
	providers := []struct {
		name     string
		provider func() adaptor.Adaptor
		native   bool
	}{
		{"missing", func() adaptor.Adaptor { return nil }, true},
		{"native_openai", func() adaptor.Adaptor { return &openai.Adaptor{ChannelType: channeltype.OpenAI} }, true},
		{"unset_openai", func() adaptor.Adaptor { return &openai.Adaptor{} }, true},
		{"azure", func() adaptor.Adaptor { return &openai.Adaptor{ChannelType: channeltype.Azure} }, false},
		{"openai_compatible", func() adaptor.Adaptor { return &openai.Adaptor{ChannelType: channeltype.OpenAICompatible} }, false},
		{"aiproxy", func() adaptor.Adaptor { return &aiproxy.Adaptor{} }, false},
	}
	models := []struct {
		name                  string
		declared, nativeOnly  bool
		longContextWriteGuard bool
	}{
		{"gpt-6.1-terra", true, false, false},
		{" GPT-6.1-Terra ", true, false, false},
		{"gpt-6-cyber", true, true, true},
		{"\tGPT-6-CYBER ", true, true, true},
		{"gpt-6-sol", false, false, false},
		{"gpt-5.6-cyber", false, false, false},
	}
	receipts := []struct {
		name     string
		usage    model.Usage
		tierOnly bool
	}{
		{"fast_service_tier", model.Usage{PromptTokens: 1000, CompletionTokens: 10, ServiceTier: "fast"}, true},
		{"padded_priority_tier", model.Usage{PromptTokens: 1000, CompletionTokens: 10, ServiceTier: " Priority "}, true},
		{"standard_service_tier", model.Usage{PromptTokens: 1000, CompletionTokens: 10, ServiceTier: " DEFAULT "}, false},
		{"long_context_cache_write", model.Usage{PromptTokens: 300_000, CompletionTokens: 10, CacheWrite5mTokens: 1000}, false},
		{"short_context_cache_write", model.Usage{PromptTokens: 272_000, CompletionTokens: 10, CacheWrite5mTokens: 1000}, false},
	}
	for _, providerCase := range providers {
		for _, modelCase := range models {
			for _, receiptCase := range receipts {
				name := providerCase.name + "/" + modelCase.name + "/" + receiptCase.name
				usage := receiptCase.usage
				result := quota.Compute(quota.ComputeInput{
					Usage: &usage, ModelName: modelCase.name, ModelRatio: 2 * ratio.MilliTokensUsd,
					GroupRatio: 1, PricingAdaptor: providerCase.provider(),
				})
				applies := modelCase.declared && (!modelCase.nativeOnly || providerCase.native)
				want := applies && receiptCase.tierOnly
				if receiptCase.name == "long_context_cache_write" {
					want = applies && modelCase.longContextWriteGuard
				}
				require.Equal(t, want, result.UnpricedUsage, name)
				require.Positive(t, result.TotalQuota, name)
			}
		}
	}
}
