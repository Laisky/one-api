package controller

import (
	"testing"

	"github.com/stretchr/testify/require"

	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

func TestGetImageCostRatio_Dalle3Tiers(t *testing.T) {
	t.Parallel()
	// standard 1024x1024 -> 1x
	r := &relaymodel.ImageRequest{Model: "dall-e-3", Size: "1024x1024", Quality: "standard"}
	v, err := getImageCostRatio(r, nil)
	require.NoError(t, err)
	require.Equal(t, float64(1), v)

	// standard 1024x1792 -> 2x
	r = &relaymodel.ImageRequest{Model: "dall-e-3", Size: "1024x1792", Quality: "standard"}
	v, err = getImageCostRatio(r, nil)
	require.NoError(t, err)
	require.Equal(t, float64(2), v)

	// hd 1024x1024 -> 2x
	r = &relaymodel.ImageRequest{Model: "dall-e-3", Size: "1024x1024", Quality: "hd"}
	v, err = getImageCostRatio(r, nil)
	require.NoError(t, err)
	require.Equal(t, float64(2), v)

	// hd 1024x1792 -> 3x
	r = &relaymodel.ImageRequest{Model: "dall-e-3", Size: "1024x1792", Quality: "hd"}
	v, err = getImageCostRatio(r, nil)
	require.NoError(t, err)
	require.Equal(t, float64(3), v)
}
