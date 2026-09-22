package controller

import (
	"encoding/json"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/xai"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestSystematicImageResolutionBilling checks actual provider conversion after
// tier preparation. Parameters: t runs the cases. Returns: none; no HTTP call.
func TestSystematicImageResolutionBilling(t *testing.T) {
	for _, tc := range []struct {
		name, size, resolution, wantSize string
		wantMultiplier                  float64
		wantError                       bool
	}{
		{"native_2k", "", "2k", "2048x2048", 2, false},
		{"native_normalized", "", " 2K ", "2048x2048", 2, false},
		{"native_1k", "", "1k", "1024x1024", 1.5, false},
		{"size_2k", "2048 × 2048", "", "2048x2048", 2, false},
		{"consistent", "2048X2048", "2k", "2048x2048", 2, false},
		{"conflicting", "1024x1024", "2k", "", 0, true},
		{"unsupported_resolution", "", "1.5k", "", 0, true},
		{"unrepresentable_size", "1408x1408", "", "", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := &relaymodel.ImageRequest{Model: "grok-imagine-image-2.0", Prompt: "test", Quality: "medium", Size: tc.size, Resolution: tc.resolution, N: 2}
			cfg := xai.ModelRatios[req.Model].Image.Clone()
			err := prepareImageRequest(req, cfg, &metalib.Meta{ChannelType: channeltype.XAI})
			if tc.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Nil(t, validateImageRequest(req, nil, cfg))
			before := *req
			converted, err := convertImageRequestForUpstream(nil, req, (&xai.Adaptor{}).ConvertImageRequest)
			require.NoError(t, err)
			require.Equal(t, before, *req, "converter must not destroy pricing/log fields")
			wire, err := json.Marshal(converted)
			require.NoError(t, err)
			var payload map[string]any
			require.NoError(t, json.Unmarshal(wire, &payload))
			wantResolution := "1k"
			if tc.wantSize == "2048x2048" {
				wantResolution = "2k"
			}
			require.Equal(t, wantResolution, payload["resolution"])
			require.Equal(t, "medium", payload["quality"])
			require.NotContains(t, payload, "size")
			multiplier, err := getImageCostRatio(req, cfg)
			require.NoError(t, err)
			require.Equal(t, tc.wantMultiplier, multiplier)
			override, ok := getChannelImageTierOverride(map[string]float64{
				"$image-tier:" + req.Model + "|size=" + tc.wantSize + "|quality=medium": 3.5,
			}, req.Model, req.Size, req.Quality)
			require.True(t, ok)
			require.Equal(t, 3.5, override)
		})
	}
}

// TestSystematicImageConversionIsolation covers pointer fields and failing
// converters. Parameters: t runs success/error cases. Returns: none; all
// mutations are deliberately performed by an in-memory destructive converter.
func TestSystematicImageConversionIsolation(t *testing.T) {
	for _, fail := range []bool{false, true} {
		req := &relaymodel.ImageRequest{Model: "custom", Size: "1024x1024", Quality: "high", N: 2, ResponseFormat: stringPtr("url"), ImagePrompt: stringPtr("original")}
		before, err := json.Marshal(req)
		require.NoError(t, err)
		_, err = convertImageRequestForUpstream(nil, req, func(_ *gin.Context, private *relaymodel.ImageRequest) (any, error) {
			private.Model = "provider-name"
			private.Size = ""
			private.Quality = ""
			private.N = 1
			*private.ResponseFormat = "b64_json"
			*private.ImagePrompt = "changed"
			if fail {
				return nil, errors.New("test conversion failure")
			}
			return private, nil
		})
		require.Equal(t, fail, err != nil)
		after, err := json.Marshal(req)
		require.NoError(t, err)
		require.Equal(t, before, after)
	}
	unknown := &relaymodel.ImageRequest{Model: "grok-imagine-image-future", Size: "CustomSize", Quality: "VendorQuality", Resolution: "4k"}
	require.NoError(t, prepareImageRequest(unknown, nil, &metalib.Meta{ChannelType: channeltype.XAI}))
	require.Equal(t, "CustomSize", unknown.Size)
	require.Equal(t, "4k", unknown.Resolution)
}
