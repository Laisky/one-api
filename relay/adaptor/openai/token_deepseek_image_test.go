package openai

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCountImageTokens_DeepSeekVisionUsesDocumentedUpperBound verifies image
// estimation uses DeepSeek's fixed upper bound without fetching image data.
// Parameters: t is the testing handle used for assertions and test lifecycle control.
// Returns: nothing; the test fails through t when estimation fetches or miscounts an image.
func TestCountImageTokens_DeepSeekVisionUsesDocumentedUpperBound(t *testing.T) {
	old := getImageSizeFn
	getImageSizeFn = func(_ string) (int, int, error) {
		t.Fatal("DeepSeek pre-consume estimation must not fetch the image")
		return 0, 0, nil
	}
	defer func() { getImageSizeFn = old }()

	for _, detail := range []string{"", "auto", "low", "high", "original"} {
		t.Run(detail, func(t *testing.T) {
			got, err := countImageTokens(
				"https://example.com/image.webp",
				detail,
				"deepseek-v4-flash-vision-exp",
			)
			require.NoError(t, err)
			require.Equal(t, 384, got)
		})
	}
}
