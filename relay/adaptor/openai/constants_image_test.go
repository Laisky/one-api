package openai

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/billing/ratio"
)

func TestGptImage1HasDualPricing(t *testing.T) {
	cfg, ok := ModelRatios["gpt-image-1"]
	require.True(t, ok, "gpt-image-1 not found in ModelRatios")
	require.InDelta(t, 5.0*ratio.MilliTokensUsd, cfg.Ratio, 1e-9, "unexpected input ratio for gpt-image-1")
	require.InDelta(t, 1.25*ratio.MilliTokensUsd, cfg.CachedInputRatio, 1e-9, "unexpected cached ratio for gpt-image-1")
	require.NotNil(t, cfg.Image, "expected image config for gpt-image-1")
	require.Greater(t, cfg.Image.PricePerImageUsd, 0.0, "expected image price for gpt-image-1")
}

func TestGptImage1MiniHasDualPricing(t *testing.T) {
	cfg, ok := ModelRatios["gpt-image-1-mini"]
	require.True(t, ok, "gpt-image-1-mini not found in ModelRatios")
	require.InDelta(t, 2.0*ratio.MilliTokensUsd, cfg.Ratio, 1e-9, "unexpected input ratio for gpt-image-1-mini")
	require.InDelta(t, 0.2*ratio.MilliTokensUsd, cfg.CachedInputRatio, 1e-9, "unexpected cached ratio for gpt-image-1-mini")
	require.NotNil(t, cfg.Image, "expected image config for gpt-image-1-mini")
	require.Greater(t, cfg.Image.PricePerImageUsd, 0.0, "expected image price for gpt-image-1-mini")
}

func TestNewGptImageModelsPricing(t *testing.T) {
	for _, model := range []string{"chatgpt-image-latest", "gpt-image-1.5", "gpt-image-1.5-2025-12-16"} {
		cfg, ok := ModelRatios[model]
		require.True(t, ok, "%s not found in ModelRatios", model)
		require.InDelta(t, 5.0*ratio.MilliTokensUsd, cfg.Ratio, 1e-9, "unexpected input ratio for %s", model)
		require.InDelta(t, 1.25*ratio.MilliTokensUsd, cfg.CachedInputRatio, 1e-9, "unexpected cached ratio for %s", model)
		require.InDelta(t, 2.0, cfg.CompletionRatio, 1e-9, "unexpected completion ratio for %s", model)
		require.NotNil(t, cfg.Image, "expected image config for %s", model)
		require.Greater(t, cfg.Image.PricePerImageUsd, 0.0, "expected image price for %s", model)
	}
}
