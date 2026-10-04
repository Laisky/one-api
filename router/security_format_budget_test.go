package router

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Laisky/one-api/common/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// securityBodyReader records actual source consumption and supports deterministic read failures.
type securityBodyReader struct {
	source   io.Reader
	count    int
	terminal error
}

// Read returns source bytes and records how much unauthenticated input was consumed.
func (r *securityBodyReader) Read(p []byte) (int, error) {
	n, err := r.source.Read(p)
	r.count += n
	if err == io.EOF && r.terminal != nil {
		return n, r.terminal
	}
	return n, err
}

// Close satisfies the request-body contract without external resources.
func (r *securityBodyReader) Close() error { return nil }

// TestSecurityFormatBodyBudget exercises the shipped middleware order before token authentication.
func TestSecurityFormatBodyBudget(t *testing.T) {
	oldLimit, oldDetect, oldAction := config.MaxRequestBodySizeMB, config.AutoDetectAPIFormat, config.AutoDetectAPIFormatAction
	config.MaxRequestBodySizeMB, config.AutoDetectAPIFormat, config.AutoDetectAPIFormatAction = 1, true, "transparent"
	t.Cleanup(func() {
		config.MaxRequestBodySizeMB, config.AutoDetectAPIFormat, config.AutoDetectAPIFormatAction = oldLimit, oldDetect, oldAction
	})
	for _, tc := range []struct {
		name       string
		size       int
		compressed bool
		status     int
	}{
		{"raw-too-large", 2 << 20, false, 413}, {"chunked-too-large", 2 << 20, false, 413},
		{"gzip-too-large", 2 << 20, true, 413}, {"exact-raw-limit", 1 << 20, false, 401},
		{"exact-decoded-limit", 1 << 20, true, 401}, {"within-limit", 1024, false, 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := []byte(`{"model":"gpt-4o","input":"hello"}`)
			payload = append(payload, bytes.Repeat([]byte(" "), tc.size-len(payload))...)
			if tc.compressed {
				var b bytes.Buffer
				w := gzip.NewWriter(&b)
				_, err := w.Write(payload)
				require.NoError(t, err)
				require.NoError(t, w.Close())
				payload = b.Bytes()
			}
			reader := &securityBodyReader{source: bytes.NewReader(payload)}
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			req.Body = reader
			req.ContentLength = -1
			if tc.name != "chunked-too-large" {
				req.ContentLength = int64(len(payload))
			}
			req.Header.Set("Content-Type", "application/json")
			if tc.compressed {
				req.Header.Set("Content-Encoding", "gzip")
			}
			engine := gin.New()
			SetRelayRouter(engine)
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)
			require.Equal(t, tc.status, w.Code, w.Body.String())
			require.LessOrEqual(t, reader.count, (1<<20)+1, "raw source must be bounded before detection")
		})
	}
}

// TestSecurityCompressedFormatRedispatch ensures a decoded request is not decoded a second time.
func TestSecurityCompressedFormatRedispatch(t *testing.T) {
	oldDetect, oldAction := config.AutoDetectAPIFormat, config.AutoDetectAPIFormatAction
	config.AutoDetectAPIFormat, config.AutoDetectAPIFormatAction = true, "transparent"
	t.Cleanup(func() { config.AutoDetectAPIFormat, config.AutoDetectAPIFormatAction = oldDetect, oldAction })
	body := []byte(`{"model":"gpt-4o","input":"hello"}`)
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	_, err := gz.Write(body)
	require.NoError(t, err)
	require.NoError(t, gz.Close())
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", &b)
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Content-Type", "application/json")
	engine := gin.New()
	SetRelayRouter(engine)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
	require.Equal(t, "/v1/responses", req.URL.Path)
	require.Empty(t, req.Header.Get("Content-Encoding"))
	require.EqualValues(t, len(body), req.ContentLength)
	require.NotNil(t, req.GetBody)
	replay, err := req.GetBody()
	require.NoError(t, err)
	defer replay.Close()
	got, err := io.ReadAll(replay)
	require.NoError(t, err)
	require.Equal(t, body, got)
}

// TestSecurityFormatReadFailureStopsDispatch rejects incomplete bodies rather than continuing to auth.
func TestSecurityFormatReadFailureStopsDispatch(t *testing.T) {
	old := config.AutoDetectAPIFormat
	config.AutoDetectAPIFormat = true
	t.Cleanup(func() { config.AutoDetectAPIFormat = old })
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Body = &securityBodyReader{source: strings.NewReader(`{"input":"partial`), terminal: io.ErrUnexpectedEOF}
	engine := gin.New()
	SetRelayRouter(engine)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}
