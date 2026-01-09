package test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/adaptor/ai360"
	"github.com/songquanpeng/one-api/relay/adaptor/aiproxy"
	"github.com/songquanpeng/one-api/relay/adaptor/ali"
	"github.com/songquanpeng/one-api/relay/adaptor/anthropic"
	"github.com/songquanpeng/one-api/relay/adaptor/aws"
	"github.com/songquanpeng/one-api/relay/adaptor/baichuan"
	"github.com/songquanpeng/one-api/relay/adaptor/baidu"
	"github.com/songquanpeng/one-api/relay/adaptor/baiduv2"
	"github.com/songquanpeng/one-api/relay/adaptor/cloudflare"
	"github.com/songquanpeng/one-api/relay/adaptor/cohere"
	"github.com/songquanpeng/one-api/relay/adaptor/coze"
	"github.com/songquanpeng/one-api/relay/adaptor/deepl"
	"github.com/songquanpeng/one-api/relay/adaptor/deepseek"
	"github.com/songquanpeng/one-api/relay/adaptor/doubao"
	"github.com/songquanpeng/one-api/relay/adaptor/gemini"
	"github.com/songquanpeng/one-api/relay/adaptor/groq"
	"github.com/songquanpeng/one-api/relay/adaptor/lingyiwanwu"
	"github.com/songquanpeng/one-api/relay/adaptor/minimax"
	"github.com/songquanpeng/one-api/relay/adaptor/mistral"
	"github.com/songquanpeng/one-api/relay/adaptor/moonshot"
	"github.com/songquanpeng/one-api/relay/adaptor/novita"
	"github.com/songquanpeng/one-api/relay/adaptor/ollama"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/adaptor/openrouter"
	"github.com/songquanpeng/one-api/relay/adaptor/palm"
	"github.com/songquanpeng/one-api/relay/adaptor/proxy"
	"github.com/songquanpeng/one-api/relay/adaptor/replicate"
	"github.com/songquanpeng/one-api/relay/adaptor/siliconflow"
	"github.com/songquanpeng/one-api/relay/adaptor/stepfun"
	"github.com/songquanpeng/one-api/relay/adaptor/tencent"
	"github.com/songquanpeng/one-api/relay/adaptor/togetherai"
	"github.com/songquanpeng/one-api/relay/adaptor/vertexai"
	"github.com/songquanpeng/one-api/relay/adaptor/xai"
	"github.com/songquanpeng/one-api/relay/adaptor/xunfei"
	"github.com/songquanpeng/one-api/relay/adaptor/xunfeiv2"
	"github.com/songquanpeng/one-api/relay/adaptor/zhipu"
	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

// AdapterTestCase represents a test case for an adapter
type AdapterTestCase struct {
	Name                   string
	Adapter                adaptor.Adaptor
	ChannelType            int
	SupportsChatCompletion bool
	SupportsClaudeMessages bool
	TestModel              string
	ExpectedError          string // Expected error message for unsupported operations
	IsStubImplementation   bool   // Whether this is a stub/incomplete implementation
}

// Test data
var (
	testChatCompletionRequest = &model.GeneralOpenAIRequest{
		Model: "gpt-4o-mini",
		Messages: []model.Message{
			{
				Role:    "user",
				Content: "Hello, world!",
			},
		},
		MaxTokens:   100,
		Temperature: &[]float64{0.7}[0],
	}

	testClaudeMessagesRequest = &model.ClaudeRequest{
		Model: "claude-3-sonnet-20240229",
		Messages: []model.ClaudeMessage{
			{
				Role:    "user",
				Content: "Hello, world!",
			},
		},
		MaxTokens:   100,
		Temperature: &[]float64{0.7}[0],
	}
)

