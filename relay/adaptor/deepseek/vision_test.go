package deepseek

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestConvertRequestPreservesVisionContent verifies that the DeepSeek adaptor
// forwards OpenAI-compatible multimodal content without flattening or dropping
// the image_url block. DeepSeek accepts detail=original for the vision model.
func TestConvertRequestPreservesVisionContent(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	content := []any{
		map[string]any{"type": "text", "text": "Describe this image."},
		map[string]any{
			"type": "image_url",
			"image_url": map[string]any{
				"url":    "https://example.com/cat.webp",
				"detail": "original",
			},
		},
	}
	request := &model.GeneralOpenAIRequest{
		Model: "deepseek-v4-flash-vision-exp",
		Messages: []model.Message{
			{Role: "user", Content: content},
		},
	}

	convertedAny, err := (&Adaptor{}).ConvertRequest(c, 0, request)
	require.NoError(t, err)

	converted, ok := convertedAny.(*model.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Equal(t, "deepseek-v4-flash-vision-exp", converted.Model)
	require.Equal(t, content, converted.Messages[0].Content)
}

// TestVisionModelUsesPromptTokenPricing prevents accidental use of the image
// generation pricing path. DeepSeek reports image-derived tokens in prompt_tokens,
// so the vision model must inherit Flash input/cache/output ratios and leave the
// generated-image pricing block unset.
func TestVisionModelUsesPromptTokenPricing(t *testing.T) {
	t.Parallel()

	flash := ModelRatios["deepseek-v4-flash"]
	vision := ModelRatios["deepseek-v4-flash-vision-exp"]

	require.Equal(t, flash.Ratio, vision.Ratio)
	require.Equal(t, flash.CachedInputRatio, vision.CachedInputRatio)
	require.Equal(t, flash.CompletionRatio, vision.CompletionRatio)
	require.Equal(t, flash.TimeWindows, vision.TimeWindows)
	require.Nil(t, vision.Image)
}
