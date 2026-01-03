package xai

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

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
			name:         "Response API passes through",
			requestPath:  "/v1/responses",
			expectedPath: "/v1/responses",
		},
		{
			name:         "Chat Completions passes through",
			requestPath:  "/v1/chat/completions",
			expectedPath: "/v1/chat/completions",
		},
		{
			name:         "Image generations passes through",
			requestPath:  "/v1/images/generations",
			expectedPath: "/v1/images/generations",
		},
		{
			name:         "Query parameters preserved",
			requestPath:  "/v1/chat/completions?stream=true",
			expectedPath: "/v1/chat/completions?stream=true",
		},
		{
			name:         "Response API with query parameters",
			requestPath:  "/v1/responses?test=123",
			expectedPath: "/v1/responses?test=123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			meta := &meta.Meta{
				BaseURL:        "https://api.x.ai",
				RequestURLPath: tt.requestPath,
			}

			url, err := adaptor.GetRequestURL(meta)
			require.NoError(t, err)
			expectedURL := "https://api.x.ai" + tt.expectedPath
			assert.Equal(t, expectedURL, url)
		})
	}
}

func TestConvertRequest(t *testing.T) {
	t.Parallel()
	adaptor := &Adaptor{}

	tests := []struct {
		name            string
		inputRequest    *model.GeneralOpenAIRequest
		expectedRequest *model.GeneralOpenAIRequest
	}{
		{
			name: "Remove reasoning_effort",
			inputRequest: &model.GeneralOpenAIRequest{
				Model:           "grok-3",
				ReasoningEffort: stringPtr("high"),
				Messages:        []model.Message{{Role: "user", Content: "hello"}},
			},
			expectedRequest: &model.GeneralOpenAIRequest{
				Model:    "grok-3",
				Messages: []model.Message{{Role: "user", Content: "hello"}},
			},
		},
		{
			name: "Remove penalty parameters for grok-4-0709",
			inputRequest: &model.GeneralOpenAIRequest{
				Model:            "grok-4-0709",
				PresencePenalty:  float64Ptr(0.5),
				FrequencyPenalty: float64Ptr(0.3),
				Messages:         []model.Message{{Role: "user", Content: "hello"}},
			},
			expectedRequest: &model.GeneralOpenAIRequest{
				Model:    "grok-4-0709",
				Messages: []model.Message{{Role: "user", Content: "hello"}},
			},
		},
		{
			name: "Remove penalty parameters for grok-4-fast-reasoning",
			inputRequest: &model.GeneralOpenAIRequest{
				Model:            "grok-4-fast-reasoning",
				PresencePenalty:  float64Ptr(0.5),
				FrequencyPenalty: float64Ptr(0.3),
				Messages:         []model.Message{{Role: "user", Content: "hello"}},
			},
			expectedRequest: &model.GeneralOpenAIRequest{
				Model:    "grok-4-fast-reasoning",
				Messages: []model.Message{{Role: "user", Content: "hello"}},
			},
		},
		{
			name: "Remove penalty parameters for grok-4-fast-non-reasoning",
			inputRequest: &model.GeneralOpenAIRequest{
				Model:            "grok-4-fast-non-reasoning",
				PresencePenalty:  float64Ptr(0.5),
				FrequencyPenalty: float64Ptr(0.3),
				Messages:         []model.Message{{Role: "user", Content: "hello"}},
			},
			expectedRequest: &model.GeneralOpenAIRequest{
				Model:    "grok-4-fast-non-reasoning",
				Messages: []model.Message{{Role: "user", Content: "hello"}},
			},
		},
		{
			name: "Keep penalty parameters for other models",
			inputRequest: &model.GeneralOpenAIRequest{
				Model:            "grok-code-fast-1",
				PresencePenalty:  float64Ptr(0.5),
				FrequencyPenalty: float64Ptr(0.3),
				Messages:         []model.Message{{Role: "user", Content: "hello"}},
			},
			expectedRequest: &model.GeneralOpenAIRequest{
				Model:            "grok-code-fast-1",
				PresencePenalty:  float64Ptr(0.5),
				FrequencyPenalty: float64Ptr(0.3),
				Messages:         []model.Message{{Role: "user", Content: "hello"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			result, err := adaptor.ConvertRequest(c, relaymode.ChatCompletions, tt.inputRequest)
			require.NoError(t, err)

			convertedReq, ok := result.(*model.GeneralOpenAIRequest)
			require.True(t, ok)

			// Compare key fields
			assert.Equal(t, tt.expectedRequest.Model, convertedReq.Model)
			assert.Equal(t, tt.expectedRequest.ReasoningEffort, convertedReq.ReasoningEffort)
			assert.Equal(t, tt.expectedRequest.PresencePenalty, convertedReq.PresencePenalty)
			assert.Equal(t, tt.expectedRequest.FrequencyPenalty, convertedReq.FrequencyPenalty)
		})
	}
}

func TestConvertImageRequest(t *testing.T) {
	t.Parallel()
	adaptor := &Adaptor{}

	tests := []struct {
		name            string
		inputRequest    *model.ImageRequest
		expectedRequest *model.ImageRequest
	}{
		{
			name: "Model name normalization",
			inputRequest: &model.ImageRequest{
				Model:  "grok-2-image",
				Prompt: "A beautiful sunset",
			},
			expectedRequest: &model.ImageRequest{
				Model:  "grok-2-image",
				Prompt: "A beautiful sunset",
			},
		},
		{
			name: "Remove unsupported parameters",
			inputRequest: &model.ImageRequest{
				Model:   "grok-2-image",
				Prompt:  "A beautiful sunset",
				Quality: "hd",
				Size:    "1024x1024",
				Style:   "vivid",
			},
			expectedRequest: &model.ImageRequest{
				Model:  "grok-2-image",
				Prompt: "A beautiful sunset",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			result, err := adaptor.ConvertImageRequest(c, tt.inputRequest)
			require.NoError(t, err)

			convertedReq, ok := result.(*model.ImageRequest)
			require.True(t, ok)

			assert.Equal(t, tt.expectedRequest.Model, convertedReq.Model)
			assert.Equal(t, tt.expectedRequest.Prompt, convertedReq.Prompt)
			assert.Equal(t, tt.expectedRequest.Quality, convertedReq.Quality)
			assert.Equal(t, tt.expectedRequest.Size, convertedReq.Size)
			assert.Equal(t, tt.expectedRequest.Style, convertedReq.Style)
		})
	}
}

func TestConvertClaudeRequest(t *testing.T) {
	t.Parallel()
	adaptor := &Adaptor{}

	t.Run("ConvertClaudeRequest delegates to openai_compatible", func(t *testing.T) {
		t.Parallel()
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		claudeRequest := &model.ClaudeRequest{
			Model: "claude-3-sonnet-20240229",
			Messages: []model.ClaudeMessage{
				{Role: "user", Content: "Hello"},
			},
		}

		result, err := adaptor.ConvertClaudeRequest(c, claudeRequest)
		// Should not error and should return a valid result
		require.NoError(t, err)
		require.NotNil(t, result)
	})
}

func TestDoResponse(t *testing.T) {
	t.Parallel()
	adaptor := &Adaptor{}

	t.Run("Image generation mode", func(t *testing.T) {
		t.Parallel()
		c, _ := gin.CreateTestContext(httptest.NewRecorder())

		// Mock image response
		imageResp := ImageResponse{
			Created: 1234567890,
			Data: []ImageData{
				{
					B64Json:       "base64data",
					URL:           "https://example.com/image.png",
					RevisedPrompt: "A revised prompt",
				},
			},
		}
		respBody, _ := json.Marshal(imageResp)

		resp := &http.Response{
			StatusCode: 200,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(string(respBody))),
		}
		resp.Header.Set("Content-Type", "application/json")

		meta := &meta.Meta{
			Mode: relaymode.ImagesGenerations,
		}

		usage, err := adaptor.DoResponse(c, resp, meta)
		assert.Nil(t, err)
		assert.Nil(t, usage) // XAI doesn't return usage for image generation
	})

	t.Run("Claude Messages mode", func(t *testing.T) {
		t.Parallel()
		c, _ := gin.CreateTestContext(httptest.NewRecorder())

		// Mock Chat Completion response
		chatResp := `{
			"id": "chatcmpl-test",
			"object": "chat.completion",
			"created": 1234567890,
			"model": "grok-3",
			"choices": [{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "Hello!"
				},
				"finish_reason": "stop"
			}],
			"usage": {
				"prompt_tokens": 5,
				"completion_tokens": 2,
				"total_tokens": 7
			}
		}`

		resp := &http.Response{
			StatusCode: 200,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(chatResp)),
		}
		resp.Header.Set("Content-Type", "application/json")

		meta := &meta.Meta{
			Mode: relaymode.ClaudeMessages,
		}

		usage, err := adaptor.DoResponse(c, resp, meta)
		assert.Nil(t, err)
		require.NotNil(t, usage)
	})
}

