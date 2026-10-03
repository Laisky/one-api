package controller

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/stretchr/testify/require"
)

// TestReviewResponseDiagnosticsRedactUpstreamURL covers both buffered and
// streaming response logging, which run after the request helper returns.
func TestReviewResponseDiagnosticsRedactUpstreamURL(t *testing.T) {
	core, observed := observer.New(zapcore.DebugLevel)
	lg, err := glog.NewWithName("response-url-review", glog.LevelDebug, zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
	require.NoError(t, err)
	u, err := url.Parse("https://user:fixture-secret@example.com/v1/chat?credential=fixture-secret#fixture-secret")
	require.NoError(t, err)
	resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Request: &http.Request{Method: http.MethodPost, URL: u}}
	logUpstreamResponseFromBytes(lg, resp, []byte("fixture-private-body"), "test")
	capture := newLoggingReadCloser(io.NopCloser(strings.NewReader("fixture-private-body")), 100)
	_, err = io.ReadAll(capture)
	require.NoError(t, err)
	logUpstreamResponseFromCapture(lg, resp, capture, "test")
	require.Len(t, observed.All(), 2)
	for _, entry := range observed.All() {
		require.Equal(t, "https://example.com/v1/chat", entry.ContextMap()["url"])
		require.EqualValues(t, 200, entry.ContextMap()["status_code"])
		for _, field := range entry.Context {
			require.NotContains(t, field.String, "fixture-secret")
			require.NotContains(t, field.String, "fixture-private-body")
		}
	}
	require.Contains(t, resp.Request.URL.String(), "fixture-secret", "do not mutate the actual response request")
}
