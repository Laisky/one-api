package middleware

import (
	"compress/gzip"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

func GzipDecodeMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("Content-Encoding") == "gzip" {
			gzipReader, err := gzip.NewReader(c.Request.Body)
			if err != nil {
				c.AbortWithStatus(http.StatusBadRequest)
				return
			}
			defer gzipReader.Close()

			// Bound the DECOMPRESSED stream, not just the upload: ~1 MB of gzipped
			// zeros expands to ~1 GB, and downstream readers (common.GetRequestBody,
			// the JSON decoders) read the body fully into memory.
			c.Request.Body = newBoundedBody(io.NopCloser(gzipReader))
			// The body now represents decoded bytes, including during transparent redispatch.
			c.Request.Header.Del("Content-Encoding")
			c.Request.Header.Del("Content-Length")
			c.Request.ContentLength = -1
			c.Request.GetBody = nil
		}

		// Continue processing the request
		c.Next()
	}
}
