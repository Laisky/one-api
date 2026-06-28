package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/internal/streambridgetest"
	"github.com/Laisky/one-api/relay/meta"
)

func newProxyCtx(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	return c, rec
}

func chatSSEResponse() *http.Response {
	body := strings.Join([]string{
		`data: {"id":"x","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"Hello"}}]}`,
		``,
		`data: {"id":"x","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":" world"}}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	h := http.Header{}
	h.Set("Content-Type", "text/event-stream")
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     h,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// TestProxyDoResponse_RoutesThroughBridge verifies that when the Response API
// streaming fallback installs a rewrite bridge, the proxy adaptor parses the
// upstream chat-completions SSE and routes it through the bridge instead of
// copying raw bytes to the client.
func TestProxyDoResponse_RoutesThroughBridge(t *testing.T) {
	c, rec := newProxyCtx(t)
	rw := &streambridgetest.Recorder{}
	c.Set(ctxkey.ResponseStreamRewriteHandler, rw)

	a := &Adaptor{}
	_, errResp := a.DoResponse(c, chatSSEResponse(), &meta.Meta{PromptTokens: 5, ActualModelName: "proxied-model"})
	require.Nil(t, errResp)

	require.Equal(t, "Hello world", rw.JoinedDeltas())
	require.Equal(t, 1, rw.DoneCount)
	require.True(t, rw.UsageSet)
	// No raw passthrough of upstream chunk bytes when the bridge is active.
	require.NotContains(t, rec.Body.String(), `"object":"chat.completion.chunk"`)
}

// TestProxyDoResponse_VerbatimWithoutBridge verifies the default proxy behavior
// is unchanged when no bridge is installed: the upstream body is copied through.
func TestProxyDoResponse_VerbatimWithoutBridge(t *testing.T) {
	c, rec := newProxyCtx(t)

	a := &Adaptor{}
	_, errResp := a.DoResponse(c, chatSSEResponse(), &meta.Meta{})
	require.Nil(t, errResp)

	body := rec.Body.String()
	require.Contains(t, body, `"object":"chat.completion.chunk"`)
	require.Contains(t, body, "Hello")
}
