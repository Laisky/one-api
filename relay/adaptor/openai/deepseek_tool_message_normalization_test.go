package openai

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

func TestConvertRequest_NormalizesToolArrayForDeepSeekBaseURL(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	request := &relaymodel.GeneralOpenAIRequest{
		Model: "deepseek-chat",
		Messages: []relaymodel.Message{
			{Role: "user", Content: "hello"},
			{Role: "tool", ToolCallId: "call_1", Content: []any{map[string]any{"type": "text", "text": "README.md\n"}}},
		},
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = &http.Request{}

	metaInfo := &meta.Meta{
		Mode:            relaymode.ChatCompletions,
		ChannelType:     channeltype.OpenAICompatible,
		BaseURL:         "https://api.deepseek.com",
		RequestURLPath:  "/v1/chat/completions",
		ActualModelName: "deepseek-chat",
		Config:          model.ChannelConfig{APIFormat: channeltype.OpenAICompatibleAPIFormatChatCompletion},
	}
	c.Set(ctxkey.Meta, metaInfo)

	adaptor := &Adaptor{}
	convertedAny, err := adaptor.ConvertRequest(c, relaymode.ChatCompletions, request)
	require.NoError(t, err)

	converted, ok := convertedAny.(*relaymodel.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Equal(t, "README.md\n", converted.Messages[1].Content)
}

func TestConvertRequest_DoesNotNormalizeToolArrayForDeepSeekModelPrefixOnly(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	original := []any{map[string]any{"type": "text", "text": "ok"}}
	request := &relaymodel.GeneralOpenAIRequest{
		Model: "deepseek-chat",
		Messages: []relaymodel.Message{
			{Role: "tool", ToolCallId: "call_2", Content: original},
		},
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = &http.Request{}

	metaInfo := &meta.Meta{
		Mode:            relaymode.ChatCompletions,
		ChannelType:     channeltype.OpenAICompatible,
		BaseURL:         "https://proxy.example.com",
		RequestURLPath:  "/v1/chat/completions",
		ActualModelName: "deepseek-chat",
		Config:          model.ChannelConfig{APIFormat: channeltype.OpenAICompatibleAPIFormatChatCompletion},
	}
	c.Set(ctxkey.Meta, metaInfo)

	adaptor := &Adaptor{}
	convertedAny, err := adaptor.ConvertRequest(c, relaymode.ChatCompletions, request)
	require.NoError(t, err)

	converted, ok := convertedAny.(*relaymodel.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Equal(t, original, converted.Messages[0].Content)
}

func TestConvertRequest_DoesNotNormalizeToolArrayForNonDeepSeek(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	original := []any{map[string]any{"type": "text", "text": "ok"}}
	request := &relaymodel.GeneralOpenAIRequest{
		Model: "gpt-4o-mini",
		Messages: []relaymodel.Message{
			{Role: "tool", ToolCallId: "call_3", Content: original},
		},
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = &http.Request{}

	metaInfo := &meta.Meta{
		Mode:            relaymode.ChatCompletions,
		ChannelType:     channeltype.OpenAICompatible,
		BaseURL:         "https://proxy.example.com",
		RequestURLPath:  "/v1/chat/completions",
		ActualModelName: "gpt-4o-mini",
		Config:          model.ChannelConfig{APIFormat: channeltype.OpenAICompatibleAPIFormatChatCompletion},
	}
	c.Set(ctxkey.Meta, metaInfo)

	adaptor := &Adaptor{}
	convertedAny, err := adaptor.ConvertRequest(c, relaymode.ChatCompletions, request)
	require.NoError(t, err)

	converted, ok := convertedAny.(*relaymodel.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Equal(t, original, converted.Messages[0].Content)
}

func TestConvertRequest_NormalizesToolArrayAtLaterMessageIndexForDeepSeek(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	request := &relaymodel.GeneralOpenAIRequest{
		Model: "deepseek-chat",
		Messages: []relaymodel.Message{
			{Role: "system", Content: "sys"},
			{Role: "user", Content: "u1"},
			{Role: "assistant", Content: "a1"},
			{Role: "user", Content: "u2"},
			{Role: "assistant", Content: "", ToolCalls: []relaymodel.Tool{{Id: "call_4", Type: "function", Function: &relaymodel.Function{Name: "Bash", Arguments: "{}"}}}},
			{Role: "tool", ToolCallId: "call_4", Content: []any{map[string]any{"type": "text", "text": "short tool output"}}},
		},
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = &http.Request{}

	metaInfo := &meta.Meta{
		Mode:            relaymode.ChatCompletions,
		ChannelType:     channeltype.OpenAICompatible,
		BaseURL:         "https://api.deepseek.com",
		RequestURLPath:  "/v1/chat/completions",
		ActualModelName: "deepseek-chat",
		Config:          model.ChannelConfig{APIFormat: channeltype.OpenAICompatibleAPIFormatChatCompletion},
	}
	c.Set(ctxkey.Meta, metaInfo)

	adaptor := &Adaptor{}
	convertedAny, err := adaptor.ConvertRequest(c, relaymode.ChatCompletions, request)
	require.NoError(t, err)

	converted, ok := convertedAny.(*relaymodel.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Equal(t, "short tool output", converted.Messages[5].Content)
}

// TestConvertRequest_EnforcesDeepSeekHistoryContractForDeepSeekBaseURL covers the
// generic OpenAI adaptor reaching a DeepSeek-contract upstream: the same history
// rules apply there, so the unanswered tool call must be dropped and the in-flight
// assistant turn must carry a reasoning_content field.
// Parameters: t is the testing handle used for assertions.
// Returns: nothing; the test fails through t when the contract is not applied.
func TestConvertRequest_EnforcesDeepSeekHistoryContractForDeepSeekBaseURL(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	request := &relaymodel.GeneralOpenAIRequest{
		Model: "deepseek-flash",
		Messages: []relaymodel.Message{
			{Role: "user", Content: "weather in Paris?"},
			{
				Role:    "assistant",
				Content: "Checking.",
				ToolCalls: []relaymodel.Tool{{
					Id:       "call_1",
					Type:     "function",
					Function: &relaymodel.Function{Name: "get_weather", Arguments: `{"city":"Paris"}`},
				}},
			},
		},
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = &http.Request{}

	metaInfo := &meta.Meta{
		Mode:            relaymode.ChatCompletions,
		ChannelType:     channeltype.OpenAICompatible,
		BaseURL:         "https://api.deepseek.com",
		RequestURLPath:  "/v1/chat/completions",
		ActualModelName: "deepseek-flash",
		Config:          model.ChannelConfig{APIFormat: channeltype.OpenAICompatibleAPIFormatChatCompletion},
	}
	c.Set(ctxkey.Meta, metaInfo)

	convertedAny, err := (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, request)
	require.NoError(t, err)

	converted, ok := convertedAny.(*relaymodel.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Len(t, converted.Messages, 2)
	require.Empty(t, converted.Messages[1].ToolCalls)
	require.NotNil(t, converted.Messages[1].ReasoningContent)
	require.Equal(t, "", *converted.Messages[1].ReasoningContent)
}

// TestConvertRequest_LeavesHistoryAloneForNonDeepSeekUpstream guards the scoping:
// these repairs encode DeepSeek's own validation, so an unrelated OpenAI-compatible
// upstream must keep receiving exactly what the client sent.
// Parameters: t is the testing handle used for assertions.
// Returns: nothing; the test fails through t when an unrelated upstream is rewritten.
func TestConvertRequest_LeavesHistoryAloneForNonDeepSeekUpstream(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	request := &relaymodel.GeneralOpenAIRequest{
		Model: "some-model",
		Messages: []relaymodel.Message{
			{Role: "user", Content: "weather in Paris?"},
			{
				Role:    "assistant",
				Content: "Checking.",
				ToolCalls: []relaymodel.Tool{{
					Id:       "call_1",
					Type:     "function",
					Function: &relaymodel.Function{Name: "get_weather", Arguments: `{"city":"Paris"}`},
				}},
			},
		},
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = &http.Request{}

	metaInfo := &meta.Meta{
		Mode:            relaymode.ChatCompletions,
		ChannelType:     channeltype.OpenAICompatible,
		BaseURL:         "https://api.example.com",
		RequestURLPath:  "/v1/chat/completions",
		ActualModelName: "some-model",
		Config:          model.ChannelConfig{APIFormat: channeltype.OpenAICompatibleAPIFormatChatCompletion},
	}
	c.Set(ctxkey.Meta, metaInfo)

	convertedAny, err := (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, request)
	require.NoError(t, err)

	converted, ok := convertedAny.(*relaymodel.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Len(t, converted.Messages, 2)
	require.Len(t, converted.Messages[1].ToolCalls, 1)
	require.Nil(t, converted.Messages[1].ReasoningContent)
}
