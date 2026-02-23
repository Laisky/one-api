package openai

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common/ctxkey"
	appmodel "github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/adaptor/common/deepseekcompat"
	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

// TestNormalizeDeepSeekThinkingType verifies mapping of Claude thinking.type values into DeepSeek-compatible enums.
func TestNormalizeDeepSeekThinkingType(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		rawType      string
		budgetTokens int
		expectedType string
		expectedEdit bool
	}{
		{name: "already enabled", rawType: "enabled", budgetTokens: 0, expectedType: "enabled", expectedEdit: false},
		{name: "adaptive to enabled", rawType: "adaptive", budgetTokens: 4096, expectedType: "enabled", expectedEdit: true},
		{name: "unknown with budget to enabled", rawType: "auto", budgetTokens: 1024, expectedType: "enabled", expectedEdit: true},
		{name: "unknown without budget to disabled", rawType: "auto", budgetTokens: 0, expectedType: "disabled", expectedEdit: true},
		{name: "empty with budget to enabled", rawType: "", budgetTokens: 2048, expectedType: "enabled", expectedEdit: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			normalized, changed := deepseekcompat.NormalizeThinkingType(testCase.rawType, testCase.budgetTokens)
			require.Equal(t, testCase.expectedType, normalized)
			require.Equal(t, testCase.expectedEdit, changed)
		})
	}
}

// TestConvertClaudeRequest_NormalizesAdaptiveThinkingForDeepSeek ensures Claude adaptive thinking is coerced
// to DeepSeek-compatible enabled mode while preserving budget tokens.
func TestConvertClaudeRequest_NormalizesAdaptiveThinkingForDeepSeek(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	claudeRequest := &relaymodel.ClaudeRequest{
		Model:     "deepseek-chat",
		MaxTokens: 512,
		Messages: []relaymodel.ClaudeMessage{
			{Role: "user", Content: "hello"},
		},
		Thinking: &relaymodel.Thinking{Type: "adaptive", BudgetTokens: 2048},
	}

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = &http.Request{}
	context.Set(ctxkey.Meta, &meta.Meta{
		Mode:            relaymode.ClaudeMessages,
		ChannelType:     channeltype.OpenAICompatible,
		BaseURL:         "https://api.deepseek.com",
		RequestURLPath:  "/v1/messages",
		ActualModelName: "deepseek-chat",
		Config:          appmodel.ChannelConfig{APIFormat: channeltype.OpenAICompatibleAPIFormatChatCompletion},
	})

	adaptor := &Adaptor{}
	convertedAny, err := adaptor.ConvertClaudeRequest(context, claudeRequest)
	require.NoError(t, err)

	converted, ok := convertedAny.(*relaymodel.GeneralOpenAIRequest)
	require.True(t, ok)
	require.NotNil(t, converted.Thinking)
	require.Equal(t, "enabled", converted.Thinking.Type)
	require.Equal(t, 2048, converted.Thinking.BudgetTokens)
}

// TestConvertClaudeRequest_PreservesAdaptiveThinkingForNonDeepSeek ensures non-DeepSeek routes remain unchanged
// for backward compatibility.
func TestConvertClaudeRequest_PreservesAdaptiveThinkingForNonDeepSeek(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	claudeRequest := &relaymodel.ClaudeRequest{
		Model:     "gpt-4o-mini",
		MaxTokens: 256,
		Messages: []relaymodel.ClaudeMessage{
			{Role: "user", Content: "hello"},
		},
		Thinking: &relaymodel.Thinking{Type: "adaptive", BudgetTokens: 1024},
	}

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = &http.Request{}
	context.Set(ctxkey.Meta, &meta.Meta{
		Mode:            relaymode.ClaudeMessages,
		ChannelType:     channeltype.OpenAICompatible,
		BaseURL:         "https://proxy.example.com",
		RequestURLPath:  "/v1/messages",
		ActualModelName: "gpt-4o-mini",
		Config:          appmodel.ChannelConfig{APIFormat: channeltype.OpenAICompatibleAPIFormatChatCompletion},
	})

	adaptor := &Adaptor{}
	convertedAny, err := adaptor.ConvertClaudeRequest(context, claudeRequest)
	require.NoError(t, err)

	converted, ok := convertedAny.(*relaymodel.GeneralOpenAIRequest)
	require.True(t, ok)
	require.NotNil(t, converted.Thinking)
	require.Equal(t, "adaptive", converted.Thinking.Type)
	require.Equal(t, 1024, converted.Thinking.BudgetTokens)
}

// TestConvertRequest_NormalizesAdaptiveThinkingForDeepSeek ensures direct chat-completions payloads routed
// to DeepSeek also normalize unsupported thinking.type values.
func TestConvertRequest_NormalizesAdaptiveThinkingForDeepSeek(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	request := &relaymodel.GeneralOpenAIRequest{
		Model: "deepseek-chat",
		Messages: []relaymodel.Message{
			{Role: "user", Content: "hello"},
		},
		Thinking: &relaymodel.Thinking{Type: "adaptive", BudgetTokens: 1536},
	}

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = &http.Request{}
	context.Set(ctxkey.Meta, &meta.Meta{
		Mode:            relaymode.ChatCompletions,
		ChannelType:     channeltype.OpenAICompatible,
		BaseURL:         "https://api.deepseek.com",
		RequestURLPath:  "/v1/chat/completions",
		ActualModelName: "deepseek-chat",
		Config:          appmodel.ChannelConfig{APIFormat: channeltype.OpenAICompatibleAPIFormatChatCompletion},
	})

	adaptor := &Adaptor{}
	convertedAny, err := adaptor.ConvertRequest(context, relaymode.ChatCompletions, request)
	require.NoError(t, err)

	converted, ok := convertedAny.(*relaymodel.GeneralOpenAIRequest)
	require.True(t, ok)
	require.NotNil(t, converted.Thinking)
	require.Equal(t, "enabled", converted.Thinking.Type)
	require.Equal(t, 1536, converted.Thinking.BudgetTokens)
}
