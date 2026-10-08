package quota_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	modelcfg "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/quota"
)

// TestHaiku55QuotaIncludesCacheInTier settles real catalog prices through Compute.
// It takes t and returns nothing. Cache buckets select the tier but are charged
// separately, exactly once; a long output alone never selects the input tier.
func TestHaiku55QuotaIncludesCacheInTier(t *testing.T) {
	t.Parallel()
	provider := &anthropic.Adaptor{}
	const name = "claude-haiku-5-5"
	for _, tc := range []struct {
		name                    string
		input, read, five, hour int
		upper                   bool
	}{
		{"below", 99999, 0, 0, 0, false},
		{"boundary", 100000, 0, 0, 0, false},
		{"above", 100001, 0, 0, 0, true},
		{"cache_boundary", 1, 99999, 0, 0, false},
		{"cache_crossing", 1, 100000, 0, 0, true},
		{"five_minute_crossing", 1, 0, 100000, 0, true},
		{"one_hour_crossing", 1, 0, 0, 100000, true},
		{"mixed_boundary", 1, 49999, 25000, 25000, false},
		{"mixed_crossing", 2, 49999, 25000, 25000, true},
		{"all_cached", 0, 100001, 0, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, output := range []int{123, 128000} {
				usage := &model.Usage{
					PromptTokens: tc.input, CompletionTokens: output,
					PromptTokensDetails: &model.UsagePromptTokensDetails{CachedTokens: tc.read},
					CacheWrite5mTokens:  tc.five, CacheWrite1hTokens: tc.hour,
				}
				result := quota.Compute(quota.ComputeInput{
					Usage: usage, ModelName: name, GroupRatio: 1,
					ModelRatio: provider.GetModelRatio(name), PricingAdaptor: provider,
				})
				factor := 1.0
				if tc.upper {
					factor = 5
				}
				want := (float64(tc.input)*0.1 + float64(output)*0.5 + float64(tc.read)*0.01 +
					float64(tc.five)*0.125 + float64(tc.hour)*0.2) * factor * ratio.MilliTokensUsd
				require.Equal(t, int64(math.Ceil(want)), result.TotalQuota)
				require.InDelta(t, factor*0.1*ratio.MilliTokensUsd, result.UsedModelRatio, 1e-12)
				require.InDelta(t, 5, result.UsedCompletionRatio, 1e-12)
				require.Equal(t, tc.input, result.PromptTokens, "tier length must not replace raw charged input")
				require.Equal(t, tc.read, result.CachedPromptTokens)
				require.Equal(t, tc.input, usage.PromptTokens, "do not mutate the receipt")
			}
		})
	}
}

// TestHaiku55PricingOverridesRemainAuthoritative checks both local configuration
// and legacy scalar precedence. It takes t and returns nothing.
func TestHaiku55PricingOverridesRemainAuthoritative(t *testing.T) {
	t.Parallel()
	provider := &anthropic.Adaptor{}
	const name = "claude-haiku-5-5"
	input := quota.ComputeInput{
		Usage: &model.Usage{PromptTokens: 1, CompletionTokens: 123,
			PromptTokensDetails: &model.UsagePromptTokensDetails{CachedTokens: 100000}},
		ModelName: name, ModelRatio: 9 * ratio.MilliTokensUsd,
		GroupRatio: 1, PricingAdaptor: provider,
		ChannelModelConfigs: map[string]modelcfg.ModelConfigLocal{
			name: {Ratio: 9 * ratio.MilliTokensUsd, CompletionRatio: 2,
				CachedInputRatio: 3 * ratio.MilliTokensUsd},
		},
	}
	result := quota.Compute(input)
	require.InDelta(t, 9*ratio.MilliTokensUsd, result.UsedModelRatio, 1e-12)
	require.InDelta(t, 2, result.UsedCompletionRatio, 1e-12)
	require.Equal(t, int64(math.Ceil((9+123*18+100000*3)*ratio.MilliTokensUsd)), result.TotalQuota)
	input.ChannelModelConfigs = nil
	input.ModelRatio = 2 * ratio.MilliTokensUsd
	input.ChannelModelRatio = map[string]float64{name: input.ModelRatio}
	result = quota.Compute(input)
	require.InDelta(t, 2*ratio.MilliTokensUsd, result.UsedModelRatio, 1e-12)
	require.InDelta(t, 5, result.UsedCompletionRatio, 1e-12)
	require.Equal(t, int64(math.Ceil((2+123*10+100000*0.05)*ratio.MilliTokensUsd)), result.TotalQuota)
}
