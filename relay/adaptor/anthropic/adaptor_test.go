package anthropic

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

// TestGetRequestURL covers the native Anthropic Messages surface: the base URL is
// suffixed with /v1/messages regardless of channel (Azure's /anthropic prefix is
// handled by the dedicated azure adaptor, not here).
func TestGetRequestURL(t *testing.T) {
	t.Parallel()

	a := &Adaptor{}

	got, err := a.GetRequestURL(&metalib.Meta{BaseURL: "https://api.anthropic.com"})
	require.NoError(t, err)
	require.Equal(t, "https://api.anthropic.com/v1/messages", got)

	got, err = a.GetRequestURL(&metalib.Meta{BaseURL: "https://proxy.example.com"})
	require.NoError(t, err)
	require.Equal(t, "https://proxy.example.com/v1/messages", got)
}

func TestSetupRequestHeader_MergesBetaHeadersAndToolSearchBeta(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("anthropic-beta", "messages-2023-12-15,custom-beta")
	c.Set(ctxkey.ClaudeToolSearchEnabled, true)

	upstreamReq, err := http.NewRequest(http.MethodPost, "https://example.com/v1/messages", nil)
	require.NoError(t, err)

	meta := &metalib.Meta{APIKey: "k", ActualModelName: "claude-4-sonnet-20250514"}
	a := &Adaptor{}
	require.NoError(t, a.SetupRequestHeader(c, upstreamReq, meta))

	require.Equal(t, AnthropicVersionDefault, upstreamReq.Header.Get("anthropic-version"))
	betaTokens := strings.Split(upstreamReq.Header.Get("anthropic-beta"), ",")
	require.Equal(t, []string{
		AnthropicBetaMessages,
		"custom-beta",
		"context-1m-2025-08-07",
		"interleaved-thinking-2025-05-14",
		AnthropicBetaAdvancedToolUse,
	}, betaTokens)
}

func TestSetupRequestHeader_PreservesInboundAnthropicVersion(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("anthropic-version", "2025-01-01")

	upstreamReq, err := http.NewRequest(http.MethodPost, "https://example.com/v1/messages", nil)
	require.NoError(t, err)

	a := &Adaptor{}
	require.NoError(t, a.SetupRequestHeader(c, upstreamReq, &metalib.Meta{APIKey: "k", ActualModelName: "claude-3-7-sonnet-latest"}))
	require.Equal(t, "2025-01-01", upstreamReq.Header.Get("anthropic-version"))
}

func TestConvertClaudeRequest_SetsToolSearchContextFlag(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	request := &model.ClaudeRequest{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 128,
		Messages:  []model.ClaudeMessage{{Role: "user", Content: "hello"}},
		Tools: []model.ClaudeTool{
			{Type: "tool_search_tool_bm25_20251119", Name: "tool_search_tool_bm25"},
		},
	}

	a := &Adaptor{}
	_, err := a.ConvertClaudeRequest(c, request)
	require.NoError(t, err)

	toolSearchEnabledAny, exists := c.Get(ctxkey.ClaudeToolSearchEnabled)
	require.True(t, exists)
	require.Equal(t, true, toolSearchEnabledAny)
}

// TestConvertRequest_SanitizesDottedToolName covers the OpenAI→Anthropic
// conversion path: a `*model.GeneralOpenAIRequest` with a tool name containing
// disallowed punctuation must surface a sanitized identifier in the resulting
// Anthropic Request, and the rename map must be stashed on the context for
// later restoration by Handler/StreamHandler.
func TestConvertRequest_SanitizesDottedToolName(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)

	a := &Adaptor{}
	request := &model.GeneralOpenAIRequest{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 1024,
		Messages: []model.Message{
			{Role: "user", Content: "hi"},
		},
		Tools: []model.Tool{
			{
				Type: "function",
				Function: &model.Function{
					Name:        "tool.x",
					Description: "do x",
					Parameters:  map[string]any{"type": "object"},
				},
			},
		},
	}

	out, err := a.ConvertRequest(c, 0, request)
	require.NoError(t, err)
	anthropicReq, ok := out.(*Request)
	require.True(t, ok, "expected *anthropic.Request, got %T", out)
	require.Len(t, anthropicReq.Tools, 1)
	require.Equal(t, "tool_x", anthropicReq.Tools[0].Name)
	require.True(t, regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(anthropicReq.Tools[0].Name))

	raw, exists := c.Get(ctxkey.ToolNameSanitizeMap)
	require.True(t, exists, "expected rename map on context")
	mp, ok := raw.(map[string]string)
	require.True(t, ok)
	require.Equal(t, "tool.x", mp["tool_x"])
}
