package image

import (
	"encoding/base64"

	"github.com/Laisky/errors/v2"

	"github.com/songquanpeng/one-api/common/config"
)

// MaxInlineImageBytes returns the configured max inline image size in bytes.
// Returns: the max size in bytes based on MaxInlineImageSizeMB.
func MaxInlineImageBytes() int64 {
	return int64(config.MaxInlineImageSizeMB) * 1024 * 1024
}

// ValidateInlineImageBase64Size rejects base64 payloads exceeding the configured limit.
// Parameters: base64Data is the raw base64 payload without the data URL prefix.
// Returns: an error when the decoded size exceeds MaxInlineImageSizeMB.
func ValidateInlineImageBase64Size(base64Data string) error {
	maxSize := MaxInlineImageBytes()
	if int64(base64.StdEncoding.DecodedLen(len(base64Data))) > maxSize {
		return errors.Errorf("image size should not exceed %dMB", config.MaxInlineImageSizeMB)
	}
	return nil
}