func TestResponseAPIUsage_ToModelUsage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		usage         *ResponseAPIUsage
		expectedUsage *model.Usage
	}{
		{
			name: "Full usage with details",
			usage: &ResponseAPIUsage{
				InputTokens:  100,
				OutputTokens: 50,
				TotalTokens:  150,
				InputTokensDetails: &ResponseAPIInputTokensDetails{
					CachedTokens: 20,
					TextTokens:   80,
				},
				OutputTokensDetails: &ResponseAPIOutputTokensDetails{
					ReasoningTokens: 30,
					TextTokens:      20,
				},
			},
			expectedUsage: &model.Usage{
				PromptTokens:     100,
				CompletionTokens: 50,
				TotalTokens:      150,
				PromptTokensDetails: &model.UsagePromptTokensDetails{
					CachedTokens: 20,
					TextTokens:   80,
				},
				CompletionTokensDetails: &model.UsageCompletionTokensDetails{
					ReasoningTokens: 30,
					TextTokens:      20,
				},
			},
		},
		{
			name: "Minimal usage without details",
			usage: &ResponseAPIUsage{
				InputTokens:  10,
				OutputTokens: 5,
				TotalTokens:  15,
			},
			expectedUsage: &model.Usage{
				PromptTokens:     10,
				CompletionTokens: 5,
				TotalTokens:      15,
			},
		},
		{
			name:          "Nil usage",
			usage:         nil,
			expectedUsage: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := tt.usage.ToModelUsage()
			if tt.expectedUsage == nil {
				assert.Nil(t, result)
				return
			}

			require.NotNil(t, result)
			assert.Equal(t, tt.expectedUsage.PromptTokens, result.PromptTokens)
			assert.Equal(t, tt.expectedUsage.CompletionTokens, result.CompletionTokens)
			assert.Equal(t, tt.expectedUsage.TotalTokens, result.TotalTokens)

			if tt.expectedUsage.PromptTokensDetails != nil {
				require.NotNil(t, result.PromptTokensDetails)
				assert.Equal(t, tt.expectedUsage.PromptTokensDetails.CachedTokens, result.PromptTokensDetails.CachedTokens)
				assert.Equal(t, tt.expectedUsage.PromptTokensDetails.TextTokens, result.PromptTokensDetails.TextTokens)
			}

			if tt.expectedUsage.CompletionTokensDetails != nil {
				require.NotNil(t, result.CompletionTokensDetails)
				assert.Equal(t, tt.expectedUsage.CompletionTokensDetails.ReasoningTokens, result.CompletionTokensDetails.ReasoningTokens)
				assert.Equal(t, tt.expectedUsage.CompletionTokensDetails.TextTokens, result.CompletionTokensDetails.TextTokens)
			}
		})
	}
}

