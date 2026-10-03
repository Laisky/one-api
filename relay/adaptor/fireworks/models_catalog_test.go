package fireworks

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
)

// TestCurrentServerlessCatalogPricing verifies every model currently tagged
// Serverless by Fireworks has the published standard-tier price and context.
func TestCurrentServerlessCatalogPricing(t *testing.T) {
	t.Parallel()

	// This table mirrors the 20 Serverless-tagged entries in the Fireworks model
	// library on 2026-09-04. Historical and dedicated-only models may remain in
	// ModelRatios for backward compatibility, but they do not belong here.
	currentServerlessModels := map[string]struct {
		inputUSD     float64
		cachedUSD    float64
		outputUSD    float64
		contextLimit int32
	}{
		"accounts/fireworks/models/glm-5p3-flash":                  {0.15, 0.03, 0.50, 1048576},
		"accounts/fireworks/models/glm-5p3":                        {1.40, 0.26, 4.40, 1048576},
		"accounts/fireworks/models/kimi-k3":                        {3.00, 0.30, 15.00, 1048576},
		"accounts/fireworks/models/deepseek-v4-pro-0813":           {1.32, 0.044, 3.96, 1048576},
		"accounts/fireworks/models/qwen3p8-2p4t-a95b":              {2.00, 0.25, 6.00, 262144},
		"accounts/fireworks/models/muse-glimmer-30b":               {0.35, 0.04, 1.50, 131072},
		"accounts/fireworks/models/deepseek-v4-flash-0731":         {0.22, 0.007, 0.66, 1048576},
		"accounts/fireworks/models/glm-5p2":                        {1.40, 0.14, 4.40, 1048576},
		"accounts/fireworks/models/kimi-k2p7-code":                 {0.95, 0.19, 4.00, 262144},
		"accounts/fireworks/models/minimax-m3":                     {0.30, 0.06, 1.20, 512000},
		"accounts/fireworks/models/minimax-m2p7":                   {0.30, 0.059, 1.20, 196608},
		"accounts/fireworks/models/gpt-oss-120b":                   {0.15, 0.015, 0.60, 131072},
		"accounts/fireworks/models/nemotron-3-ultra-nvfp4":         {0.60, 0.12, 2.40, 262144},
		"accounts/fireworks/models/deepseek-v4-flash-vision-exp":   {0.22, 0.007, 0.66, 1048576},
		"accounts/fireworks/models/nemotron-lightning-3p5-30b-a3b": {0.05, 0.01, 0.20, 262144},
		"accounts/fireworks/models/inkling":                        {1.00, 0.17, 4.05, 1048576},
		"accounts/fireworks/models/qwen3p7-plus":                   {0.40, 0.08, 1.60, 262144},
		"accounts/fireworks/models/kimi-k2p6":                      {0.95, 0.16, 4.00, 262144},
		"accounts/fireworks/models/qwen3-reranker-8b":              {0.20, 0, 0.20, 40960},
		"accounts/fireworks/models/qwen3-embedding-8b":             {0.10, 0, 0.10, 40960},
	}
	require.Len(t, currentServerlessModels, 20)

	for modelID, want := range currentServerlessModels {
		modelID, want := modelID, want
		t.Run(modelID, func(t *testing.T) {
			t.Parallel()

			got, ok := ModelRatios[modelID]
			require.True(t, ok, "current Fireworks serverless model is missing")

			require.InDelta(t, want.inputUSD, got.Ratio/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, want.cachedUSD, got.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, want.outputUSD, got.Ratio*got.CompletionRatio/ratio.MilliTokensUsd, 1e-12)
			require.Equal(t, want.contextLimit, got.ContextLength)
			require.NotEmpty(t, got.Description)
		})
	}
}

// TestCurrentServerlessCatalogCapabilities verifies modality, tool support, and
// canonical model-path metadata for the current Fireworks catalog.
func TestCurrentServerlessCatalogCapabilities(t *testing.T) {
	t.Parallel()

	for _, modelID := range []string{
		"accounts/fireworks/models/glm-5p3-flash",
		"accounts/fireworks/models/kimi-k3",
		"accounts/fireworks/models/muse-glimmer-30b",
		"accounts/fireworks/models/deepseek-v4-flash-vision-exp",
		"accounts/fireworks/models/kimi-k2p7-code",
		"accounts/fireworks/models/inkling",
		"accounts/fireworks/models/qwen3p7-plus",
		"accounts/fireworks/models/kimi-k2p6",
	} {
		require.Contains(t, ModelRatios[modelID].InputModalities, "image", "model=%s", modelID)
	}

	require.Equal(t, fwTextOnlyModalities, ModelRatios["accounts/fireworks/models/glm-5p3"].InputModalities)
	require.Equal(t, fwTextOnlyModalities, ModelRatios["accounts/fireworks/models/minimax-m3"].InputModalities)
	require.Contains(t, ModelRatios["accounts/fireworks/models/gpt-oss-120b"].SupportedFeatures, "tools")
	require.NotContains(t, ModelRatios, "accounts/fireworks/models/qwen3p8-max")
}
