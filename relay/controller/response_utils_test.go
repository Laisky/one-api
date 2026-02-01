package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/adaptor/openai"
)

// TestGetResponseAPIPromptTokens_CountsImageInputs verifies image inputs are counted.
func TestGetResponseAPIPromptTokens_CountsImageInputs(t *testing.T) {
	req := &openai.ResponseAPIRequest{
		Model: "gpt-4.1",
		Input: openai.ResponseAPIInput{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_text", "text": "draw a cat"},
					map[string]any{
						"type":      "input_image",
						"image_url": "https://example.com/a.png",
						"detail":    "low",
					},
				},
			},
		},
	}

	ctx := context.Background()
	imageTokens, err := openai.CountImageTokens("https://example.com/a.png", "low", req.Model)
	require.NoError(t, err, "count image tokens")

	tokensPerMessage, _ := responseMessageTokenOverhead(req.Model)
	expected := tokensPerMessage +
		openai.CountTokenText("user", req.Model) +
		openai.CountTokenText("draw a cat", req.Model) +
		imageTokens
	got := getResponseAPIPromptTokens(ctx, req)

	require.Equal(t, expected, got, "prompt token counting should include image input")
	require.Greater(t, got, 0, "prompt tokens should be positive")
}
