package deepseek

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
)

// TestModelRatiosMatchOfficialCatalog verifies that the adapter exposes only
// the model IDs currently returned by DeepSeek's official model catalog.
func TestModelRatiosMatchOfficialCatalog(t *testing.T) {
	t.Parallel()

	require.Len(t, ModelRatios, 2)
	require.Contains(t, ModelRatios, "deepseek-v4-flash")
	require.Contains(t, ModelRatios, "deepseek-v4-pro")
	require.NotContains(t, ModelRatios, "deepseek-chat")
	require.NotContains(t, ModelRatios, "deepseek-reasoner")
}

// TestModelRatiosMatchOfficialPricing verifies current regular prices, context
// limits, and the absence of an unannounced peak-pricing overlay.
func TestModelRatiosMatchOfficialPricing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       float64
		cachedInput float64
		output      float64
	}{
		{name: "deepseek-v4-flash", input: 0.14, cachedInput: 0.0028, output: 0.28},
		{name: "deepseek-v4-pro", input: 0.435, cachedInput: 0.003625, output: 0.87},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := ModelRatios[tt.name]
			require.InDelta(t, tt.input*ratio.MilliTokensUsd, cfg.Ratio, 1e-15)
			require.InDelta(t, tt.cachedInput*ratio.MilliTokensUsd, cfg.CachedInputRatio, 1e-15)
			require.InDelta(t, tt.output*ratio.MilliTokensUsd, cfg.Ratio*cfg.CompletionRatio, 1e-15)
			require.Equal(t, int32(1048576), cfg.ContextLength)
			require.Equal(t, int32(393216), cfg.MaxOutputTokens)
			require.Empty(t, cfg.TimeWindows, "peak pricing has no announced effective date")
			require.NotContains(t, cfg.SupportedFeatures, "structured_outputs")
			require.Equal(t, "fp4", cfg.Quantization)
		})
	}
}

// TestModelRatiosMatchOfficialCapabilities verifies model-specific version,
// reasoning-effort, and native Responses API capability metadata.
func TestModelRatiosMatchOfficialCapabilities(t *testing.T) {
	t.Parallel()

	flash := ModelRatios["deepseek-v4-flash"]
	require.ElementsMatch(t, []string{"low", "high", "max"}, flash.SupportedReasoningEfforts)
	require.Contains(t, flash.SupportedFeatures, "web_search")
	require.Contains(t, flash.Description, "DeepSeek-V4-Flash-0731")

	pro := ModelRatios["deepseek-v4-pro"]
	require.ElementsMatch(t, []string{"high", "max"}, pro.SupportedReasoningEfforts)
	require.NotContains(t, pro.SupportedFeatures, "web_search")
	require.Contains(t, pro.Description, "Responses API support is pending")
}
