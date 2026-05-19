package openai

import (
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// validToolNamePattern mirrors `^[a-zA-Z0-9_-]+$` enforced by OpenAI/DeepSeek/Anthropic.
var validToolNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func newOpenAIToolCtx(t *testing.T, channelType int, baseURL, modelName string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	m := &metalib.Meta{
		ChannelType:    channelType,
		BaseURL:        baseURL,
		RequestURLPath: "/v1/chat/completions",
		Mode:           relaymode.ChatCompletions,
		// Force ChatCompletion path so the result is the original
		// *model.GeneralOpenAIRequest rather than the Response API shape.
		ResponseAPIFallback: true,
		ActualModelName:     modelName,
	}
	metalib.Set2Context(c, m)
	return c
}

// TestConvertRequest_SanitizesToolNames_OpenAIChannel ensures the OpenAI adapter
// rewrites tool/function names that include disallowed punctuation, regardless
// of which strict upstream the channel ultimately calls.
func TestConvertRequest_SanitizesToolNames_OpenAIChannel(t *testing.T) {
	t.Parallel()

	c := newOpenAIToolCtx(t, channeltype.OpenAI, "https://api.openai.com", "gpt-4o-mini")

	request := &model.GeneralOpenAIRequest{
		Model: "gpt-4o-mini",
		Messages: []model.Message{
			{Role: "user", Content: "hi"},
		},
		Tools: []model.Tool{
			{
				Type: "function",
				Function: &model.Function{
					Name:        "fs.read",
					Description: "read a file",
					Parameters:  map[string]any{"type": "object"},
				},
			},
		},
	}

	adaptor := &Adaptor{ChannelType: channeltype.OpenAI}
	out, err := adaptor.ConvertRequest(c, relaymode.ChatCompletions, request)
	require.NoError(t, err)

	converted, ok := out.(*model.GeneralOpenAIRequest)
	require.True(t, ok, "expected *model.GeneralOpenAIRequest, got %T", out)
	require.Len(t, converted.Tools, 1)
	require.NotNil(t, converted.Tools[0].Function)
	require.Equal(t, "fs_read", converted.Tools[0].Function.Name)
	require.True(t, validToolNamePattern.MatchString(converted.Tools[0].Function.Name))

	raw, exists := c.Get(ctxkey.ToolNameSanitizeMap)
	require.True(t, exists, "expected rename map on context")
	mp, ok := raw.(map[string]string)
	require.True(t, ok)
	require.Equal(t, "fs.read", mp["fs_read"])
}

// TestConvertRequest_SanitizesToolNames_GeminiCompatChannel exercises the
// OpenAI adapter when targeting a Gemini-style OpenAI-compatible channel. Even
// though Gemini may accept dots, sanitization is applied uniformly; the test
// asserts the rename map allows a transparent round-trip via the response
// path.
func TestConvertRequest_SanitizesToolNames_GeminiCompatChannel(t *testing.T) {
	t.Parallel()

	c := newOpenAIToolCtx(t, channeltype.OpenAICompatible, "https://generativelanguage.googleapis.com", "gemini-2.0-flash")

	request := &model.GeneralOpenAIRequest{
		Model: "gemini-2.0-flash",
		Messages: []model.Message{
			{Role: "user", Content: "hi"},
		},
		Tools: []model.Tool{
			{
				Type: "function",
				Function: &model.Function{
					Name:        "search.web",
					Description: "search the web",
					Parameters:  map[string]any{"type": "object"},
				},
			},
		},
	}

	adaptor := &Adaptor{ChannelType: channeltype.OpenAICompatible}
	out, err := adaptor.ConvertRequest(c, relaymode.ChatCompletions, request)
	require.NoError(t, err)

	converted, ok := out.(*model.GeneralOpenAIRequest)
	require.True(t, ok, "expected *model.GeneralOpenAIRequest, got %T", out)
	require.Len(t, converted.Tools, 1)
	require.NotNil(t, converted.Tools[0].Function)
	require.Equal(t, "search_web", converted.Tools[0].Function.Name)

	raw, exists := c.Get(ctxkey.ToolNameSanitizeMap)
	require.True(t, exists)
	mp, ok := raw.(map[string]string)
	require.True(t, ok)
	require.Equal(t, "search.web", mp["search_web"])
}
