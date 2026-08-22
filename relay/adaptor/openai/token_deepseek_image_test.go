package openai

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
