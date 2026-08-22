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

// TestConvertRequestEnablesStreamingUsage verifies streamed DeepSeek requests
// ask the upstream for its authoritative usage chunk before the terminal event.
func TestConvertRequestEnablesStreamingUsage(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)
	request := &model.GeneralOpenAIRequest{
		Model:  "deepseek-v4-flash-vision-exp",
		Stream: true,
		Messages: []model.Message{
			{Role: "user", Content: "Describe this image."},
		},
	}

	convertedAny, err := (&Adaptor{}).ConvertRequest(c, 0, request)
	require.NoError(t, err)
	converted, ok := convertedAny.(*model.GeneralOpenAIRequest)
	require.True(t, ok)
	require.NotNil(t, converted.StreamOptions)
	require.True(t, converted.StreamOptions.IncludeUsage)
}

// TestConvertClaudeRequestEnablesStreamingUsage verifies the Claude Messages
// conversion applies the same authoritative usage requirement as Chat format.
func TestConvertClaudeRequestEnablesStreamingUsage(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)
	stream := true
	request := &model.ClaudeRequest{
		Model:     "deepseek-v4-flash-vision-exp",
		MaxTokens: 128,
		Stream:    &stream,
		Messages:  []model.ClaudeMessage{{Role: "user", Content: "Describe this image."}},
	}

	convertedAny, err := (&Adaptor{}).ConvertClaudeRequest(c, request)
	require.NoError(t, err)
	converted, ok := convertedAny.(*model.GeneralOpenAIRequest)
	require.True(t, ok)
	require.NotNil(t, converted.StreamOptions)
	require.True(t, converted.StreamOptions.IncludeUsage)
}

// TestConvertRequestPreservesJSONMode verifies the documented json_object mode
// survives conversion to the DeepSeek Chat Completions request.
func TestConvertRequestPreservesJSONMode(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)
	request := &model.GeneralOpenAIRequest{
		Model:          "deepseek-v4-flash-vision-exp",
		ResponseFormat: &model.ResponseFormat{Type: "json_object"},
	}

	convertedAny, err := (&Adaptor{}).ConvertRequest(c, 0, request)
	require.NoError(t, err)
	converted, ok := convertedAny.(*model.GeneralOpenAIRequest)
	require.True(t, ok)
	require.NotNil(t, converted.ResponseFormat)
	require.Equal(t, "json_object", converted.ResponseFormat.Type)
}

// TestConvertRequestPreservesReasoningEffort verifies documented DeepSeek
// reasoning levels are forwarded instead of being silently discarded.
func TestConvertRequestPreservesReasoningEffort(t *testing.T) {
	t.Parallel()

	for _, effort := range []string{"low", "high", "max"} {
		t.Run(effort, func(t *testing.T) {
			t.Parallel()

			gin.SetMode(gin.TestMode)
			writer := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(writer)
			request := &model.GeneralOpenAIRequest{
				Model:           "deepseek-v4-flash-vision-exp",
				ReasoningEffort: &effort,
			}

			convertedAny, err := (&Adaptor{}).ConvertRequest(c, 0, request)
			require.NoError(t, err)
			converted, ok := convertedAny.(*model.GeneralOpenAIRequest)
			require.True(t, ok)
			require.NotNil(t, converted.ReasoningEffort)
			require.Equal(t, effort, *converted.ReasoningEffort)
		})
	}
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
