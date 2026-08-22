package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
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

// TestGetResponseAPIPromptTokens_CountsDeepSeekFileImages verifies file-backed
// DeepSeek vision inputs reserve the documented per-image upper bound.
// Parameters: t is the testing handle used for assertions and test lifecycle control.
// Returns: nothing; the test fails through t when a file image is omitted from estimation.
func TestGetResponseAPIPromptTokens_CountsDeepSeekFileImages(t *testing.T) {
	t.Parallel()

	modelName := "deepseek-v4-flash-vision-exp"
	base := &openai.ResponseAPIRequest{
		Model: modelName,
		Input: openai.ResponseAPIInput{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_text", "text": "describe this image"},
				},
			},
		},
	}
	baseTokens := getResponseAPIPromptTokens(context.Background(), base)
	for name, source := range map[string]map[string]any{
		"file_id":   {"file_id": "file-api-123"},
		"file_data": {"file_data": "data:image/png;base64,AAAA"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			content := []any{
				map[string]any{"type": "input_text", "text": "describe this image"},
				map[string]any{"type": "input_image"},
			}
			for key, value := range source {
				content[1].(map[string]any)[key] = value
			}
			fileRequest := &openai.ResponseAPIRequest{
				Model: modelName,
				Input: openai.ResponseAPIInput{
					map[string]any{"role": "user", "content": content},
				},
			}
			fileTokens := getResponseAPIPromptTokens(context.Background(), fileRequest)
			require.Equal(t, baseTokens+384, fileTokens)
		})
	}
}

// TestGetResponseAPIPromptTokens_CountsDeepSeekToolOutputFileImages verifies
// file-backed images nested in tool outputs reserve the same image estimate.
// Parameters: t is the testing handle used for assertions and test lifecycle control.
// Returns: nothing; the test fails through t when nested file images are omitted.
func TestGetResponseAPIPromptTokens_CountsDeepSeekToolOutputFileImages(t *testing.T) {
	t.Parallel()

	modelName := "deepseek-v4-flash-vision-exp"
	withImage := &openai.ResponseAPIRequest{
		Model: modelName,
		Input: openai.ResponseAPIInput{
			map[string]any{
				"type":    "function_call_output",
				"call_id": "call-image",
				"output": []any{
					map[string]any{"type": "input_image", "file_id": "file-api-123"},
				},
			},
		},
	}

	fileTokens := getResponseAPIPromptTokens(context.Background(), withImage)
	require.Equal(t, 384, fileTokens)
}