func TestHandleResponseAPIResponse(t *testing.T) {
	t.Parallel()
	adaptor := &Adaptor{}

	t.Run("Streaming Response API", func(t *testing.T) {
		t.Parallel()
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		meta := &meta.Meta{IsStream: true}

		resp := &http.Response{
			StatusCode: 200,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("streaming data")),
		}

		usage, err := adaptor.handleResponseAPIResponse(c, resp, meta)
		assert.Nil(t, err)
		assert.Nil(t, usage) // Streaming doesn't return usage
	})
}

func TestGetChannelName(t *testing.T) {
	t.Parallel()
	adaptor := &Adaptor{}
	assert.Equal(t, "xai", adaptor.GetChannelName())
}

func TestGetModelList(t *testing.T) {
	t.Parallel()
	adaptor := &Adaptor{}
	models := adaptor.GetModelList()
	assert.NotEmpty(t, models)
	// Should include grok models from ModelRatios
	assert.Contains(t, models, "grok-code-fast-1")
}

func TestGetDefaultModelPricing(t *testing.T) {
	t.Parallel()
	adaptor := &Adaptor{}
	pricing := adaptor.GetDefaultModelPricing()
	assert.NotNil(t, pricing)
	assert.NotEmpty(t, pricing)
}

func TestGetModelRatio(t *testing.T) {
	t.Parallel()
	adaptor := &Adaptor{}

	// Test known model
	ratio := adaptor.GetModelRatio("grok-3")
	assert.Greater(t, ratio, 0.0)

	// Test unknown model (should fall back to default)
	ratio = adaptor.GetModelRatio("unknown-model")
	assert.Greater(t, ratio, 0.0)
}

