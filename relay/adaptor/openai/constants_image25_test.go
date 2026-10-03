package openai

import (
	"testing"

	"github.com/stretchr/testify/require"

	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
)

// TestGPTImage25Catalog verifies the four documented aliases and snapshots.
// Parameters: t is the test runner. Returns: none; inconsistent metadata fails the test.
func TestGPTImage25Catalog(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"gpt-image-2.5-sunburst",
		"gpt-image-2.5-sunburst-2026-09-08",
		"gpt-image-2.5-flare",
		"gpt-image-2.5-flare-2026-09-08",
	} {
		t.Run(name, func(t *testing.T) {
			cfg, ok := ModelRatios[name]
			require.True(t, ok)
			require.Contains(t, ModelList, name)
			require.InDelta(t, 5*billingratio.MilliTokensUsd, cfg.Ratio, 1e-12)
			require.InDelta(t, 1.25*billingratio.MilliTokensUsd, cfg.CachedInputRatio, 1e-12)
			require.InDelta(t, 30*billingratio.MilliTokensUsd, cfg.Ratio*cfg.CompletionRatio, 1e-12)
			require.Equal(t, []string{"text", "image"}, cfg.InputModalities)
			require.Equal(t, []string{"image"}, cfg.OutputModalities)
			require.Zero(t, cfg.ContextLength, "do not invent a context window")
			require.Zero(t, cfg.MaxOutputTokens, "do not invent an output-token limit")
			require.Empty(t, cfg.SupportedReasoningEfforts)
			require.Empty(t, cfg.SupportedFeatures)
			require.Empty(t, cfg.Tiers)
			require.NotEmpty(t, cfg.Description)

			require.NotNil(t, cfg.Image)
			require.True(t, cfg.Image.HasData(), "token-only image metadata must remain discoverable")
			require.Zero(t, cfg.Image.PricePerImageUsd, "actual output-token billing must not add a render fee")
			require.InDelta(t, 8.0/5.0, cfg.Image.PromptRatio, 1e-12)
			require.Equal(t, "auto", cfg.Image.DefaultSize)
			require.Equal(t, "auto", cfg.Image.DefaultQuality)
			require.Equal(t, 32000, cfg.Image.PromptTokenLimit)
			require.Equal(t, 1, cfg.Image.MinImages)
			require.Equal(t, 10, cfg.Image.MaxImages)
			require.Empty(t, cfg.Image.SizeMultipliers)
			require.Empty(t, cfg.Image.QualityMultipliers)
			require.Empty(t, cfg.Image.QualitySizeMultipliers)
		})
	}
}

// TestGPTImage25SnapshotParity verifies that snapshots retain their alias metadata.
// Parameters: t is the test runner. Returns: none; unexpected differences fail the test.
func TestGPTImage25SnapshotParity(t *testing.T) {
	t.Parallel()
	for _, alias := range []string{"gpt-image-2.5-sunburst", "gpt-image-2.5-flare"} {
		base := ModelRatios[alias]
		snapshot := ModelRatios[alias+"-2026-09-08"]
		base.Description = ""
		snapshot.Description = ""
		require.Equal(t, base, snapshot, alias)
	}
}

// TestNewGPTImage25ConfigIsolation verifies that configurations do not share mutable state.
// Parameters: t is the test runner. Returns: none; shared pointers or slices fail the test.
func TestNewGPTImage25ConfigIsolation(t *testing.T) {
	t.Parallel()
	first := newGPTImage25Config("first")
	second := newGPTImage25Config("second")
	first.Image.DefaultQuality = "high"
	first.InputModalities[0] = "modified"
	first.OutputModalities[0] = "modified"
	require.Equal(t, "auto", second.Image.DefaultQuality)
	require.Equal(t, []string{"text", "image"}, second.InputModalities)
	require.Equal(t, []string{"image"}, second.OutputModalities)
}
