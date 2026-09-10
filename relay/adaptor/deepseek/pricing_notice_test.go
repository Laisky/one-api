package deepseek

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	modelcfg "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/Laisky/one-api/relay/quota"
)

// TestDeepSeekReviewPricingNotice verifies both announced Beijing-noon changes
// through the production pricing resolvers and final quota calculation.
// Parameters: t is the test handle. Returns: nothing; incorrect rates fail tests.
func TestDeepSeekReviewPricingNotice(t *testing.T) {
	t.Parallel()
	// Each price triple is cache-miss input, cache-hit input, output in USD/1M.
	// Pre-notice Flash prices preserve the immediately preceding defaults from
	// e000b8eb; new prices and both dates follow the operator-provided notice.
	cases := []struct {
		at    string
		flash [3]float64
		pro   [3]float64
	}{
		{"2026-09-09T12:00:00Z", [3]float64{0.22, 0.007, 0.66}, [3]float64{0.66, 0.022, 1.98}},
		{"2026-09-10T00:00:00Z", [3]float64{0.22, 0.007, 0.66}, [3]float64{0.66, 0.022, 1.98}},
		{"2026-09-10T01:00:00Z", [3]float64{0.44, 0.014, 1.32}, [3]float64{1.32, 0.044, 3.96}},
		{"2026-09-10T03:59:59.999999999Z", [3]float64{0.44, 0.014, 1.32}, [3]float64{1.32, 0.044, 3.96}},
		{"2026-09-10T04:00:00Z", [3]float64{0.15, 0.003, 0.60}, [3]float64{0.66, 0.022, 1.98}},
		{"2026-09-10T04:00:00.000000001Z", [3]float64{0.15, 0.003, 0.60}, [3]float64{0.66, 0.022, 1.98}},
		{"2026-09-10T06:00:00Z", [3]float64{0.30, 0.006, 1.20}, [3]float64{1.32, 0.044, 3.96}},
		{"2026-09-12T06:00:00Z", [3]float64{0.15, 0.003, 0.60}, [3]float64{0.66, 0.022, 1.98}},
		{"2026-09-14T03:59:59.999999999Z", [3]float64{0.30, 0.006, 1.20}, [3]float64{1.32, 0.044, 3.96}},
		{"2026-09-14T04:00:00Z", [3]float64{0.15, 0.003, 0.60}, [3]float64{0.15, 0.003, 0.60}},
		{"2026-09-14T04:00:00.000000001Z", [3]float64{0.15, 0.003, 0.60}, [3]float64{0.15, 0.003, 0.60}},
		{"2026-09-14T06:00:00Z", [3]float64{0.30, 0.006, 1.20}, [3]float64{0.30, 0.006, 1.20}},
		{"2026-09-15T00:00:00Z", [3]float64{0.15, 0.003, 0.60}, [3]float64{0.15, 0.003, 0.60}},
	}
	provider := &Adaptor{}
	for _, name := range []string{"deepseek-flash", "deepseek-v4-flash", "deepseek-v4-flash-vision-exp", "deepseek-v4-pro"} {
		for _, tc := range cases {
			t.Run(name+"/"+tc.at, func(t *testing.T) {
				t.Parallel()
				at, err := time.Parse(time.RFC3339Nano, tc.at)
				require.NoError(t, err)
				want := tc.flash
				if name == "deepseek-v4-pro" {
					want = tc.pro
				}
				for label, resolve := range map[string]func(string, map[string]modelcfg.ModelConfigLocal, adaptor.Adaptor, time.Time) (adaptor.ModelConfig, bool){
					"full": pricing.ResolveModelConfig, "token_only": pricing.ResolveModelConfigRatioOnly,
				} {
					t.Run(label, func(t *testing.T) {
						cfg, ok := resolve(name, nil, provider, at)
						require.True(t, ok)
						require.InDelta(t, want[0], cfg.Ratio/ratio.MilliTokensUsd, 1e-12)
						require.InDelta(t, want[1], cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
						require.InDelta(t, want[2], cfg.Ratio*cfg.CompletionRatio/ratio.MilliTokensUsd, 1e-12)
						beijing := at.In(time.FixedZone("Beijing", 8*60*60))
						local, found := resolve(name, nil, provider, beijing)
						require.True(t, found)
						require.Equal(t, cfg, local, "caller timezone must not change billing")
					})
				}
				for _, usage := range []struct {
					name                    string
					prompt, cached, output  int
				}{
					{"cache_miss", 100000, 0, 0},
					{"cache_hit", 100000, 100000, 0},
					{"output", 0, 0, 100000},
					{"mixed", 300000, 100000, 50000},
				} {
					t.Run("settlement/"+usage.name, func(t *testing.T) {
						result := quota.Compute(quota.ComputeInput{
							ModelName: name, ModelRatio: provider.GetModelRatio(name),
							PricingAdaptor: provider, GroupRatio: 1, RequestTime: at,
							Usage: &relaymodel.Usage{
								PromptTokens: usage.prompt, CompletionTokens: usage.output,
								PromptTokensDetails: &relaymodel.UsagePromptTokensDetails{CachedTokens: usage.cached},
							},
						})
						usd := (float64(usage.prompt-usage.cached)*want[0] + float64(usage.cached)*want[1] + float64(usage.output)*want[2]) / 1000000
						require.Equal(t, int64(math.Round(usd*ratio.QuotaPerUsd)), result.TotalQuota)
						require.Equal(t, usage.cached, result.CachedPromptTokens)
					})
				}
			})
		}
	}
}
