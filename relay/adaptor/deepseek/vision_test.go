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
// Parameters: t is the testing handle used for assertions and test lifecycle control.
// Returns: nothing; the test fails through t when conversion changes the content.
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
// Parameters: t is the testing handle used for assertions and test lifecycle control.
// Returns: nothing; the test fails through t when usage reporting is not enabled.
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
// Parameters: t is the testing handle used for assertions and test lifecycle control.
// Returns: nothing; the test fails through t when usage reporting is not enabled.
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

// TestConvertClaudeRequestPreservesFileBackedVision verifies Claude Files API
// image sources survive conversion to DeepSeek Chat content parts.
// Parameters: t is the testing handle used for assertions and test lifecycle control.
// Returns: nothing; the test fails through t when a file image is dropped.
func TestConvertClaudeRequestPreservesFileBackedVision(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)
	request := &model.ClaudeRequest{
		Model:     "deepseek-v4-flash-vision-exp",
		MaxTokens: 128,
		Messages: []model.ClaudeMessage{{
			Role: "user",
			Content: []any{
				map[string]any{"type": "text", "text": "describe"},
				map[string]any{
					"type": "image",
					"source": map[string]any{
						"type":    "file",
						"file_id": "file-api-123",
					},
				},
			},
		}},
	}

	convertedAny, err := (&Adaptor{}).ConvertClaudeRequest(c, request)
	require.NoError(t, err)
	converted, ok := convertedAny.(*model.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Len(t, converted.Messages, 1)
	parts, ok := converted.Messages[0].Content.([]model.MessageContent)
	require.True(t, ok)
	require.Len(t, parts, 2)
	require.Equal(t, model.ContentTypeFile, parts[1].Type)
	require.Equal(t, "file-api-123", parts[1].FileID)
}

// TestConvertRequestPreservesJSONMode verifies the documented json_object mode
// survives conversion to the DeepSeek Chat Completions request.
// Parameters: t is the testing handle used for assertions and test lifecycle control.
// Returns: nothing; the test fails through t when JSON mode is discarded.
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
// Parameters: t is the testing handle used for assertions and test lifecycle control.
// Returns: nothing; the test fails through t when a documented effort is dropped.
func TestConvertRequestPreservesReasoningEffort(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		input    string
		expected string
	}{
		{input: "low", expected: "high"},
		{input: "high", expected: "high"},
		{input: "max", expected: "max"},
		{input: "medium", expected: "high"},
		{input: "xhigh", expected: "max"},
	} {
		testCase := testCase
		t.Run(testCase.input, func(t *testing.T) {
			t.Parallel()

			gin.SetMode(gin.TestMode)
			writer := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(writer)
			request := &model.GeneralOpenAIRequest{
				Model:           "deepseek-v4-flash-vision-exp",
				ReasoningEffort: &testCase.input,
			}

			convertedAny, err := (&Adaptor{}).ConvertRequest(c, 0, request)
			require.NoError(t, err)
			converted, ok := convertedAny.(*model.GeneralOpenAIRequest)
			require.True(t, ok)
			require.NotNil(t, converted.ReasoningEffort)
			require.Equal(t, testCase.expected, *converted.ReasoningEffort)
		})
	}
}

// TestConvertRequestMapsMaxCompletionTokens verifies DeepSeek receives the
// documented max_tokens field when callers use OpenAI's portable alias.
// Parameters: t is the testing handle used for assertions and test lifecycle control.
// Returns: nothing; the test fails through t when the output limit is not mapped.
func TestConvertRequestMapsMaxCompletionTokens(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)
	limit := 128
	request := &model.GeneralOpenAIRequest{
		Model:               "deepseek-v4-flash-vision-exp",
		MaxCompletionTokens: &limit,
	}

	convertedAny, err := (&Adaptor{}).ConvertRequest(c, 0, request)
	require.NoError(t, err)
	converted, ok := convertedAny.(*model.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Equal(t, limit, converted.MaxTokens)
	require.Nil(t, converted.MaxCompletionTokens)
}

// TestVisionModelUsesPromptTokenPricing prevents accidental use of the image
// generation pricing path. DeepSeek reports image-derived tokens in prompt_tokens,
// so the vision model must inherit Flash input/cache/output ratios and leave the
// generated-image pricing block unset.
// Parameters: t is the testing handle used for assertions and test lifecycle control.
// Returns: nothing; the test fails through t when vision pricing diverges from Flash.
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
