package openai

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

func TestGetRequestURLWithModelSupport(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adaptor := &Adaptor{}

	testCases := []struct {
		name        string
		modelName   string
		expectedURL string
	}{
		{
			name:        "GPT-4 should use Response API",
			modelName:   "gpt-4",
			expectedURL: "https://api.openai.com/v1/responses",
		},
		{
			name:        "GPT-4 Search should use ChatCompletion API",
			modelName:   "gpt-4-search-2024-12-20",
			expectedURL: "https://api.openai.com/v1/chat/completions",
		},
		{
			name:        "GPT-4o Search should use ChatCompletion API",
			modelName:   "gpt-4o-search-2024-12-20",
			expectedURL: "https://api.openai.com/v1/chat/completions",
		},
		{
			name:        "Regular GPT-4o should use Response API",
			modelName:   "gpt-4o",
			expectedURL: "https://api.openai.com/v1/responses",
		},
		{
			name:        "GPT-5-mini should use Response API",
			modelName:   "gpt-5-mini",
			expectedURL: "https://api.openai.com/v1/responses",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup gin context
			c, _ := gin.CreateTestContext(nil)

			// Setup meta with ChatCompletion mode and OpenAI channel
			relayMeta := &meta.Meta{
				Mode:            relaymode.ChatCompletions,
				ChannelType:     channeltype.OpenAI,
				BaseURL:         "https://api.openai.com",
				RequestURLPath:  "/v1/chat/completions", // Original ChatCompletion path
				ActualModelName: tc.modelName,
			}

			// Store meta in context
			meta.Set2Context(c, relayMeta)

			// Get the request URL
			requestURL, err := adaptor.GetRequestURL(relayMeta)
			require.NoError(t, err, "GetRequestURL failed")

			// Verify the URL matches expectation
			require.Equal(t, tc.expectedURL, requestURL)

			onlyChatCompletion := IsModelsOnlySupportedByChatCompletionAPI(tc.modelName)
			t.Logf("✓ %s: URL=%s, OnlyChatCompletion=%v", tc.name, requestURL, onlyChatCompletion)
		})
	}
}

func TestModelSupportConsistencyBetweenURLAndConversion(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adaptor := &Adaptor{}

	testModels := []string{
		"gpt-4",
		"gpt-4o",
		"gpt-4-search-2024-12-20",
		"gpt-4o-search-2024-12-20",
		"gpt-3.5-turbo-search-enhanced",
		"gpt-3.5-turbo",
		"o1-preview",
	}

	for _, modelName := range testModels {
		t.Run(modelName, func(t *testing.T) {
			// Setup gin context
			c, _ := gin.CreateTestContext(nil)

			// Setup meta
			relayMeta := &meta.Meta{
				Mode:            relaymode.ChatCompletions,
				ChannelType:     channeltype.OpenAI,
				BaseURL:         "https://api.openai.com",
				RequestURLPath:  "/v1/chat/completions",
				ActualModelName: modelName,
			}
			meta.Set2Context(c, relayMeta)

			// Get URL decision
			requestURL, err := adaptor.GetRequestURL(relayMeta)
			require.NoError(t, err, "GetRequestURL failed")

			// Get conversion decision
			onlyChatCompletion := IsModelsOnlySupportedByChatCompletionAPI(modelName)

			expectedURL := "https://api.openai.com/v1/responses"
			if onlyChatCompletion {
				expectedURL = "https://api.openai.com/v1/chat/completions"
			}

			// Check consistency
			require.Equal(t, expectedURL, requestURL)

			t.Logf("✓ Model %s: OnlyChat=%v, URL=%s", modelName, onlyChatCompletion, requestURL)
		})
	}
}

func TestGetRequestURLWithClaudeMessages(t *testing.T) {
	testCases := []struct {
		name          string
		modelName     string
		expectedPath  string
		expectedURL   string
		shouldConvert bool
	}{
		{
			name:          "GPT-5-mini with Claude Messages should use Response API",
			modelName:     "gpt-5-mini",
			expectedPath:  "/v1/responses",
			expectedURL:   "https://api.openai.com/v1/responses",
			shouldConvert: true,
		},
		{
			name:          "GPT-4 with Claude Messages should use Response API",
			modelName:     "gpt-4",
			expectedPath:  "/v1/responses",
			expectedURL:   "https://api.openai.com/v1/responses",
			shouldConvert: true,
		},
		{
			name:          "GPT-4-search with Claude Messages should use ChatCompletion API",
			modelName:     "gpt-4-search-2024-12-20",
			expectedPath:  "/v1/chat/completions",
			expectedURL:   "https://api.openai.com/v1/chat/completions",
			shouldConvert: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup gin context
			c, _ := gin.CreateTestContext(nil)

			// Setup meta with Claude Messages mode and OpenAI channel
			relayMeta := &meta.Meta{
				Mode:            relaymode.ClaudeMessages,
				ChannelType:     channeltype.OpenAI,
				BaseURL:         "https://api.openai.com",
				RequestURLPath:  "/v1/messages", // Original Claude Messages path
				ActualModelName: tc.modelName,
			}

			// Store meta in context
			meta.Set2Context(c, relayMeta)

			// Create adaptor instance
			adaptor := &Adaptor{ChannelType: channeltype.OpenAI}
			adaptor.Init(relayMeta)

			// Get request URL
			requestURL, err := adaptor.GetRequestURL(relayMeta)
			require.NoError(t, err, "GetRequestURL failed")

			// Verify the URL matches expectation
			require.Equal(t, tc.expectedURL, requestURL)

			// Verify model support detection
			onlyChatCompletion := IsModelsOnlySupportedByChatCompletionAPI(tc.modelName)
			if tc.shouldConvert {
				require.False(t, onlyChatCompletion, "Model %s should support Response API but IsModelsOnlySupportedByChatCompletionAPI returned true", tc.modelName)
			} else {
				require.True(t, onlyChatCompletion, "Model %s should only support ChatCompletion API but IsModelsOnlySupportedByChatCompletionAPI returned false", tc.modelName)
			}

			t.Logf("✓ %s: URL=%s, OnlyChatCompletion=%v", tc.name, requestURL, onlyChatCompletion)
		})
	}
}
