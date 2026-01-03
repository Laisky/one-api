package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

func TestProxyTokenSummaryNilUsage(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/v1/videos/abc", nil)
	c.Request = req

	meta := &metalib.Meta{ChannelId: 7}

	prompt, completion := proxyTokenSummary(c, meta, nil)
	require.Equal(t, 0, prompt)
	require.Equal(t, 0, completion)
}

func TestProxyTokenSummaryWithUsage(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	c.Request = req

	meta := &metalib.Meta{ChannelId: 3}
	usage := &relaymodel.Usage{PromptTokens: 12, CompletionTokens: 34}

	prompt, completion := proxyTokenSummary(c, meta, usage)
	require.Equal(t, 12, prompt)
	require.Equal(t, 34, completion)
}
