package openai

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
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

func TestConvertRequest_NormalizesToolArrayForDeepSeekModelPrefix(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	request := &relaymodel.GeneralOpenAIRequest{
		Model: "deepseek-chat",
		Messages: []relaymodel.Message{
			{Role: "tool", ToolCallId: "call_2", Content: []any{map[string]any{"type": "text", "text": "ok"}}},
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
	require.Equal(t, "ok", converted.Messages[0].Content)
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
