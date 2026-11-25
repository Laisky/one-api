package ali_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	relay "github.com/Laisky/one-api/relay"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/ali"
	"github.com/Laisky/one-api/relay/adaptor/alibailian"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/model"
	quotautil "github.com/Laisky/one-api/relay/quota"
)

// qwenTieredCacheModels lists, per DashScope host catalog, the tiered Qwen
// models whose base entry publishes an implicit cache-hit price.
var qwenTieredCacheModels = map[string][]string{
	"ali": {"qwen-flash", "qwen3-max", "qwen3-max-preview", "qwen3.5-flash", "qwen3.5-plus", "qwen3.6-flash",
		"qwen3.6-max-preview", "qwen3.6-plus", "qwen3.7-plus", "qwen3-coder-flash", "qwen3-coder-plus", "qwen3-vl-flash", "qwen3-vl-plus"},
	"alibailian": {"qwen-flash", "qwen3-max", "qwen3.5-flash", "qwen3.5-plus", "qwen3.6-flash", "qwen3.6-max-preview",
		"qwen3.6-plus", "qwen3.7-plus", "qwen3-coder-flash", "qwen3-coder-next", "qwen3-coder-plus", "qwen3-vl-flash", "qwen3-vl-plus"},
}

// qwenCacheFraction returns the documented cache-hit share of the stated input
// price: 20% (https://help.aliyun.com/zh/model-studio/context-cache), except
// qwen3.7-plus, whose stated input is a 20%-off promotion of list CNY 2 / 6 and
// whose cache price stays 20% of the list price (25% of the promotion).
func qwenCacheFraction(name string) float64 {
	if name == "qwen3.7-plus" {
		return 0.25
	}
	return 0.2
}

// qwenHostCatalogs returns the ali and alibailian default pricing catalogs.
func qwenHostCatalogs() map[string]map[string]adaptor.ModelConfig {
	return map[string]map[string]adaptor.ModelConfig{
		"ali":        (&ali.Adaptor{}).GetDefaultModelPricing(),
		"alibailian": alibailian.ModelRatios,
	}
}

// TestQwenTiersStateDashScopeCachePrice verifies every tier of the tiered Qwen
// models states the DashScope cache-hit price of its own input band, so a
// long-context prompt never bills cached tokens at the cheaper base-band price,
// and that no tiered model with a base cache price leaves a tier without one.
func TestQwenTiersStateDashScopeCachePrice(t *testing.T) {
	t.Parallel()
	catalogs := qwenHostCatalogs()
	for host, names := range qwenTieredCacheModels {
		for _, name := range names {
			cfg, ok := catalogs[host][name]
			require.True(t, ok, "%s/%s must be in the catalog", host, name)
			require.NotEmpty(t, cfg.Tiers, "%s/%s must be tiered", host, name)
			fraction := qwenCacheFraction(name)
			require.InDelta(t, fraction*cfg.Ratio, cfg.CachedInputRatio, cfg.Ratio*1e-9, "%s/%s base cache price", host, name)
			for _, tier := range cfg.Tiers {
				require.InDelta(t, fraction*tier.Ratio, tier.CachedInputRatio, tier.Ratio*1e-9,
					"%s/%s tier at %d input tokens must state its band's cache price", host, name, tier.InputTokenThreshold)
			}
		}
	}
	for host, catalog := range catalogs {
		for name, cfg := range catalog {
			if cfg.CachedInputRatio <= 0 {
				continue
			}
			for _, tier := range cfg.Tiers {
				require.NotZero(t, tier.CachedInputRatio, "%s/%s tier at %d input tokens omits its cache price", host, name, tier.InputTokenThreshold)
			}
		}
	}
}

// TestQwenTieredPromptBillsTierCachePrice verifies quota.Compute on the native
// Ali adaptor settles the cached half of a top-band prompt at that band's cache
// price for every listed model.
func TestQwenTieredPromptBillsTierCachePrice(t *testing.T) {
	t.Parallel()
	provider := relay.GetAdaptor(apitype.Ali)
	require.NotNil(t, provider)
	for _, name := range qwenTieredCacheModels["ali"] {
		cfg := provider.GetDefaultModelPricing()[name]
		top := cfg.Tiers[len(cfg.Tiers)-1]
		prompt, cached, completion := top.InputTokenThreshold, top.InputTokenThreshold/2, 1000
		result := quotautil.Compute(quotautil.ComputeInput{
			Usage: &model.Usage{PromptTokens: prompt, CompletionTokens: completion,
				PromptTokensDetails: &model.UsagePromptTokensDetails{CachedTokens: cached}},
			ModelName: name, ModelRatio: provider.GetModelRatio(name), GroupRatio: 1, PricingAdaptor: provider,
		})
		want := int64(math.Ceil(float64(prompt-cached)*top.Ratio + float64(cached)*top.CachedInputRatio +
			float64(completion)*top.Ratio*top.CompletionRatio))
		require.Equal(t, want, result.TotalQuota, "%s top band must bill cached tokens at its own cache price", name)
	}
}
