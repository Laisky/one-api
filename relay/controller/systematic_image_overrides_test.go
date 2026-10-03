package controller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	persistmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/xai"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
)

// TestSystematicImageTariffOverridePreservesDefaults verifies that explicit
// tariffs replace rather than add to provider fees. Parameters: t runs the
// assertions. Returns: none; no billable inference is performed.
func TestSystematicImageTariffOverridePreservesDefaults(t *testing.T) {
	const name = "grok-imagine-image-2.0"
	at := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	original := xai.ModelRatios[name].Image.Clone()
	local := &persistmodel.ImagePricingLocal{PricePerImageUsd: 0.123}
	cfg, ok := pricing.ResolveImagePricing(name, map[string]persistmodel.ModelConfigLocal{name: {Image: local}}, &xai.Adaptor{}, at)
	require.True(t, ok)
	require.InDelta(t, 0.123, cfg.PricePerImageUsd, 1e-12)
	require.Equal(t, original.DefaultSize, cfg.DefaultSize)
	require.Equal(t, original.DefaultQuality, cfg.DefaultQuality)
	require.Equal(t, original.MinImages, cfg.MinImages)
	require.Equal(t, original.MaxImages, cfg.MaxImages)
	require.Empty(t, cfg.QualitySizeMultipliers, "do not silently add provider multipliers to an explicit flat tariff")
	req := &relaymodel.ImageRequest{Model: name, Prompt: "test", N: 1}
	applyImageDefaults(req, cfg)
	multiplier, err := getImageCostRatio(req, cfg)
	require.NoError(t, err)
	require.Equal(t, 1.0, multiplier)
	cfg.DefaultSize = "changed"
	require.Equal(t, original, xai.ModelRatios[name].Image)
	require.Empty(t, local.DefaultSize)
}

// TestSystematicImageDefaultsPreserveDatedTariff checks both sides of the
// migration boundary and configuration isolation. Parameters: t runs cases.
// Returns: none; the time is injected rather than read from the wall clock.
func TestSystematicImageDefaultsPreserveDatedTariff(t *testing.T) {
	const name = "grok-imagine-image-quality"
	original := xai.ModelRatios[name].Image.Clone()
	for _, tc := range []struct {
		at string
		usd float64
	}{
		{"2026-11-01T23:59:59Z", 0.07},
		{"2026-11-02T00:00:00Z", 0.06},
	} {
		t.Run(tc.at, func(t *testing.T) {
			at, err := time.Parse(time.RFC3339, tc.at)
			require.NoError(t, err)
			local := &persistmodel.ImagePricingLocal{DefaultSize: "2048x2048", MaxImages: 2}
			cfg, ok := pricing.ResolveImagePricing(name, map[string]persistmodel.ModelConfigLocal{name: {Image: local}}, &xai.Adaptor{}, at)
			require.True(t, ok)
			require.Equal(t, 2, cfg.MaxImages)
			req := &relaymodel.ImageRequest{Model: name, Prompt: "test", N: 1}
			applyImageDefaults(req, cfg)
			multiplier, err := getImageCostRatio(req, cfg)
			require.NoError(t, err)
			require.InDelta(t, tc.usd, cfg.PricePerImageUsd*multiplier, 1e-12)
			cfg.SizeMultipliers["2048x2048"] = 999
			require.Equal(t, original, xai.ModelRatios[name].Image)
			require.Nil(t, local.SizeMultipliers)
		})
	}
}

// TestSystematicUnknownImageDefaultsDoNotInventTariff preserves unknown vendor
// spelling and does not fabricate a render charge. Parameters: t runs checks.
// Returns: none; assertions verify unconfigured-model compatibility.
func TestSystematicUnknownImageDefaultsDoNotInventTariff(t *testing.T) {
	req := &relaymodel.ImageRequest{Model: "tenant-image-model", Size: "CustomSize", Quality: "VendorQuality", N: 1}
	applyImageDefaults(req, nil)
	require.Equal(t, "CustomSize", req.Size)
	require.Equal(t, "VendorQuality", req.Quality)
	cfg, ok := pricing.ResolveImagePricing(req.Model, map[string]persistmodel.ModelConfigLocal{
		req.Model: {Image: &persistmodel.ImagePricingLocal{DefaultSize: "1024x1024"}},
	}, nil, time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC))
	require.True(t, ok)
	require.Zero(t, cfg.PricePerImageUsd)
}
