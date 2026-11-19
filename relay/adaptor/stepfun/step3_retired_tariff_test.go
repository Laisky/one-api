package stepfun_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/stepfun"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/pricing"
)

// TestStep3RetiredModelKeepsLastPublishedTieredTariff pins step-3 to the last
// tariff StepFun published for it before retirement (pricing page
// https://platform.stepfun.com/docs/zh/guides/pricing/details, Wayback snapshot
// 2026-03-16): input <=4k and output <=4k ¥1.5 in / ¥0.3 cache hit / ¥4 out;
// input <=4k and output >4k ¥1.5 / ¥0.3 / ¥8; 4k < input <=64k ¥4 / ¥0.8 / ¥10
// per 1M tokens. "4k" is read as 4,000 so the 4,001-4,096 band never bills
// below the higher tier. The model was retired on 2026-07-08
// (https://platform.stepfun.com/docs/zh/guides/models/deprecations, fetched
// 2026-11-18), so the ID stays for configured channels but must say so. It
// accepts t and returns no values.
func TestStep3RetiredModelKeepsLastPublishedTieredTariff(t *testing.T) {
	t.Parallel()
	cfg, exists := (&stepfun.Adaptor{}).GetDefaultModelPricing()["step-3"]
	require.True(t, exists, "configured channels may still name step-3")
	require.Contains(t, cfg.Description, "retired 2026-07-08")
	require.Contains(t, cfg.Description, "step-3.7-flash")
	for _, tc := range []struct {
		name                   string
		input, output          int
		in, cached, outputYuan float64
	}{
		{"short prompt short answer", 3000, 3000, 1.5, 0.3, 4},
		{"4k prompt 4k answer", 4000, 4000, 1.5, 0.3, 4},
		{"short prompt long answer", 4000, 4001, 1.5, 0.3, 8},
		{"first long-prompt token", 4001, 10, 4, 0.8, 10},
		{"long prompt long answer", 5000, 5000, 4, 0.8, 10},
		{"below the old 32k threshold", 31999, 100, 4, 0.8, 10},
		{"64k prompt", 64000, 8192, 4, 0.8, 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			eff := pricing.ResolveEffectivePricingForUsageFromConfig(tc.input, tc.output, cfg)
			require.InDelta(t, tc.in, eff.InputRatio/ratio.MilliTokensRmb, 1e-9)
			require.InDelta(t, tc.cached, eff.CachedInputRatio/ratio.MilliTokensRmb, 1e-9)
			require.InDelta(t, tc.outputYuan, eff.OutputRatio/ratio.MilliTokensRmb, 1e-9)
		})
	}
}