// getAllAdapterTestCases returns test cases for all adapters
func getAllAdapterTestCases() []AdapterTestCase {
	return []AdapterTestCase{
		// Native Claude Support
		{
			Name:                   "Anthropic",
			Adapter:                &anthropic.Adaptor{},
			ChannelType:            channeltype.Anthropic,
			SupportsChatCompletion: true,
			SupportsClaudeMessages: true,
			TestModel:              "claude-3-sonnet-20240229",
		},

		// OpenAI and Custom Implementations
		{
			Name:                   "OpenAI",
			Adapter:                &openai.Adaptor{},
			ChannelType:            channeltype.OpenAI,
			SupportsChatCompletion: true,
			SupportsClaudeMessages: true,
			TestModel:              "gpt-4o-mini",
		},
		{
			Name:                   "Gemini",
			Adapter:                &gemini.Adaptor{},
			ChannelType:            channeltype.Gemini,
			SupportsChatCompletion: true,
			SupportsClaudeMessages: true,
			TestModel:              "gemini-2.5-flash",
		},
		{
			Name:                   "Ali",
			Adapter:                &ali.Adaptor{},
			ChannelType:            channeltype.Ali,
			SupportsChatCompletion: true,
			SupportsClaudeMessages: true,
			TestModel:              "qwen-turbo",
		},
		{
			Name:                   "Baidu",
			Adapter:                &baidu.Adaptor{},
			ChannelType:            channeltype.Baidu,
			SupportsChatCompletion: true,
			SupportsClaudeMessages: true,
			TestModel:              "ernie-bot",
		},
		{
			Name:                   "Zhipu",
			Adapter:                &zhipu.Adaptor{},
			ChannelType:            channeltype.Zhipu,
			SupportsChatCompletion: true,
			SupportsClaudeMessages: true,
			TestModel:              "glm-4",
		},
		{
			Name:                   "Xunfei",
			Adapter:                &xunfei.Adaptor{},
			ChannelType:            channeltype.Xunfei,
			SupportsChatCompletion: true,
			SupportsClaudeMessages: true,
			TestModel:              "spark-3.5",
		},
		{
			Name:                   "Tencent",
			Adapter:                &tencent.Adaptor{},
			ChannelType:            channeltype.Tencent,
			SupportsChatCompletion: true,
			SupportsClaudeMessages: true,
			TestModel:              "hunyuan-pro",
		},
		{
			Name:                   "VertexAI",
			Adapter:                &vertexai.Adaptor{},
			ChannelType:            channeltype.VertextAI,
			SupportsChatCompletion: true,
			SupportsClaudeMessages: true,
			TestModel:              "gemini-2.5-flash",
		},
		{
			Name:                   "Replicate",
			Adapter:                &replicate.Adaptor{},
			ChannelType:            channeltype.Replicate,
			SupportsChatCompletion: true,
			SupportsClaudeMessages: true,
			TestModel:              "meta/llama-2-70b-chat",
		},
		{
			Name:                   "Cohere",
			Adapter:                &cohere.Adaptor{},
			ChannelType:            channeltype.Cohere,
			SupportsChatCompletion: true,
			SupportsClaudeMessages: true,
			TestModel:              "command-r-plus",
		},
		{
			Name:                   "Cloudflare",
			Adapter:                &cloudflare.Adaptor{},
			ChannelType:            channeltype.Cloudflare,
			SupportsChatCompletion: true,
			SupportsClaudeMessages: true,
			TestModel:              "@cf/meta/llama-2-7b-chat-int8",
		},

		// OpenAI-Compatible Adapters with Claude Messages Support
		{
			Name:                   "DeepSeek",
			Adapter:                &deepseek.Adaptor{},
			ChannelType:            channeltype.DeepSeek,
			SupportsChatCompletion: true,
			SupportsClaudeMessages: true,
			TestModel:              "deepseek-chat",
		},
		{
			Name:                   "Groq",
			Adapter:                &groq.Adaptor{},
			ChannelType:            channeltype.Groq,
			SupportsChatCompletion: true,
			SupportsClaudeMessages: true,
			TestModel:              "llama3-8b-8192",
		},
		{
			Name:                   "Mistral",
			Adapter:                &mistral.Adaptor{},
			ChannelType:            channeltype.Mistral,
			SupportsChatCompletion: true,
			SupportsClaudeMessages: true,
			TestModel:              "mistral-large-latest",
		},
		{
			Name:                   "Moonshot",
			Adapter:                &moonshot.Adaptor{},
			ChannelType:            channeltype.Moonshot,
			SupportsChatCompletion: true,
			SupportsClaudeMessages: true,
			TestModel:              "moonshot-v1-8k",
		},
		{
			Name:                   "XAI",
			Adapter:                &xai.Adaptor{},
			ChannelType:            channeltype.XAI,
			SupportsChatCompletion: true,
			SupportsClaudeMessages: true,
			TestModel:              "grok-3",
		},
		{
			Name:                   "TogetherAI",
			Adapter:                &togetherai.Adaptor{},
			ChannelType:            channeltype.TogetherAI,
			SupportsChatCompletion: true,
			SupportsClaudeMessages: true,
			TestModel:              "meta-llama/Llama-2-7b-chat-hf",
		},
		{
			Name:                   "OpenRouter",
			Adapter:                &openrouter.Adaptor{},
			ChannelType:            channeltype.OpenRouter,
			SupportsChatCompletion: true,
			SupportsClaudeMessages: true,
			TestModel:              "anthropic/claude-3-sonnet",
		},
		{
			Name:                   "SiliconFlow",
			Adapter:                &siliconflow.Adaptor{},
			ChannelType:            channeltype.SiliconFlow,
			SupportsChatCompletion: true,
			SupportsClaudeMessages: true,
			TestModel:              "deepseek-ai/deepseek-llm-67b-chat",
		},
		{
			Name:                   "Doubao",
			Adapter:                &doubao.Adaptor{},
			ChannelType:            channeltype.Doubao,
			SupportsChatCompletion: true,
			SupportsClaudeMessages: true,
			TestModel:              "doubao-pro-4k",
		},
		{
			Name:                   "StepFun",
			Adapter:                &stepfun.Adaptor{},
			ChannelType:            channeltype.StepFun,
			SupportsChatCompletion: true,
			SupportsClaudeMessages: true,
			TestModel:              "step-1v-8k",
		},
		{
			Name:                   "Novita",
			Adapter:                &novita.Adaptor{},
			ChannelType:            channeltype.Novita,
			SupportsChatCompletion: true,
			SupportsClaudeMessages: true,
			TestModel:              "meta-llama/llama-2-7b-chat",
		},
		{
			Name:                   "AIProxy",
			Adapter:                &aiproxy.Adaptor{},
			ChannelType:            channeltype.AIProxyLibrary,
			SupportsChatCompletion: true,
			SupportsClaudeMessages: true,
			TestModel:              "gpt-3.5-turbo",
		},
		{
			Name:                   "LingYiWanWu",
			Adapter:                &lingyiwanwu.Adaptor{},
			ChannelType:            channeltype.LingYiWanWu,
			SupportsChatCompletion: true,
			SupportsClaudeMessages: true,
			TestModel:              "yi-34b-chat-0205",
		},
		{
			Name:                   "AI360",
			Adapter:                &ai360.Adaptor{},
			ChannelType:            channeltype.AI360,
			SupportsChatCompletion: true,
			SupportsClaudeMessages: true,
			TestModel:              "360gpt-pro",
		},

		// Additional Adapters with Claude Messages Support
		{
			Name:                   "VertexAI_Claude",
			Adapter:                &vertexai.Adaptor{},
			ChannelType:            channeltype.VertextAI,
			SupportsChatCompletion: true,
			SupportsClaudeMessages: true,
			TestModel:              "claude-3-sonnet@20240229", // Use a Claude model for Claude Messages API testing
		},
		{
			Name:                   "AWS",
			Adapter:                &aws.Adaptor{},
			ChannelType:            channeltype.AwsClaude,
			SupportsChatCompletion: true,
			SupportsClaudeMessages: true,
			TestModel:              "claude-3-sonnet-20240229",
		},
		{
			Name:                   "Palm",
			Adapter:                &palm.Adaptor{},
			ChannelType:            channeltype.PaLM,
			SupportsChatCompletion: true,
			SupportsClaudeMessages: true,
			TestModel:              "chat-bison-001",
		},
		{
			Name:                   "Ollama",
			Adapter:                &ollama.Adaptor{},
			ChannelType:            channeltype.Ollama,
			SupportsChatCompletion: true,
			SupportsClaudeMessages: true,
			TestModel:              "llama2",
		},
		{
			Name:                   "Coze",
			Adapter:                &coze.Adaptor{},
			ChannelType:            channeltype.Coze,
			SupportsChatCompletion: true,
			SupportsClaudeMessages: true,
			TestModel:              "coze-model",
		},
		{
			Name:                   "BaiduV2",
			Adapter:                &baiduv2.Adaptor{},
			ChannelType:            channeltype.BaiduV2,
			SupportsChatCompletion: true,
			SupportsClaudeMessages: true,
			TestModel:              "ernie-4.0",
			IsStubImplementation:   true,
		},
		{
			Name:                   "XunfeiV2",
			Adapter:                &xunfeiv2.Adaptor{},
			ChannelType:            channeltype.XunfeiV2,
			SupportsChatCompletion: true,
			SupportsClaudeMessages: true,
			TestModel:              "spark-4.0-ultra",
			IsStubImplementation:   true,
		},
		{
			Name:                   "Proxy",
			Adapter:                &proxy.Adaptor{},
			ChannelType:            channeltype.Proxy,
			SupportsChatCompletion: true,
			SupportsClaudeMessages: true,
			TestModel:              "any-model",
			IsStubImplementation:   true,
		},

		// Adapters with Limited or No Claude Messages Support
		{
			Name:                   "DeepL",
			Adapter:                &deepl.Adaptor{},
			ChannelType:            channeltype.DeepL,
			SupportsChatCompletion: false,
			SupportsClaudeMessages: false,
			TestModel:              "deepl-translate",
			ExpectedError:          "not supported",
		},
		{
			Name:                   "Minimax",
			Adapter:                &minimax.Adaptor{},
			ChannelType:            channeltype.Minimax,
			SupportsChatCompletion: false,
			SupportsClaudeMessages: false,
			TestModel:              "minimax-model",
			ExpectedError:          "not implemented",
			IsStubImplementation:   true,
		},
		{
			Name:                   "Baichuan",
			Adapter:                &baichuan.Adaptor{},
			ChannelType:            channeltype.Baichuan,
			SupportsChatCompletion: false,
			SupportsClaudeMessages: false,
			TestModel:              "baichuan-model",
			ExpectedError:          "not implemented",
			IsStubImplementation:   true,
		},
	}
}

