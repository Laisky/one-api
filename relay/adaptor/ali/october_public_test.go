package ali_test

import (
	"github.com/Laisky/one-api/relay/adaptor/ali"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/stretchr/testify/require"
	"testing"
)

// TestOctoberAliPublishedRates pins native configuration prices and metadata
// through public methods using t. It returns no values and makes no API calls.
func TestOctoberAliPublishedRates(t *testing.T) {
	t.Parallel()
	a := &ali.Adaptor{}
	for _, tc := range []struct {
		id                 string
		input, output      float64
		context, maxOutput int32
	}{
		{"qwen-mt-plus", 1.8, 5.4, 16384, 8192},
		{"qwen-mt-flash", .7, 1.95, 16384, 8192},
		{"qwen-mt-lite", .6, 1.6, 16384, 8192},
		{"qwen-vl-ocr", .3, .5, 38192, 8192},
	} {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			cfg, exists := a.GetDefaultModelPricing()[tc.id]
			require.True(t, exists)
			require.Contains(t, a.GetModelList(), tc.id)
			require.InDelta(t, tc.input, a.GetModelRatio(tc.id)/ratio.MilliTokensRmb, 1e-12)
			require.InDelta(t, tc.output, a.GetModelRatio(tc.id)*a.GetCompletionRatio(tc.id)/ratio.MilliTokensRmb, 1e-12)
			require.Zero(t, cfg.CachedInputRatio)
			require.Equal(t, tc.context, cfg.ContextLength)
			require.Equal(t, tc.maxOutput, cfg.MaxOutputTokens)
			require.Empty(t, cfg.SupportedFeatures)
			require.Empty(t, cfg.Quantization)
			require.Zero(t, cfg.MaxTokens)
		})
	}
	require.Contains(t, ali.ModelRatios["qwen-vl-ocr"].Description, "not a qualified OCR image route")
	require.Contains(t, ali.ModelRatios["qwen-mt-plus"].Description, "translation_options")
	require.Contains(t, ali.ModelRatios["qwen-vl-ocr-latest"].Description, "historical")
	require.NotContains(t, a.GetModelList(), "qwen3.5-ocr")
	require.InDelta(t, 2.4, a.GetModelRatio("qwen-max")/ratio.MilliTokensRmb, 1e-12)
	require.InDelta(t, 9.6, a.GetModelRatio("qwen-max")*a.GetCompletionRatio("qwen-max")/ratio.MilliTokensRmb, 1e-12)
}
