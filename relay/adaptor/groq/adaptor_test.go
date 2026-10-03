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

// TestGetModelListMatchesCurrentCatalog checks discovery and retained legacy metadata using t and returns no value.
func TestGetModelListMatchesCurrentCatalog(t *testing.T) {
	t.Parallel()

	want := []string{
		"llama-3.1-8b-instant",
		"llama-3.3-70b-versatile",
		"openai/gpt-oss-120b",
		"openai/gpt-oss-20b",
		"whisper-large-v3",
		"whisper-large-v3-turbo",
		"canopylabs/orpheus-arabic-saudi",
		"canopylabs/orpheus-v1-english",
		"meta-llama/llama-prompt-guard-2-22m",
		"meta-llama/llama-prompt-guard-2-86m",
		"minimaxai/minimax-m2.7",
		"openai/gpt-oss-safeguard-20b",
		"qwen/qwen3.8-27b",
	}

	models := (&Adaptor{}).GetModelList()
	require.ElementsMatch(t, want, models)
	require.Len(t, models, len(want))
	for _, modelID := range models {
		require.Contains(t, (&Adaptor{}).GetDefaultModelPricing(), modelID)
	}

	// Superseded enterprise IDs and decommissioned systems keep compatibility
	// metadata without appearing in the current upstream catalog.
	for _, retired := range []string{
		"groq/compound",
		"groq/compound-mini",
		"qwen/qwen3.6-27b",
		"meta-llama/llama-4-scout-17b-16e-instruct",
		"qwen/qwen3-32b",
	} {
		require.NotContains(t, models, retired)
		require.Contains(t, ModelRatios, retired)
	}
}

func TestConvertRequest_DropsReasoningFields(t *testing.T) {
	t.Parallel()

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

	for _, effort := range []string{"none", "default"} {
		require.True(t, groqReasoningEffortAllowed("qwen/qwen3.6-27b", effort), "Qwen 3.6 should accept %q", effort)
	}
	for _, effort := range []string{"low", "medium", "high"} {
		require.False(t, groqReasoningEffortAllowed("qwen/qwen3.6-27b", effort), "Qwen 3.6 should reject %q", effort)
	}

	require.False(t, groqReasoningEffortAllowed("openai/gpt-oss-120b", "none"))
	require.True(t, groqReasoningEffortAllowed("openai/gpt-oss-120b", "high"))
	require.True(t, groqReasoningEffortAllowed("openai/gpt-oss-safeguard-20b", "low"))
	require.False(t, groqReasoningEffortAllowed("minimaxai/minimax-m2.7", "high"))
	require.False(t, groqReasoningEffortAllowed("unknown-model", "minimal"))
}

// TestCurrentGroqModelMetadata checks current capabilities and legacy system metadata using t and returns no value.
func TestCurrentGroqModelMetadata(t *testing.T) {
	t.Parallel()

	qwen, ok := ModelRatios["qwen/qwen3.8-27b"]
	require.True(t, ok)
	require.EqualValues(t, 16_384, qwen.MaxOutputTokens)
	require.EqualValues(t, 131_072, qwen.ContextLength)
	require.Equal(t, []string{"none", "default", "low", "medium", "high"}, qwen.SupportedReasoningEfforts)
	require.Equal(t, "none", qwen.DefaultReasoningEffort)
	require.Equal(t, []string{"text", "image"}, qwen.InputModalities)
	require.Equal(t, []string{"text"}, qwen.OutputModalities)
	require.Equal(t, "Qwen/Qwen3.8-27B", qwen.HuggingFaceID)
	require.Contains(t, qwen.SupportedFeatures, "structured_outputs")
	require.NotContains(t, qwen.SupportedFeatures, "web_search")
	require.Zero(t, qwen.CachedInputRatio)

	minimax, ok := ModelRatios["minimaxai/minimax-m2.7"]
	require.True(t, ok)
	require.EqualValues(t, 196_608, minimax.ContextLength)
	require.EqualValues(t, 131_072, minimax.MaxOutputTokens)
	require.Zero(t, minimax.Ratio, "contact-sales models must not use a guessed token price")
	require.Empty(t, minimax.SupportedReasoningEfforts)
	require.Equal(t, "MiniMaxAI/MiniMax-M2.7", minimax.HuggingFaceID)
	require.NotContains(t, minimax.SupportedFeatures, "structured_outputs")

	compound, ok := ModelRatios["groq/compound"]
	require.True(t, ok)
	require.Zero(t, compound.Ratio, "Compound has no standalone token tariff")
	require.NotContains(t, compound.SupportedFeatures, "reasoning")
	require.Contains(t, compound.Description, "DECOMMISSIONED on 2026-09-21")
}

func TestConvertRequest_RejectsMultimodalForGPTOSS(t *testing.T) {
	t.Parallel()

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