func TestGetCompletionRatio(t *testing.T) {
	t.Parallel()
	adaptor := &Adaptor{}

	// Test known model
	ratio := adaptor.GetCompletionRatio("grok-3")
	assert.Greater(t, ratio, 0.0)

	// Test unknown model (should fall back to default)
	ratio = adaptor.GetCompletionRatio("unknown-model")
	assert.Greater(t, ratio, 0.0)
}

func TestResponseAPIInputTokensDetails_UnmarshalJSON(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		jsonInput   string
		expected    *ResponseAPIInputTokensDetails
		expectError bool
	}{
		{
			name:      "Full structure with all fields",
			jsonInput: `{"cached_tokens": 100, "audio_tokens": 50, "text_tokens": 200, "image_tokens": 25, "web_search": {"query": "test"}, "custom_field": "value"}`,
			expected: &ResponseAPIInputTokensDetails{
				CachedTokens: 100,
				AudioTokens:  50,
				TextTokens:   200,
				ImageTokens:  25,
				WebSearch:    map[string]any{"query": "test"},
			},
			expectError: false,
		},
		{
			name:      "Partial fields",
			jsonInput: `{"cached_tokens": 10, "text_tokens": 20}`,
			expected: &ResponseAPIInputTokensDetails{
				CachedTokens: 10,
				TextTokens:   20,
			},
			expectError: false,
		},
		{
			name:      "Empty object",
			jsonInput: `{}`,
			expected: &ResponseAPIInputTokensDetails{
				CachedTokens: 0,
				AudioTokens:  0,
				TextTokens:   0,
				ImageTokens:  0,
			},
			expectError: false,
		},
		{
			name:      "Float values (should be converted to int)",
			jsonInput: `{"cached_tokens": 10.5, "audio_tokens": 5.7}`,
			expected: &ResponseAPIInputTokensDetails{
				CachedTokens: 10,
				AudioTokens:  5,
			},
			expectError: false,
		},
		{
			name:        "Invalid JSON structure",
			jsonInput:   `{"cached_tokens": }`, // Invalid JSON syntax
			expected:    nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var result ResponseAPIInputTokensDetails
			err := json.Unmarshal([]byte(tt.jsonInput), &result)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, tt.expected)

			assert.Equal(t, tt.expected.CachedTokens, result.CachedTokens)
			assert.Equal(t, tt.expected.AudioTokens, result.AudioTokens)
			assert.Equal(t, tt.expected.TextTokens, result.TextTokens)
			assert.Equal(t, tt.expected.ImageTokens, result.ImageTokens)
			assert.Equal(t, tt.expected.WebSearch, result.WebSearch)

			// Check that additional fields are stored if present
			if strings.Contains(tt.jsonInput, "custom_field") {
				assert.NotNil(t, result.additional)
				assert.Equal(t, "value", result.additional["custom_field"])
			}
		})
	}
}

