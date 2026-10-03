package router

import (
	"bytes"
	"compress/gzip"
	"github.com/Laisky/one-api/common/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// securityCountingBody records bytes consumed by middleware before authentication.
type securityCountingBody struct {
	io.Reader
	read int
}

// Read tracks consumed bytes while delegating to the fixture reader.
func (b *securityCountingBody) Read(p []byte) (int, error) {
	n, e := b.Reader.Read(p)
	b.read += n
	return n, e
}

// Close releases the in-memory fixture body.
func (b *securityCountingBody) Close() error { return nil }

// TestSecurityRelayDetectionBodyBound verifies the real route caps upload before format detection.
func TestSecurityRelayDetectionBodyBound(t *testing.T) {
	old := config.MaxRequestBodySizeMB
	auto := config.AutoDetectAPIFormat
	config.MaxRequestBodySizeMB = 1
	config.AutoDetectAPIFormat = true
	t.Cleanup(func() { config.MaxRequestBodySizeMB = old; config.AutoDetectAPIFormat = auto })
	r := gin.New()
	SetRelayRouter(r)
	body := &securityCountingBody{Reader: strings.NewReader(strings.Repeat(" ", 2*1024*1024))}
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req.Body = body
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.LessOrEqual(t, body.read, 1024*1024+1)
	require.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
}

// TestSecurityChannelMutationsRejectGET verifies navigation cannot invoke paid probes or mutate balances.
func TestSecurityChannelMutationsRejectGET(t *testing.T) {
	r := gin.New()
	SetApiRouter(r)
	for _, route := range r.Routes() {
		if strings.Contains(route.Path, "/channel/test") || strings.Contains(route.Path, "/channel/update_balance") {
			require.Equal(t, http.MethodPost, route.Method, route.Path)
		}
	}
}

// TestSecurityRelayCompressedDetectionBound verifies expanded body limits apply before automatic detection.
func TestSecurityRelayCompressedDetectionBound(t *testing.T) {
	old := config.MaxRequestBodySizeMB
	auto := config.AutoDetectAPIFormat
	config.MaxRequestBodySizeMB = 1
	config.AutoDetectAPIFormat = true
	t.Cleanup(func() { config.MaxRequestBodySizeMB = old; config.AutoDetectAPIFormat = auto })
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, err := writer.Write([]byte(strings.Repeat(" ", 2*1024*1024)))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	r := gin.New()
	SetRelayRouter(r)
	for _, path := range []string{"/v1/chat/completions", "/v1/responses", "/v1/messages"} {
		req := httptest.NewRequest("POST", path, bytes.NewReader(compressed.Bytes()))
		req.Header.Set("Content-Encoding", "gzip")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, 413, w.Code, path)
	}
}
