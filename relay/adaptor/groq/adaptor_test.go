package groq

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

func TestGetRequestURL(t *testing.T) {
	t.Parallel()
	adaptor := &Adaptor{}

	testCases := []struct {
		name           string
		requestURLPath string
		expectedURL    string
		baseURL        string
		channelType    int
	}{
		{
			name:           "Claude Messages API with query conversion",
			requestURLPath: "/v1/messages?beta=true",
			expectedURL:    "https://api.groq.com/v1/chat/completions",
			baseURL:        "https://api.groq.com",
			channelType:    channeltype.Groq,
		},
		{
			name:           "Claude Messages API conversion",
			requestURLPath: "/v1/messages",
			expectedURL:    "https://api.groq.com/v1/chat/completions",
			baseURL:        "https://api.groq.com",
			channelType:    channeltype.Groq,
		},
		{
			name:           "OpenAI Chat Completions passthrough",
			requestURLPath: "/v1/chat/completions",
			expectedURL:    "https://api.groq.com/v1/chat/completions",
			baseURL:        "https://api.groq.com",
			channelType:    channeltype.Groq,
		},
		{
			name:           "Other endpoints passthrough",
			requestURLPath: "/v1/models",
			expectedURL:    "https://api.groq.com/v1/models",
			baseURL:        "https://api.groq.com",
			channelType:    channeltype.Groq,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			meta := &meta.Meta{
				RequestURLPath: tc.requestURLPath,
				BaseURL:        tc.baseURL,
				ChannelType:    tc.channelType,
			}

			url, err := adaptor.GetRequestURL(meta)
			require.NoError(t, err, "GetRequestURL failed")
			require.Equal(t, tc.expectedURL, url)
		})
	}
}

func TestGetModelListIncludesCurrentPreviewModel(t *testing.T) {
	t.Parallel()

	models := (&Adaptor{}).GetModelList()
	require.Contains(t, models, "minimaxai/minimax-m2.7")
	require.Contains(t, models, "qwen/qwen3.6-27b")
	// Retired IDs remain available for existing channels and billing continuity.
	require.Contains(t, models, "qwen/qwen3-32b")
}

func TestConvertRequest_DropsReasoningFields(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	adaptor := &Adaptor{}
	effort := "high"
	req := &model.GeneralOpenAIRequest{
		Model:     "openai/gpt-oss-120b",
		Reasoning: &model.OpenAIResponseReasoning{Effort: &effort},
	}

	convertedAny, err := adaptor.ConvertRequest(c, 0, req)
	require.NoError(t, err)

	converted, ok := convertedAny.(*model.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Nil(t, converted.Reasoning)
	require.NotNil(t, converted.ReasoningEffort)
	require.Equal(t, effort, *converted.ReasoningEffort)

	jsonBytes, err := json.Marshal(converted)
	require.NoError(t, err)
	require.NotContains(t, string(jsonBytes), `"reasoning"`)
	require.Contains(t, string(jsonBytes), `"reasoning_effort"`)
}

func TestGroqReasoningEffortAllowedIsModelSpecific(t *testing.T) {
	t.Parallel()

	for _, effort := range []string{"none", "default", "low", "medium", "high"} {
		require.True(t, groqReasoningEffortAllowed("qwen/qwen3.6-27b", effort), "Qwen 3.6 should accept %q", effort)
	}
	require.False(t, groqReasoningEffortAllowed("openai/gpt-oss-120b", "none"))
	require.True(t, groqReasoningEffortAllowed("openai/gpt-oss-120b", "high"))
	require.False(t, groqReasoningEffortAllowed("unknown-model", "minimal"))
}

func TestCurrentGroqModelMetadata(t *testing.T) {
	t.Parallel()

	qwen, ok := ModelRatios["qwen/qwen3.6-27b"]
	require.True(t, ok)
	require.EqualValues(t, 16_384, qwen.MaxOutputTokens)
	require.EqualValues(t, 131_072, qwen.ContextLength)

	minimax, ok := ModelRatios["minimaxai/minimax-m2.7"]
	require.True(t, ok)
	require.EqualValues(t, 196_608, minimax.ContextLength)
	require.EqualValues(t, 131_072, minimax.MaxOutputTokens)
	require.Zero(t, minimax.Ratio, "contact-sales models must not use a guessed token price")

	compound, ok := ModelRatios["groq/compound"]
	require.True(t, ok)
	require.Zero(t, compound.Ratio, "Compound has no standalone token tariff")
	require.NotContains(t, compound.SupportedFeatures, "reasoning")
}

func TestConvertRequest_RejectsMultimodalForGPTOSS(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	adaptor := &Adaptor{}
	req := &model.GeneralOpenAIRequest{
		Model: "openai/gpt-oss-120b",
		Messages: []model.Message{
			{Role: "system", Content: "You are helpful"},
			{
				Role: "user",
				Content: []model.MessageContent{
					{Type: model.ContentTypeText, Text: strPtr("what is in this image?")},
					{Type: model.ContentTypeImageURL, ImageURL: &model.ImageURL{Url: "https://example.com/a.png"}},
				},
			},
		},
	}

	convertedAny, err := adaptor.ConvertRequest(c, 0, req)
	require.Error(t, err)
	require.Nil(t, convertedAny)
	require.Contains(t, err.Error(), "validation failed")
	require.Contains(t, err.Error(), "openai/gpt-oss-120b")
	require.Contains(t, err.Error(), "image_url")
}

func TestConvertRequest_AllowsMultimodalForLlama4(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	adaptor := &Adaptor{}
	req := &model.GeneralOpenAIRequest{
		Model: "meta-llama/llama-4-scout-17b-16e-instruct",
		Messages: []model.Message{
			{
				Role: "user",
				Content: []any{
					map[string]any{"type": "input_text", "text": "describe this image"},
					map[string]any{
						"type": "input_image",
						"image_url": map[string]any{
							"url": "https://example.com/a.png",
						},
					},
				},
			},
		},
	}

	convertedAny, err := adaptor.ConvertRequest(c, 0, req)
	require.NoError(t, err)
	converted, ok := convertedAny.(*model.GeneralOpenAIRequest)
	require.True(t, ok)
	require.NotNil(t, converted)
	require.Len(t, converted.Messages, 1)
}

func strPtr(v string) *string {
	return &v
}
