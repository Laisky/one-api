package utils

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common/config"
)

// TestDownloadImageFromURL_DataURISizeLimit ensures oversized inline images are rejected.
func TestDownloadImageFromURL_DataURISizeLimit(t *testing.T) {
	prevMax := config.MaxInlineImageSizeMB
	config.MaxInlineImageSizeMB = 1
	t.Cleanup(func() { config.MaxInlineImageSizeMB = prevMax })

	payload := make([]byte, 1024*1024+1)
	encoded := base64.StdEncoding.EncodeToString(payload)
	dataURL := "data:image/png;base64," + encoded

	_, _, err := DownloadImageFromURL(context.Background(), dataURL)
	require.Error(t, err)
	require.Contains(t, err.Error(), "image size should not exceed")
}
