package anthropic

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common/ctxkey"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
)

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
