package togetherai

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestGetRequestURL verifies TogetherAI rewrites Claude Messages requests to chat completions
// and preserves the documented OpenAI-compatible request paths.
func TestGetRequestURL(t *testing.T) {
	t.Parallel()
	adaptor := &Adaptor{}

	tests := []struct {
		name         string
		requestPath  string
		expectedPath string
	}{
		{
			name:         "Claude Messages converts to Chat Completions",
			requestPath:  "/v1/messages",
			expectedPath: "/v1/chat/completions",
		},
		{
			name:         "Image generations passes through",
			requestPath:  "/v1/images/generations",
			expectedPath: "/v1/images/generations",
		},
		{
			name:         "Audio speech passes through",
			requestPath:  "/v1/audio/speech",
			expectedPath: "/v1/audio/speech",
		},
		{
			name:         "Completions passes through",
			requestPath:  "/v1/completions",
			expectedPath: "/v1/completions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			metaInfo := &meta.Meta{
				BaseURL:        "https://api.together.xyz",
				RequestURLPath: tt.requestPath,
			}

			url, err := adaptor.GetRequestURL(metaInfo)
			require.NoError(t, err)
			assert.Equal(t, "https://api.together.xyz"+tt.expectedPath, url)
		})
	}
}

// TestConvertRequestPreservesReasoningEffort verifies TogetherAI keeps reasoning_effort intact
// because the current compatibility docs explicitly support it.
func TestConvertRequestPreservesReasoningEffort(t *testing.T) {
	t.Parallel()
	adaptor := &Adaptor{}
	effort := "high"
	request := &model.GeneralOpenAIRequest{
		Model:           "deepseek-ai/DeepSeek-R1",
		ReasoningEffort: &effort,
		Messages:        []model.Message{{Role: "user", Content: "hello"}},
	}

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	result, err := adaptor.ConvertRequest(c, relaymode.ChatCompletions, request)
	require.NoError(t, err)

	converted, ok := result.(*model.GeneralOpenAIRequest)
	require.True(t, ok)
	require.NotNil(t, converted.ReasoningEffort)
	assert.Equal(t, effort, *converted.ReasoningEffort)
}

// TestConvertImageRequestNormalizesResponseFormat verifies TogetherAI accepts image generation
// requests and rewrites OpenAI's b64_json request value to TogetherAI's documented base64 value.
func TestConvertImageRequestNormalizesResponseFormat(t *testing.T) {
	t.Parallel()
	adaptor := &Adaptor{}
	responseFormat := "b64_json"
	request := &model.ImageRequest{
		Model:          "google/imagen-4.0-fast",
		Prompt:         "a neon city skyline",
		ResponseFormat: &responseFormat,
	}

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	result, err := adaptor.ConvertImageRequest(c, request)
	require.NoError(t, err)

	converted, ok := result.(*model.ImageRequest)
	require.True(t, ok)
	require.NotNil(t, converted.ResponseFormat)
	assert.Equal(t, "base64", *converted.ResponseFormat)
	assert.Equal(t, request.Model, converted.Model)
	assert.Equal(t, request.Prompt, converted.Prompt)
}

// TestModelListIncludesCurrentCatalog verifies the TogetherAI adapter exposes current public
// catalog entries even when some models do not yet have published pricing metadata.
func TestModelListIncludesCurrentCatalog(t *testing.T) {
	t.Parallel()
	require.Contains(t, ModelList, "openai/gpt-oss-20b")
	require.Contains(t, ModelList, "google/flash-image-3.1")
	require.Contains(t, ModelList, "openai/whisper-large-v3")
	require.Contains(t, ModelList, "intfloat/multilingual-e5-large-instruct")
	require.NotEmpty(t, ModelRatios["google/imagen-4.0-fast"].Image)
	require.NotEmpty(t, ModelRatios["openai/whisper-large-v3"].Audio)
}
