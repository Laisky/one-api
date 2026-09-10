package openai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestDeepSeekReviewImageEstimationNoFetch verifies every Flash alias reserves
// the documented image bound without image downloads or OpenAI tile accounting.
func TestDeepSeekReviewImageEstimationNoFetch(t *testing.T) {
	original := getImageSizeFn
	fetches := 0
	getImageSizeFn = func(string) (int, int, error) {
		fetches++
		return 512, 512, nil
	}
	t.Cleanup(func() { getImageSizeFn = original })
	for _, name := range []string{"deepseek-flash", "deepseek-v4-flash", "deepseek-v4-flash-vision-exp"} {
		for _, detail := range []string{"", "auto", "low", "high", "original"} {
			for _, source := range []string{"https://image.invalid/picture.png", "data:image/png;base64,AAAA"} {
				t.Run(name+"/"+detail+"/"+source, func(t *testing.T) {
					before := fetches
					tokens, err := CountImageTokens(source, detail, name)
					t.Run("tokens", func(t *testing.T) {
						require.NoError(t, err)
						require.Equal(t, 1024, tokens)
					})
					t.Run("no-fetch", func(t *testing.T) {
						require.Equal(t, before, fetches, "DeepSeek preflight must not fetch image dimensions")
					})
				})
			}
		}
	}
	// A non-DeepSeek model must retain the existing OpenAI formula.
	tokens, err := CountImageTokens("https://image.invalid/picture.png", "high", "gpt-4o")
	require.NoError(t, err)
	require.Equal(t, 255, tokens)
	require.Greater(t, fetches, 0)
}

// TestDeepSeekReviewChatImageReservation decodes Chat payloads and checks URL,
// inline and file-backed image deltas, including typed Claude-conversion content.
func TestDeepSeekReviewChatImageReservation(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"deepseek-flash", "deepseek-v4-flash", "deepseek-v4-flash-vision-exp"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			text := map[string]any{"type": "text", "text": strings.Repeat("Describe this image. ", 10)}
			count := func(content any) int {
				body, err := json.Marshal(map[string]any{"model": name, "messages": []any{map[string]any{"role": "user", "content": content}}})
				require.NoError(t, err)
				var request model.GeneralOpenAIRequest
				require.NoError(t, json.Unmarshal(body, &request))
				return CountTokenMessages(context.Background(), request.Messages, request.Model)
			}
			base := count([]any{text})
			images := []any{
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://image.invalid/picture.png", "detail": "low"}},
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,AAAA", "detail": "low"}},
				map[string]any{"type": "file", "file_id": "file-api-image"},
				map[string]any{"type": "file", "file_data": "data:image/png;base64,AAAA"},
			}
			for index, image := range images {
				t.Run([]string{"url", "inline", "file_id", "file_data"}[index], func(t *testing.T) {
					require.Equal(t, base+1024, count([]any{text, image}))
				})
			}
			t.Run("multiple", func(t *testing.T) {
				require.Equal(t, base+len(images)*1024, count(append([]any{text}, images...)))
			})
			t.Run("empty", func(t *testing.T) {
				require.Equal(t, base, count([]any{text, map[string]any{"type": "file", "file_id": " "}}))
			})
			t.Run("typed-file", func(t *testing.T) {
				plain := []model.Message{{Role: "user", Content: []model.MessageContent{}}}
				typed := []model.Message{{Role: "user", Content: []model.MessageContent{{Type: model.ContentTypeFile, FileID: "file-api-image"}}}}
				require.Equal(t, CountTokenMessages(context.Background(), plain, name)+1024, CountTokenMessages(context.Background(), typed, name))
			})
		})
	}
}
