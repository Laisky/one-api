package openai

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

func TestChannelSpecificConversion(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a sample ChatCompletion request
	chatRequest := &model.GeneralOpenAIRequest{
		Model: "gpt-4",
		Messages: []model.Message{
			{Role: "user", Content: "Hello, world!"},
		},
		Stream: false,
	}

	// Test cases
	testCases := []struct {
		channelType      int
		expectConversion bool
		name             string
	}{
		{channeltype.OpenAI, true, "OpenAI"},
		{channeltype.Azure, false, "Azure"},
		{channeltype.AI360, false, "AI360"},
		{channeltype.Moonshot, false, "Moonshot"},
		{channeltype.Groq, false, "Groq"},
		{channeltype.DeepSeek, false, "DeepSeek"},
		{channeltype.OpenRouter, false, "OpenRouter"},
		{channeltype.OpenAICompatible, false, "OpenAI Compatible"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create Gin context
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = &http.Request{}

			// Create meta context with the channel type
			testMeta := &meta.Meta{
				Mode:           relaymode.ChatCompletions,
				ChannelType:    tc.channelType,
				RequestURLPath: "/v1/chat/completions",
				BaseURL:        "https://api.openai.com",
			}
			testMeta.ActualModelName = chatRequest.Model
			// Azure requires a deployment/model name in the URL; set a dummy one for the test
			if tc.channelType == channeltype.Azure {
				testMeta.ActualModelName = "gpt-4o-mini"
			}
			c.Set(ctxkey.Meta, testMeta)

			// Create adaptor
			adaptor := &Adaptor{}
			adaptor.Init(testMeta)

			// Test URL generation
			url, err := adaptor.GetRequestURL(testMeta)
			require.NoError(t, err, "GetRequestURL failed")

			// Check if URL was converted to /responses
			urlConverted := (url == "https://api.openai.com/v1/responses")

			// Test request conversion
			convertedReq, err := adaptor.ConvertRequest(c, relaymode.ChatCompletions, chatRequest)
			require.NoError(t, err, "ConvertRequest failed")

			// Check if request was converted to ResponseAPIRequest
			_, isResponseAPI := convertedReq.(*ResponseAPIRequest)

			// Verify expectations
			if tc.expectConversion {
				require.True(t, urlConverted, "Expected URL conversion for %s but got: %s", tc.name, url)
				require.True(t, isResponseAPI, "Expected request conversion for %s but request was not converted", tc.name)
				t.Logf("✓ %s: Converted to Response API", tc.name)
			} else {
				require.False(t, urlConverted, "Did not expect URL conversion for %s but got: %s", tc.name, url)
				require.False(t, isResponseAPI, "Did not expect request conversion for %s but request was converted", tc.name)
				t.Logf("✓ %s: Kept as native ChatCompletion payload", tc.name)
			}
		})
	}
}

func TestModelSpecificConversion(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Test different models with OpenAI channel type
	testCases := []struct {
		model            string
		expectConversion bool
		name             string
	}{
		{"gpt-4", true, "GPT-4 should convert"},
		{"gpt-4o", true, "GPT-4o should convert"},
		{"gpt-3.5-turbo", true, "GPT-3.5-turbo should convert"},
		{"o1-preview", true, "o1-preview should convert"},
		{"gpt-4-search-2024-12-20", false, "Search model should stay ChatCompletion"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a sample ChatCompletion request with specific model
			chatRequest := &model.GeneralOpenAIRequest{
				Model: tc.model,
				Messages: []model.Message{
					{Role: "user", Content: "Hello, world!"},
				},
				Stream: false,
			}

			// Create Gin context
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = &http.Request{}

			// Create meta context with OpenAI channel type
			testMeta := &meta.Meta{
				Mode:           relaymode.ChatCompletions,
				ChannelType:    channeltype.OpenAI, // Always OpenAI for this test
				RequestURLPath: "/v1/chat/completions",
				BaseURL:        "https://api.openai.com",
			}
			testMeta.ActualModelName = tc.model
			c.Set(ctxkey.Meta, testMeta)

			// Create adaptor
			adaptor := &Adaptor{}
			adaptor.Init(testMeta)

			// Test request conversion
			convertedReq, err := adaptor.ConvertRequest(c, relaymode.ChatCompletions, chatRequest)
			require.NoError(t, err, "ConvertRequest failed")

			_, isResponseAPI := convertedReq.(*ResponseAPIRequest)

			if tc.expectConversion {
				require.True(t, isResponseAPI, "Expected request conversion for model %s but request was not converted", tc.model)
				t.Logf("✓ Model %s: Converted to Response API", tc.model)
			} else {
				require.False(t, isResponseAPI, "Did not expect request conversion for model %s but request was converted", tc.model)
				t.Logf("✓ Model %s: Kept as ChatCompletion payload", tc.model)
			}
		})
	}
}

func TestAzureGPT5ConversionToResponseAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)

	chatRequest := &model.GeneralOpenAIRequest{
		Model: "gpt-5-mini",
		Messages: []model.Message{
			{Role: "user", Content: "Hi"},
		},
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = &http.Request{}

	azureMeta := &meta.Meta{
		Mode:            relaymode.ChatCompletions,
		ChannelType:     channeltype.Azure,
		BaseURL:         "https://example.azure.com",
		RequestURLPath:  "/v1/chat/completions",
		ActualModelName: chatRequest.Model,
	}
	c.Set(ctxkey.Meta, azureMeta)

	adaptor := &Adaptor{}
	adaptor.Init(azureMeta)

	converted, err := adaptor.ConvertRequest(c, relaymode.ChatCompletions, chatRequest)
	require.NoError(t, err)
	require.IsType(t, &ResponseAPIRequest{}, converted)

	url, urlErr := adaptor.GetRequestURL(azureMeta)
	require.NoError(t, urlErr)
	require.Contains(t, url, "/openai/v1/responses?api-version=v1")

	stored, ok := c.Get(ctxkey.ConvertedRequest)
	require.True(t, ok)
	require.IsType(t, &ResponseAPIRequest{}, stored)
}

func TestConvertRequest_ResponseAPIModeBypassesConversionSanitization(t *testing.T) {
	gin.SetMode(gin.TestMode)

	req := &model.GeneralOpenAIRequest{
		Model: "gpt-4.1-nano",
		Messages: []model.Message{
			{
				Role: "user",
				Content: []any{
					map[string]any{
						"type": "input_text",
						"text": "hello",
						"cache_control": map[string]any{
							"type": "ephemeral",
						},
					},
				},
			},
		},
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = &http.Request{}

	metaInfo := &meta.Meta{
		Mode:            relaymode.ResponseAPI,
		ChannelType:     channeltype.OpenAI,
		BaseURL:         "https://api.openai.com",
		RequestURLPath:  "/v1/responses",
		ActualModelName: req.Model,
	}
	c.Set(ctxkey.Meta, metaInfo)

	adaptor := &Adaptor{}
	converted, err := adaptor.ConvertRequest(c, relaymode.ResponseAPI, req)
	require.NoError(t, err)
	require.Same(t, req, converted, "Response API mode should pass through request object")

	content, ok := req.Messages[0].Content.([]any)
	require.True(t, ok)
	firstItem, ok := content[0].(map[string]any)
	require.True(t, ok)
	_, hasCacheControl := firstItem["cache_control"]
	require.True(t, hasCacheControl, "cache_control must remain untouched in direct Response API mode")
}