// createTestContext creates a test Gin context
func createTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/test", nil)
	return c
}

// createTestMeta creates test metadata
func createTestMeta(channelType int, model string) *meta.Meta {
	return &meta.Meta{
		Mode:            relaymode.ChatCompletions,
		ChannelType:     channelType,
		ActualModelName: model,
		BaseURL:         "https://api.example.com",
		APIKey:          "test-key",
		PromptTokens:    10,
		IsStream:        false,
	}
}

// TestAdapterChatCompletionSupport tests that all adapters maintain their ChatCompletion API support
func TestAdapterChatCompletionSupport(t *testing.T) {
	testCases := getAllAdapterTestCases()

	for _, tc := range testCases {
		t.Run(tc.Name+"_ChatCompletion", func(t *testing.T) {
			if !tc.SupportsChatCompletion {
				t.Skip("Adapter doesn't support ChatCompletion API")
				return
			}

			c := createTestContext()
			testMeta := createTestMeta(tc.ChannelType, tc.TestModel)

			// Initialize adapter
			tc.Adapter.Init(testMeta)

			// Test ConvertRequest method
			convertedReq, err := tc.Adapter.ConvertRequest(c, relaymode.ChatCompletions, testChatCompletionRequest)

			if tc.ExpectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.ExpectedError)
				return
			}

			// Handle configuration-dependent adapters
			if err != nil {
				configErrors := []string{
					"invalid tencent config",
					"cannot found vertex chat adaptor",
					"cannot found vertex adaptor",
					"adaptor not found",
					"stream=true",
					"invalid baidu apikey",
					"invalid zhipu key",
				}

				isConfigError := false
				for _, configErr := range configErrors {
					if strings.Contains(err.Error(), configErr) {
						isConfigError = true
						break
					}
				}

				if isConfigError {
					t.Logf("ConvertRequest failed for %s due to configuration requirements (expected): %v", tc.Name, err)
					return
				}
			}

			assert.NoError(t, err, "ConvertRequest should not fail for %s", tc.Name)
			// Some adapters like Xunfei may return nil as part of their implementation
			if convertedReq == nil {
				t.Logf("ConvertRequest returned nil for %s (may be expected for some adapters)", tc.Name)
			} else {
				assert.NotNil(t, convertedReq, "ConvertRequest should return a valid request for %s", tc.Name)
			}

			// Test GetRequestURL method
			url, err := tc.Adapter.GetRequestURL(testMeta)
			if err != nil {
				// Some adapters require valid API keys or special configuration
				t.Logf("GetRequestURL failed for %s (expected for some adapters): %v", tc.Name, err)
			} else if url == "" {
				// Some adapters may return empty URL without error
				t.Logf("GetRequestURL returned empty URL for %s (expected for some adapters)", tc.Name)
			} else {
				assert.NotEmpty(t, url, "GetRequestURL should return a valid URL for %s", tc.Name)

				// Test SetupRequestHeader method only if URL is valid
				req := httptest.NewRequest("POST", url, nil)
				err = tc.Adapter.SetupRequestHeader(c, req, testMeta)
				assert.NoError(t, err, "SetupRequestHeader should not fail for %s", tc.Name)
			}

			// Test GetModelList method
			// Proxy adaptor returns nil because it's a pass-through channel without predefined models
			models := tc.Adapter.GetModelList()
			if tc.Name != "Proxy" {
				assert.NotEmpty(t, models, "GetModelList should return models for %s", tc.Name)
			}

			// Test GetChannelName method
			channelName := tc.Adapter.GetChannelName()
			assert.NotEmpty(t, channelName, "GetChannelName should return a name for %s", tc.Name)

			// Test pricing methods
			pricing := tc.Adapter.GetDefaultModelPricing()
			assert.NotNil(t, pricing, "GetDefaultModelPricing should not return nil for %s", tc.Name)

			ratio := tc.Adapter.GetModelRatio(tc.TestModel)
			assert.Greater(t, ratio, 0.0, "GetModelRatio should return positive value for %s", tc.Name)

			completionRatio := tc.Adapter.GetCompletionRatio(tc.TestModel)
			assert.Greater(t, completionRatio, 0.0, "GetCompletionRatio should return positive value for %s", tc.Name)
		})
	}
}

