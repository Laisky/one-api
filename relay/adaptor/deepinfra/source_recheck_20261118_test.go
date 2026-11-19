package deepinfra_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/deepinfra"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// TestDeepInfraCachedPricesUsePublishedDecimals pins the cached-input prices
// DeepInfra displays on its own model pages (https://deepinfra.com/<model>,
// checked 2026-11-18) instead of the float products of the API's 8-decimal
// cache multiplier (https://api.deepinfra.com/models/<model>). The API quotes
// cents per token, a discount applied once, and `rate_per_input_token_cached`
// as a truncated multiplier of the discounted input price, so its product is
// within 4e-7 (relative) of the clean decimal. MiMo-V2.6-Pro's page rounds to $0.003, so
// its clean value is the API product rounded to the list-price precision
// ($0.0036 list, 10% off), which is not below the API product. It accepts t and
// returns no values.
func TestDeepInfraCachedPricesUsePublishedDecimals(t *testing.T) {
	t.Parallel()
	a := &deepinfra.Adaptor{}
	for _, tc := range []struct {
		id                          string
		input, cached, output       float64
		centsIn, centsOut, discount float64
		cacheMultiplier             float64
	}{
		{"thinkingmachines/Inkling-Small", 0.45, 0.1, 1.2, 4.5e-05, 0.00012, 0, 0.22222222},
		{"Qwen/Qwen3-Coder-480B-A35B-Instruct-Turbo", 0.3, 0.1, 1.0, 3e-05, 0.0001, 0, 0.33333333},
		{"zai-org/GLM-5.2", 0.5625, 0.105, 1.8, 7.5e-05, 0.00024, 0.25, 0.18666667},
		{"XiaomiMiMo/MiMo-V2.6-Pro", 0.387, 0.00324, 0.783, 4.3e-05, 8.7e-05, 0.1, 0.00837209},
		// Second sweep (API https://api.deepinfra.com/models/<model> fetched
		// 2026-11-18T14:25Z, pages https://deepinfra.com/<model> fetched
		// 2026-11-18T14:27Z). Pages round to three decimals: Qwen3.8-Flash shows
		// $0.014 and V4-Flash-Vision-Exp $0.007 (discounted) / $0.014 (list); the
		// API products pin 0.0141 and 0.014 * (1 - 0.51) = 0.00686. MiniMax-M2.7-Turbo,
		// Hy3 and GLM-5.1 pages are retired (404), so only the API quote applies.
		{"deepseek-ai/DeepSeek-V4-Flash-Vision-Exp", 0.2156, 0.00686, 0.6468, 4.4e-05, 0.000132, 0.51, 0.03181818},
		{"deepseek-ai/DeepSeek-V4-Pro", 1.3, 0.1, 2.6, 0.00013, 0.00026, 0, 0.07692308},
		{"deepseek-ai/DeepSeek-V4-Pro-0813", 1.3, 0.1, 2.6, 0.00013, 0.00026, 0, 0.07692308},
		{"nvidia/NVIDIA-Nemotron-3.5-Super-VL", 0.15, 0.02, 0.5, 1.5e-05, 5e-05, 0, 0.13333333},
		{"google/gemma-4-31B-it-turbo", 0.09, 0.05, 0.34, 9e-06, 3.4e-05, 0, 0.55555556},
		{"MiniMaxAI/MiniMax-M2.7-Turbo", 0.38, 0.07, 1.7, 3.8e-05, 0.00017, 0, 0.18421053},
		{"meta-models/Muse-Glimmer-30B", 0.3, 0.04, 1.2, 3e-05, 0.00012, 0, 0.13333333},
		// tencent/Hy3 ($0.13 / $0.033 / $0.53 own quote) is deprecated and forwarded
		// to tencent/Hy4-preview, so it bills the dearer forwarded tariff pinned by
		// TestDeepInfraDeprecatedModelsNeverBillBelowForwardedTariff.
		{"tencent/Hy4-preview", 0.834, 0.042, 2.501, 8.34e-05, 0.0002501, 0, 0.05035971},
		{"zai-org/GLM-5.1", 1.05, 0.205, 3.5, 0.000105, 0.00035, 0, 0.1952381},
		{"zai-org/GLM-5.3", 0.5625, 0.125, 2.5, 9e-05, 0.0004, 0.375, 0.22222222},
		{"Qwen/Qwen3.5-35B-A3B", 0.14, 0.05, 1.0, 1.4e-05, 0.0001, 0, 0.35714286},
		{"Qwen/Qwen3.5-397B-A17B", 0.45, 0.22, 3.0, 4.5e-05, 0.0003, 0, 0.48888889},
		{"Qwen/Qwen3.8-Flash", 0.113, 0.0141, 0.382, 1.13e-05, 3.82e-05, 0, 0.12477876},
		{"Qwen/Qwen3.8-Max", 1.65, 0.206, 4.951, 0.000165, 0.0004951, 0, 0.12484848},
		{"Qwen/Qwen4-Max", 1.413, 0.071, 4.24, 0.0001413, 0.000424, 0, 0.0502477},
		{"thinkingmachines/Inkling-2-Small", 0.285, 0.0285, 0.95, 3e-05, 0.0001, 0.05, 0.1},
	} {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			cfg, exists := a.GetDefaultModelPricing()[tc.id]
			require.True(t, exists)
			require.InDelta(t, tc.input, cfg.Ratio/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, tc.cached, cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-15)
			require.InDelta(t, tc.output, cfg.Ratio*cfg.CompletionRatio/ratio.MilliTokensUsd, 1e-12)

			// The clean decimals must agree with the host API's own quote.
			apiInput := tc.centsIn * 10000 * (1 - tc.discount)
			apiOutput := tc.centsOut * 10000 * (1 - tc.discount)
			apiCached := apiInput * tc.cacheMultiplier
			require.InDelta(t, apiInput, tc.input, 1e-12)
			require.InDelta(t, apiOutput, tc.output, 1e-12)
			require.LessOrEqual(t, math.Abs(tc.cached-apiCached)/apiCached, 4e-7)
		})
	}
}

// TestDeepInfraCompletionRatiosAreExactPublishedQuotients pins completion ratios
// that were stored as 15-digit truncations (3.33333333333333) to the exact
// quotient of the published per-1M decimals, so the billed output price equals
// the page price instead of sitting one rounding step below it. Sources:
// https://api.deepinfra.com/models/<model> (2026-11-18T14:25Z) and
// https://deepinfra.com/<model> (2026-11-18T14:27Z): Nemotron-3.5-Super-VL
// $0.15 in / $0.50 out; Inkling-2-Small $0.285 in / $0.95 out after the
// published 5% discount ($0.30 / $1.00 list). It accepts t and returns no values.
func TestDeepInfraCompletionRatiosAreExactPublishedQuotients(t *testing.T) {
	t.Parallel()
	catalog := (&deepinfra.Adaptor{}).GetDefaultModelPricing()
	for _, tc := range []struct {
		id            string
		input, output float64
	}{
		{"nvidia/NVIDIA-Nemotron-3.5-Super-VL", 0.15, 0.5},
		{"thinkingmachines/Inkling-2-Small", 0.285, 0.95},
	} {
		cfg, exists := catalog[tc.id]
		require.True(t, exists, tc.id)
		require.Equal(t, tc.output/tc.input, cfg.CompletionRatio, tc.id)
		require.InDelta(t, tc.input, cfg.Ratio/ratio.MilliTokensUsd, 1e-12, tc.id)
	}
}
