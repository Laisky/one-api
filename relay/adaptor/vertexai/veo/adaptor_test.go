package veo

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/billing/ratio"
)

// TestModelRatios_ShouldCoverCurrentVeoModels verifies that current Vertex Veo models
// publish billing metadata and use consistent ratio conversion.
func TestModelRatios_ShouldCoverCurrentVeoModels(t *testing.T) {
	t.Parallel()

	cases := map[string]float64{
		"veo-2.0-generate-001":          0.50,
		"veo-2.0-generate-exp":          0.50,
		"veo-2.0-generate-preview":      0.50,
		"veo-3.0-generate-001":          0.20,
		"veo-3.0-fast-generate-001":     0.10,
		"veo-3.0-generate-preview":      0.20,
		"veo-3.0-fast-generate-preview": 0.10,
		"veo-3.1-generate-001":          0.20,
		"veo-3.1-fast-generate-001":     0.10,
		"veo-3.1-generate-preview":      0.20,
		"veo-3.1-fast-generate-preview": 0.10,
	}

	for modelName, perSecondUSD := range cases {
		cfg, ok := ModelRatios[modelName]
		require.Truef(t, ok, "missing veo pricing for model %s", modelName)
		require.NotNilf(t, cfg.Video, "missing video pricing metadata for model %s", modelName)
		require.InDeltaf(t, perSecondUSD, cfg.Video.PerSecondUsd, 1e-12,
			"unexpected per-second usd for model %s", modelName)

		expectedRatio := perSecondUSD * ratio.QuotaPerUsd / float64(ratio.TokensPerSec)
		require.InDeltaf(t, expectedRatio, cfg.Ratio, 1e-12,
			"unexpected token ratio for model %s", modelName)
	}
}

// TestModelRatios_ShouldIncludeExpected4KMultipliers verifies 4k billing multipliers
// for Veo 3.1 families where pricing differs by output resolution.
func TestModelRatios_ShouldIncludeExpected4KMultipliers(t *testing.T) {
	t.Parallel()

	generate31 := ModelRatios["veo-3.1-generate-001"]
	require.NotNil(t, generate31.Video, "veo-3.1-generate-001 should define video pricing")
	require.InDelta(t, 2.0, generate31.Video.ResolutionMultipliers["4k"], 1e-12,
		"veo-3.1-generate-001 should use 2x multiplier for 4k")

	fast31 := ModelRatios["veo-3.1-fast-generate-001"]
	require.NotNil(t, fast31.Video, "veo-3.1-fast-generate-001 should define video pricing")
	require.InDelta(t, 3.0, fast31.Video.ResolutionMultipliers["4k"], 1e-12,
		"veo-3.1-fast-generate-001 should use 3x multiplier for 4k")
}