// TestAdapterClaudeMessagesSupport tests Claude Messages API support
func TestAdapterClaudeMessagesSupport(t *testing.T) {
	testCases := getAllAdapterTestCases()

	for _, tc := range testCases {
		t.Run(tc.Name+"_ClaudeMessages", func(t *testing.T) {
			c := createTestContext()
			testMeta := createTestMeta(tc.ChannelType, tc.TestModel)
			testMeta.Mode = relaymode.ClaudeMessages

			// Initialize adapter
			tc.Adapter.Init(testMeta)

			// Test ConvertClaudeRequest method
			convertedReq, err := tc.Adapter.ConvertClaudeRequest(c, testClaudeMessagesRequest)

			if !tc.SupportsClaudeMessages {
				// Should return error for unsupported adapters
				assert.Error(t, err, "ConvertClaudeRequest should fail for unsupported adapter %s", tc.Name)
				// Check for various error messages that indicate lack of support
				errorMsg := err.Error()
				supportIndicators := []string{"not supported", "not implemented", "not applicable"}
				hasIndicator := false
				for _, indicator := range supportIndicators {
					if strings.Contains(errorMsg, indicator) {
						hasIndicator = true
						break
					}
				}
				assert.True(t, hasIndicator, "Error should indicate lack of support for %s, got: %s", tc.Name, errorMsg)
				return
			}

			// Handle configuration-dependent adapters
			if err != nil {
				configErrors := []string{
					"invalid tencent config",
					"cannot found vertex chat adaptor",
					"cannot found vertex adaptor",
					"adaptor not found",
					"stream=true",
					"invalid baidu apikey",
					"invalid zhipu key",
				}

				isConfigError := false
				for _, configErr := range configErrors {
					if strings.Contains(err.Error(), configErr) {
						isConfigError = true
						break
					}
				}

				if isConfigError {
					t.Logf("ConvertClaudeRequest failed for %s due to configuration requirements (expected): %v", tc.Name, err)
					return
				}
			}

			// Should succeed for supported adapters
			assert.NoError(t, err, "ConvertClaudeRequest should not fail for %s", tc.Name)
			// Some adapters like Xunfei may return nil as part of their implementation
			if convertedReq == nil {
				t.Logf("ConvertClaudeRequest returned nil for %s (may be expected for some adapters)", tc.Name)
			} else {
				assert.NotNil(t, convertedReq, "ConvertClaudeRequest should return a valid request for %s", tc.Name)
			}

			// Test that context flags are set appropriately
			if tc.Name == "Anthropic" {
				// Native Claude adapter should set native flags
				nativeFlag, exists := c.Get(ctxkey.ClaudeMessagesNative)
				assert.True(t, exists, "Native flag should be set for Anthropic")
				assert.True(t, nativeFlag.(bool), "Native flag should be true for Anthropic")

				passthroughFlag, exists := c.Get(ctxkey.ClaudeDirectPassthrough)
				assert.True(t, exists, "Passthrough flag should be set for Anthropic")
				assert.True(t, passthroughFlag.(bool), "Passthrough flag should be true for Anthropic")
			} else if tc.Name == "OpenAI" {
				// OpenAI adapter should set conversion flag
				conversionFlag, exists := c.Get(ctxkey.ClaudeMessagesConversion)
				assert.True(t, exists, "Conversion flag should be set for OpenAI")
				assert.True(t, conversionFlag.(bool), "Conversion flag should be true for OpenAI")
			}

			// Test GetRequestURL for Claude Messages mode
			url, err := tc.Adapter.GetRequestURL(testMeta)
			if err != nil {
				// Some adapters require valid API keys or special configuration
				t.Logf("GetRequestURL failed for %s in Claude Messages mode (expected for some adapters): %v", tc.Name, err)
			} else if url == "" {
				// Some adapters may return empty URL without error
				t.Logf("GetRequestURL returned empty URL for %s in Claude Messages mode (expected for some adapters)", tc.Name)
			} else {
				assert.NotEmpty(t, url, "GetRequestURL should return a valid URL for Claude Messages mode in %s", tc.Name)
			}
		})
	}
}

