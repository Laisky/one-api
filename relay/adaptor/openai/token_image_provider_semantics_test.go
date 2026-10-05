package openai

import (
	"context"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// stubImageSize makes countImageTokens measure every image as width x height.
func stubImageSize(t *testing.T, width, height int) {
	t.Helper()
	previous := getImageSizeFn
	getImageSizeFn = func(string) (int, int, error) { return width, height, nil }
	t.Cleanup(func() { getImageSizeFn = previous })
}

// TestSecurityOpenAIPatchImageDocExamples pins the patch algorithm to the
// worked gpt-6-astra high-detail examples in OpenAI's vision guide.
func TestSecurityOpenAIPatchImageDocExamples(t *testing.T) {
	for _, tc := range []struct {
		width, height, want int
	}{
		{1024, 1024, 1229},
		{2048, 2048, 3000},
		{4096, 512, 2458},
	} {
		stubImageSize(t, tc.width, tc.height)
		got, err := countImageTokens("fixture", "high", "gpt-6-astra")
		require.NoError(t, err)
		require.Equal(t, tc.want, got, "%dx%d", tc.width, tc.height)
	}
}

// TestSecurityOpenAIPatchDetailCannotUnderQuote proves low, auto, and omitted
// detail hints cannot quote patch-based models below their documented processing.
func TestSecurityOpenAIPatchDetailCannotUnderQuote(t *testing.T) {
	stubImageSize(t, 2048, 2048)
	for _, tc := range []struct {
		model, detail string
		want          int
	}{
		// gpt-5.4 low keeps a 6,144-patch budget: 64x64 patches x 1.2.
		{"gpt-5.4", "low", 4916},
		{"gpt-5.4-mini", "low", 4916},
		// gpt-5.4 auto behaves like high: 2,500 patches x 1.2.
		{"gpt-5.4", "auto", 3000},
		{"gpt-5.4", "", 3000},
		// gpt-5.5 auto and original keep up to 6000px / 10,000 patches.
		{"gpt-5.5", "auto", 4916},
		{"gpt-5.5", "original", 4916},
		{"chat-latest", "", 4916},
		// gpt-5.5 low fits 512x512: 16x16 patches x 1.2.
		{"gpt-5.5", "low", 308},
		// gpt-5.6 and gpt-6 auto preserve native dimensions.
		{"gpt-5.6-sol", "auto", 4916},
		{"gpt-6-astra", "", 4916},
		// gpt-5.2 and gpt-4.1-mini use one 2048px / 6,144-patch sizing for every level.
		{"gpt-5.2", "low", 4916},
		{"gpt-4.1-mini", "low", 6636},
	} {
		got, err := countImageTokens("fixture", tc.detail, tc.model)
		require.NoError(t, err, tc.model)
		require.Equal(t, tc.want, got, "%s detail=%q", tc.model, tc.detail)
	}

	stubImageSize(t, 8000, 6000)
	got, err := countImageTokens("fixture", "original", "gpt-5.6-sol")
	require.NoError(t, err)
	require.Equal(t, 36000, got, "native-size originals are bounded only by the 30,000-patch rejection limit")
	got, err = countImageTokens("fixture", "original", "gpt-5.5")
	require.NoError(t, err)
	// 6000x4500 after the edge limit, then scaled to 3669x2752 = 115x86 patches.
	require.Equal(t, 11868, got)
}

// TestSecurityOpenAIUnknownDetailReservesDocumentedMaximum proves unsupported
// or unmeasurable images on patch models reserve the documented maximum for the
// requested level, never the smaller legacy tile envelope.
func TestSecurityOpenAIUnknownDetailReservesDocumentedMaximum(t *testing.T) {
	previous := getImageSizeFn
	getImageSizeFn = func(string) (int, int, error) { return 0, 0, errors.New("synthetic metadata unavailable") }
	t.Cleanup(func() { getImageSizeFn = previous })
	for _, tc := range []struct {
		model, detail string
		want          int
	}{
		{"gpt-5.5", "original", 12000},
		{"gpt-5.5", "", 12000},
		{"gpt-5.5", "high", 3000},
		{"gpt-5.5", "low", 308},
		{"gpt-5.5", "unsupported", 12000},
		{"gpt-5.6-sol", "", 36000},
		{"gpt-5.4", "low", 4916},
		{"gpt-5.7", "", 36000},
		{"claude-opus-4-7", "", 4784},
		{"claude-sonnet-4-5", "", 3279},
	} {
		image := []model.Message{{Role: "user", Content: []model.MessageContent{{Type: model.ContentTypeImageURL, ImageURL: &model.ImageURL{Url: "https://fixture.invalid/image", Detail: tc.detail}}}}}
		plain := []model.Message{{Role: "user", Content: []model.MessageContent{}}}
		got := CountTokenMessages(context.Background(), image, tc.model) - CountTokenMessages(context.Background(), plain, tc.model)
		require.Equal(t, tc.want, got, "%s detail=%q", tc.model, tc.detail)
	}
}

// TestSecurityGenericProviderLowDetailIsNotDiscounted proves the OpenAI-only
// low hint cannot discount models whose providers process the complete image,
// while OpenAI tile models keep their documented low-detail price.
func TestSecurityGenericProviderLowDetailIsNotDiscounted(t *testing.T) {
	stubImageSize(t, 1024, 1024)
	for _, name := range []string{"qwen3-vl-plus", "glm-4.6v", "grok-4", "llama-4-maverick"} {
		high, err := countImageTokens("fixture", "high", name)
		require.NoError(t, err)
		low, err := countImageTokens("fixture", "low", name)
		require.NoError(t, err)
		require.Equal(t, high, low, name)
	}
	for name, want := range map[string]int{"gpt-4o": 85, "gpt-4.1": 85, "gpt-5.1": 70, "gpt-5-chat-latest": 70, "o3": 75, "gpt-4o-mini": gpt4oMiniLowDetailCost} {
		low, err := countImageTokens("fixture", "low", name)
		require.NoError(t, err)
		require.Equal(t, want, low, name)
	}
}

// TestSecurityClaudeHighResolutionImages proves models with high-resolution
// vision are quoted at their larger resize limit while legacy models keep theirs.
func TestSecurityClaudeHighResolutionImages(t *testing.T) {
	stubImageSize(t, 2576, 1449)
	for _, name := range []string{"claude-opus-4-7", "claude-opus-4-8", "claude-sonnet-5", "claude-opus-5-5", "claude-fable-5-1"} {
		got, err := countImageTokens("fixture", "low", name)
		require.NoError(t, err)
		require.Equal(t, 4784, got, name)
	}
	for _, name := range []string{"claude-sonnet-4-5", "claude-sonnet-4-20250514", "claude-opus-4-1-20250805", "claude-haiku-4-5@20251001", "claude-3-5-sonnet-20241022", "claude-sonnet-4-6"} {
		got, err := countImageTokens("fixture", "low", name)
		require.NoError(t, err)
		require.Equal(t, 1844, got, name) // ceil(1568 x 882 / 750)
	}
}
