package deepseek

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
)

// TestModelRatiosMatchOfficialCatalog checks current and compatible API names,
// without reintroducing retired V3 names or inventing version-pinned IDs.
func TestModelRatiosMatchOfficialCatalog(t *testing.T) {
	t.Parallel()
	require.ElementsMatch(t, []string{"deepseek-flash", "deepseek-v4-flash", "deepseek-v4-flash-vision-exp", "deepseek-v4-pro"}, (&Adaptor{}).GetModelList())
	for _, name := range []string{"deepseek-chat", "deepseek-reasoner", "deepseek-v4-flash-vision", "deepseek-v4.1-pro"} {
		require.NotContains(t, ModelRatios, name)
	}
}

// TestModelRatiosMatchOfficialPricing verifies current off-peak defaults. The
// separate schedule tests exercise the authoritative time-dependent prices.
func TestModelRatiosMatchOfficialPricing(t *testing.T) {
	t.Parallel()
	for name, cfg := range ModelRatios {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			input, cached, output := 0.15, 0.003, 0.60
			if name == "deepseek-v4-pro" {
				input, cached, output = 0.66, 0.022, 1.98
			}
			require.InDelta(t, input, cfg.Ratio/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, cached, cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, output, cfg.Ratio*cfg.CompletionRatio/ratio.MilliTokensUsd, 1e-12)
			require.Equal(t, int32(1048576), cfg.ContextLength)
			require.Equal(t, int32(393216), cfg.MaxOutputTokens)
		})
	}
}

// TestModelRatiosMatchOfficialCapabilities prevents stale vision, search, and
// open-weight metadata from being advertised for the replacement Flash model.
func TestModelRatiosMatchOfficialCapabilities(t *testing.T) {
	t.Parallel()
	for name, cfg := range ModelRatios {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.ElementsMatch(t, []string{"low", "high", "max"}, cfg.SupportedReasoningEfforts)
			require.Equal(t, "high", cfg.DefaultReasoningEffort)
			require.ElementsMatch(t, []string{"text"}, cfg.OutputModalities)
			require.ElementsMatch(t, []string{"tools", "json_mode", "logprobs", "reasoning"}, cfg.SupportedFeatures)
			require.Contains(t, cfg.SupportedSamplingParameters, "temperature")
			require.Contains(t, cfg.SupportedSamplingParameters, "top_p")
			require.Nil(t, cfg.Image, "image inputs are prompt tokens, not generated images")
			if name == "deepseek-v4-pro" {
				require.ElementsMatch(t, []string{"text"}, cfg.InputModalities)
				require.Contains(t, cfg.Description, "DeepSeek-V4-Pro-0813")
			} else {
				require.ElementsMatch(t, []string{"text", "image", "file"}, cfg.InputModalities)
				require.Contains(t, cfg.Description, "DeepSeek-V4.1-Flash")
				require.Empty(t, cfg.Quantization)
				require.Empty(t, cfg.HuggingFaceID)
			}
		})
	}
	require.Empty(t, (&Adaptor{}).DefaultToolingConfig().Pricing, "ignored built-in tools must not be advertised as free native tools")
}

// TestDeepSeekFlashAliasesShareCurrentConfiguration compares all alias metadata
// except their deliberately distinct descriptions.
func TestDeepSeekFlashAliasesShareCurrentConfiguration(t *testing.T) {
	t.Parallel()
	canonical := ModelRatios["deepseek-flash"]
	canonical.Description = ""
	for _, name := range []string{"deepseek-v4-flash", "deepseek-v4-flash-vision-exp"} {
		alias := ModelRatios[name]
		alias.Description = ""
		require.Equal(t, canonical, alias, name)
	}
}