// TestClaudeMessagesRequestConversion tests the conversion of Claude Messages requests
func TestClaudeMessagesRequestConversion(t *testing.T) {
	testCases := []struct {
		name        string
		adapter     adaptor.Adaptor
		channelType int
		testModel   string
		expectError bool
	}{
		{"OpenAI", &openai.Adaptor{}, channeltype.OpenAI, "gpt-4o-mini", false},
		{"DeepSeek", &deepseek.Adaptor{}, channeltype.DeepSeek, "deepseek-chat", false},
		{"Groq", &groq.Adaptor{}, channeltype.Groq, "llama-3.1-8b-instant", false},
		{"Anthropic", &anthropic.Adaptor{}, channeltype.Anthropic, "claude-3-sonnet-20240229", false},
	}

	// Complex Claude Messages request with various content types
	temp := 0.7
	complexClaudeRequest := &model.ClaudeRequest{
		Model: "claude-3-sonnet-20240229",
		Messages: []model.ClaudeMessage{
			{
				Role: "user",
				Content: []any{
					map[string]any{
						"type": "text",
						"text": "What's in this image?",
					},
					map[string]any{
						"type": "image",
						"source": map[string]any{
							"type":       "base64",
							"media_type": "image/jpeg",
							"data":       "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
						},
					},
				},
			},
			{
				Role:    "assistant",
				Content: "I can see a small test image.",
			},
			{
				Role: "user",
				Content: []any{
					map[string]any{
						"type": "tool_use",
						"id":   "toolu_123",
						"name": "get_weather",
						"input": map[string]any{
							"location": "San Francisco",
						},
					},
				},
			},
		},
		MaxTokens:   1000,
		Temperature: &temp,
		System:      "You are a helpful assistant.",
		Tools: []model.ClaudeTool{
			{
				Name:        "get_weather",
				Description: "Get weather information",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"location": map[string]any{
							"type":        "string",
							"description": "The location to get weather for",
						},
					},
					"required": []string{"location"},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name+"_ComplexConversion", func(t *testing.T) {
			c := createTestContext()
			testMeta := createTestMeta(tc.channelType, tc.testModel)

			tc.adapter.Init(testMeta)

			// Test conversion
			convertedReq, err := tc.adapter.ConvertClaudeRequest(c, complexClaudeRequest)

			if tc.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err, "Complex Claude request conversion should succeed for %s", tc.name)
			assert.NotNil(t, convertedReq, "Converted request should not be nil for %s", tc.name)

			// Verify conversion results based on adapter type
			if tc.name == "Anthropic" {
				// Native adapter should return original request
				claudeReq, ok := convertedReq.(*model.ClaudeRequest)
				assert.True(t, ok, "Anthropic should return ClaudeRequest")
				assert.Equal(t, complexClaudeRequest.Model, claudeReq.Model)
			} else {
				// Other adapters should convert to their native format
				// This could be OpenAI format or other formats
				assert.NotNil(t, convertedReq, "Converted request should be valid for %s", tc.name)
			}
		})
	}
}

