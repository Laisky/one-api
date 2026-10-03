package openai

import (
	"context"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

// TestClaude431ImageReservation verifies documented image allowances without network fetches.
func TestClaude431ImageReservation(t *testing.T) {
	original := getImageSizeFn
	calls := 0
	getImageSizeFn = func(string) (int, int, error) { calls++; return 2000, 1500, nil }
	t.Cleanup(func() { getImageSizeFn = original })
	for _, detail := range []string{"", "auto", "low", "high"} {
		got, err := CountImageTokens("https://image.invalid/test.png", detail, "claude-sonnet-5-5")
		require.NoError(t, err)
		require.Equal(t, 4784, got)
	}
	require.Zero(t, calls)
	got, err := CountImageTokens("https://image.invalid/test.png", "low", "gpt-4o")
	require.NoError(t, err)
	require.Equal(t, 85, got)
	for _, content := range []any{
		[]any{map[string]any{"type": "file", "file_id": "image-test"}},
		[]model.MessageContent{{Type: "file", FileID: "image-test"}},
		[]any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://image.invalid/test.png", "detail": "low"}}},
	} {
		plain := []model.Message{{Role: "user", Content: []any{}}}
		image := []model.Message{{Role: "user", Content: content}}
		require.Equal(t, 4784, CountTokenMessages(context.Background(), image, "claude-sonnet-5-5")-CountTokenMessages(context.Background(), plain, "claude-sonnet-5-5"))
	}
	require.Zero(t, calls)
}

// TestClaude431AzureImageReservation resolves the canonical model through the real middleware context.
func TestClaude431AzureImageReservation(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	for _, deployment := range []string{"tenant-chat", "claude-tenant-chat"} {
		c.Set(ctxkey.Meta, &meta.Meta{ChannelType: channeltype.Azure, OriginModelName: "claude-sonnet-5-5", ActualModelName: deployment})
		plain := []model.Message{{Role: "user", Content: []any{}}}
		image := []model.Message{{Role: "user", Content: []model.MessageContent{{Type: "image_url", ImageURL: &model.ImageURL{Url: "https://image.invalid/test.png", Detail: "low"}}}}}
		ctx := gmw.Ctx(c)
		require.Equal(t, 4784, CountTokenMessages(ctx, image, deployment)-CountTokenMessages(ctx, plain, deployment))
	}
}
