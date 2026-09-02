package middleware

import (
	"io"
	"net/http"

	errors "github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/config"
)

// ErrRequestBodyTooLarge is returned by the bounded readers this file installs
// once a request body exceeds config.MaxRequestBodySizeMB.
var ErrRequestBodyTooLarge = errors.New("request body exceeds the configured size limit")

// maxRequestBodyBytes returns the configured cap in bytes, or 0 when disabled.
//
// Return values:
//   - int64: the byte cap, 0 meaning "no limit".
func maxRequestBodyBytes() int64 {
	if config.MaxRequestBodySizeMB <= 0 {
		return 0
	}
	return int64(config.MaxRequestBodySizeMB) * 1024 * 1024
}

// boundedBody caps how many bytes a request body yields and reports a clear error
// instead of truncating, so a caller sees "too large" rather than malformed JSON.
type boundedBody struct {
	reader    io.ReadCloser
	remaining int64
}

// Read implements io.Reader.
//
// Parameters:
//   - p: destination buffer.
//
// Return values:
//   - int: bytes read.
//   - error: ErrRequestBodyTooLarge once the cap is exhausted, else the underlying error.
func (b *boundedBody) Read(p []byte) (int, error) {
	if b.remaining <= 0 {
		return 0, ErrRequestBodyTooLarge
	}
	if int64(len(p)) > b.remaining {
		p = p[:b.remaining]
	}
	n, err := b.reader.Read(p)
	b.remaining -= int64(n)
	return n, err
}

// Close implements io.Closer by closing the wrapped body.
//
// Return values:
//   - error: the underlying close error.
func (b *boundedBody) Close() error {
	return b.reader.Close()
}

// newBoundedBody wraps body with the configured cap. It returns body unchanged
// when no limit is configured.
//
// Parameters:
//   - body: the request body to bound.
//
// Return values:
//   - io.ReadCloser: a bounded body, or the original when the limit is disabled.
func newBoundedBody(body io.ReadCloser) io.ReadCloser {
	limit := maxRequestBodyBytes()
	if limit <= 0 || body == nil {
		return body
	}
	return &boundedBody{reader: body, remaining: limit}
}

// RequestBodyLimit bounds the uploaded byte count of every request it guards.
//
// It pairs with the decompressed-size bound inside GzipDecodeMiddleware: this one
// stops a huge upload, that one stops a small upload that expands into a huge
// body. Both are needed, and neither existed before — no relay route bounded the
// request body at all, so a single request could pin gigabytes of heap.
//
// Return values:
//   - gin.HandlerFunc: the middleware.
func RequestBodyLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request != nil && c.Request.Body != nil {
			if limit := maxRequestBodyBytes(); limit > 0 {
				c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
			}
		}
		c.Next()
	}
}