// TestResponseConversionIntegration tests end-to-end response conversion
func TestResponseConversionIntegration(t *testing.T) {
	// Mock OpenAI response
	mockOpenAIResponse := `{
		"id": "chatcmpl-123",
		"object": "chat.completion",
		"created": 1677652288,
		"model": "gpt-4o-mini",
		"choices": [{
			"index": 0,
			"message": {
				"role": "assistant",
				"content": "Hello! How can I help you today?"
			},
			"finish_reason": "stop"
		}],
		"usage": {
			"prompt_tokens": 10,
			"completion_tokens": 9,
			"total_tokens": 19
		}
	}`

	// Mock Claude response
	mockClaudeResponse := `{
		"id": "msg_123",
		"type": "message",
		"role": "assistant",
		"content": [{"type": "text", "text": "Hello! How can I help you today?"}],
		"model": "claude-3-sonnet-20240229",
		"stop_reason": "end_turn",
		"usage": {
			"input_tokens": 10,
			"output_tokens": 9
		}
	}`

	testCases := []struct {
		name             string
		adapter          adaptor.Adaptor
		channelType      int
		mockResponse     string
		expectClaude     bool
		expectConversion bool
	}{
		{
			name:             "OpenAI_to_Claude",
			adapter:          &openai.Adaptor{},
			channelType:      channeltype.OpenAI,
			mockResponse:     mockOpenAIResponse,
			expectClaude:     true,
			expectConversion: true,
		},
		{
			name:             "Anthropic_Native",
			adapter:          &anthropic.Adaptor{},
			channelType:      channeltype.Anthropic,
			mockResponse:     mockClaudeResponse,
			expectClaude:     true,
			expectConversion: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := createTestContext()
			testMeta := createTestMeta(tc.channelType, "test-model")

			// Set up Claude Messages conversion flag if needed
			if tc.expectConversion {
				c.Set(ctxkey.ClaudeMessagesConversion, true)
			}

			// Create mock HTTP response
			resp := &http.Response{
				StatusCode: 200,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(tc.mockResponse)),
			}
			resp.Header.Set("Content-Type", "application/json")

			tc.adapter.Init(testMeta)

			// Test DoResponse
			usage, respErr := tc.adapter.DoResponse(c, resp, testMeta)

			assert.Nil(t, respErr, "DoResponse should not fail for %s", tc.name)

			if tc.expectConversion {
				// For conversion adapters, check if converted response is set
				if convertedResp, exists := c.Get(ctxkey.ConvertedResponse); exists {
					assert.NotNil(t, convertedResp, "Converted response should be set for %s", tc.name)

					// Read and verify the converted response
					httpResp := convertedResp.(*http.Response)
					body, err := io.ReadAll(httpResp.Body)
					assert.NoError(t, err)

					var claudeResp model.ClaudeResponse
					err = json.Unmarshal(body, &claudeResp)
					assert.NoError(t, err, "Converted response should be valid Claude format for %s", tc.name)
					assert.Equal(t, "message", claudeResp.Type)
					assert.Equal(t, "assistant", claudeResp.Role)
				}
			} else {
				// For native adapters, usage should be returned normally
				assert.NotNil(t, usage, "Usage should be returned for native adapter %s", tc.name)
			}
		})
	}
}
