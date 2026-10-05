package openai

import (
	"context"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/one-api/relay/model"
	"github.com/stretchr/testify/require"
)

// TestImageDetailCannotDiscountUnchangedProviderInput compares provider image quotes using a fixed local geometry fixture.
func TestImageDetailCannotDiscountUnchangedProviderInput(t *testing.T) {
	previous := getImageSizeFn
	getImageSizeFn = func(string) (int, int, error) { return 1024, 1024, nil }
	t.Cleanup(func() { getImageSizeFn = previous })
	for _, name := range []string{"gemini-2.5-flash", "gemini-3-pro-preview", "claude-sonnet-4", "claude-sonnet-5-5", "deepseek-v4-flash"} {
		t.Run(name, func(t *testing.T) {
			high, err := countImageTokens("fixture", "high", name)
			require.NoError(t, err)
			require.Positive(t, high)
			for _, detail := range []string{"", "auto", "low"} {
				actual, err := countImageTokens("fixture", detail, name)
				require.NoError(t, err)
				require.Equal(t, high, actual, "a hint ignored by the provider must not discount its full image")
			}
		})
	}
	low, err := countImageTokens("fixture", "low", "gpt-4o")
	require.NoError(t, err)
	high, err := countImageTokens("fixture", "high", "gpt-4o")
	require.NoError(t, err)
	require.Less(t, low, high, "native OpenAI detail remains a supported processing control")
}

// TestImageEstimationFailureRetainsAdmissionCost prevents inaccessible or malformed image metadata from removing image reservation.
func TestImageEstimationFailureRetainsAdmissionCost(t *testing.T) {
	previous := getImageSizeFn
	getImageSizeFn = func(string) (int, int, error) { return 0, 0, errors.New("synthetic metadata unavailable") }
	t.Cleanup(func() { getImageSizeFn = previous })
	for _, name := range []string{"gpt-4o", "gpt-4o-mini", "claude-sonnet-4", "gemini-2.5-flash"} {
		plain := []model.Message{{Role: "user", Content: "hello"}}
		text := "hello"
		imaged := []model.Message{{Role: "user", Content: []model.MessageContent{{Type: model.ContentTypeText, Text: &text}, {Type: model.ContentTypeImageURL, ImageURL: &model.ImageURL{Url: "https://fixture.invalid/image", Detail: "high"}}}}}
		base := CountTokenMessages(context.Background(), plain, name)
		quoted := CountTokenMessages(context.Background(), imaged, name)
		require.Greater(t, quoted-base, 1000, "failed metadata must not admit a billable image for free")
	}
}
