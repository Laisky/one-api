package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

// TestGzipBombIsBounded is the regression test for an unauthenticated-shaped
// memory-exhaustion DoS: GzipDecodeMiddleware replaced the request body with a
// raw compress/gzip reader and nothing capped it, while every downstream consumer
// (common.GetRequestBody, the JSON decoders) reads the body fully into memory.
// A ~1 MB upload of compressed zeros expanded to ~1 GB of resident heap per
// request, and no relay route bounded the request body at all.
func TestGzipBombIsBounded(t *testing.T) {
	gin.SetMode(gin.TestMode)

	original := config.MaxRequestBodySizeMB
	config.MaxRequestBodySizeMB = 1
	t.Cleanup(func() { config.MaxRequestBodySizeMB = original })

	// 8 MiB of zeros compresses to a few KiB and would expand well past the 1 MiB cap.
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, err := writer.Write(make([]byte, 8*1024*1024))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	require.Less(t, compressed.Len(), 1024*1024, "the payload must be small on the wire")

	var readErr error
	var readBytes int
	router := gin.New()
	router.Use(RequestBodyLimit())
	router.Use(GzipDecodeMiddleware())
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		readBytes, readErr = len(body), err
		c.Status(http.StatusOK)
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(compressed.Bytes()))
	request.Header.Set("Content-Encoding", "gzip")
	router.ServeHTTP(httptest.NewRecorder(), request)

	require.ErrorIs(t, readErr, ErrRequestBodyTooLarge,
		"decompression must stop at the cap instead of expanding without bound")
	require.LessOrEqual(t, readBytes, 1024*1024, "no more than the cap may be buffered")
}

// TestOversizedUploadIsRejected covers the other half: a large uncompressed upload
// must be cut off by the raw-body cap.
func TestOversizedUploadIsRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)

	original := config.MaxRequestBodySizeMB
	config.MaxRequestBodySizeMB = 1
	t.Cleanup(func() { config.MaxRequestBodySizeMB = original })

	var readErr error
	router := gin.New()
	router.Use(RequestBodyLimit())
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		_, readErr = io.ReadAll(c.Request.Body)
		c.Status(http.StatusOK)
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(make([]byte, 4*1024*1024)))
	router.ServeHTTP(httptest.NewRecorder(), request)
	require.Error(t, readErr, "an upload past the cap must not be read in full")
}

// TestRequestWithinLimitIsUntouched guards against the cap breaking normal traffic.
func TestRequestWithinLimitIsUntouched(t *testing.T) {
	gin.SetMode(gin.TestMode)

	original := config.MaxRequestBodySizeMB
	config.MaxRequestBodySizeMB = 1
	t.Cleanup(func() { config.MaxRequestBodySizeMB = original })

	payload := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`)
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, err := writer.Write(payload)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	var got []byte
	var readErr error
	router := gin.New()
	router.Use(RequestBodyLimit())
	router.Use(GzipDecodeMiddleware())
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		got, readErr = io.ReadAll(c.Request.Body)
		c.Status(http.StatusOK)
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(compressed.Bytes()))
	request.Header.Set("Content-Encoding", "gzip")
	router.ServeHTTP(httptest.NewRecorder(), request)

	require.NoError(t, readErr)
	require.Equal(t, payload, got)
}