func TestResponseAPIOutputTokensDetails_UnmarshalJSON(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		jsonInput   string
		expected    *ResponseAPIOutputTokensDetails
		expectError bool
	}{
		{
			name:      "Full structure with all fields",
			jsonInput: `{"reasoning_tokens": 100, "audio_tokens": 50, "accepted_prediction_tokens": 20, "rejected_prediction_tokens": 5, "text_tokens": 200, "cached_tokens": 25, "custom_field": "value"}`,
			expected: &ResponseAPIOutputTokensDetails{
				ReasoningTokens:          100,
				AudioTokens:              50,
				AcceptedPredictionTokens: 20,
				RejectedPredictionTokens: 5,
				TextTokens:               200,
				CachedTokens:             25,
			},
			expectError: false,
		},
		{
			name:      "Partial fields",
			jsonInput: `{"reasoning_tokens": 10, "text_tokens": 20}`,
			expected: &ResponseAPIOutputTokensDetails{
				ReasoningTokens: 10,
				TextTokens:      20,
			},
			expectError: false,
		},
		{
			name:      "Empty object",
			jsonInput: `{}`,
			expected: &ResponseAPIOutputTokensDetails{
				ReasoningTokens:          0,
				AudioTokens:              0,
				AcceptedPredictionTokens: 0,
				RejectedPredictionTokens: 0,
				TextTokens:               0,
				CachedTokens:             0,
			},
			expectError: false,
		},
		{
			name:      "Float values (should be converted to int)",
			jsonInput: `{"reasoning_tokens": 10.5, "audio_tokens": 5.7, "accepted_prediction_tokens": 2.3}`,
			expected: &ResponseAPIOutputTokensDetails{
				ReasoningTokens:          10,
				AudioTokens:              5,
				AcceptedPredictionTokens: 2,
			},
			expectError: false,
		},
		{
			name:        "Invalid JSON structure",
			jsonInput:   `{"reasoning_tokens": }`, // Invalid JSON syntax
			expected:    nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var result ResponseAPIOutputTokensDetails
			err := json.Unmarshal([]byte(tt.jsonInput), &result)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, tt.expected)

			assert.Equal(t, tt.expected.ReasoningTokens, result.ReasoningTokens)
			assert.Equal(t, tt.expected.AudioTokens, result.AudioTokens)
			assert.Equal(t, tt.expected.AcceptedPredictionTokens, result.AcceptedPredictionTokens)
			assert.Equal(t, tt.expected.RejectedPredictionTokens, result.RejectedPredictionTokens)
			assert.Equal(t, tt.expected.TextTokens, result.TextTokens)
			assert.Equal(t, tt.expected.CachedTokens, result.CachedTokens)

			// Check that additional fields are stored if present
			if strings.Contains(tt.jsonInput, "custom_field") {
				assert.NotNil(t, result.additional)
				assert.Equal(t, "value", result.additional["custom_field"])
			}
		})
	}
}

// Helper functions
func stringPtr(s string) *string {
	return &s
}

func float64Ptr(f float64) *float64 {
	return &f
}
