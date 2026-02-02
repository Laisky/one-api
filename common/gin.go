package common

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/ctxkey"
)

// GetRequestBody reads and caches the request body so it can be reused later in the handler chain.
// It returns the raw body bytes and wraps any I/O error encountered during the read.
func GetRequestBody(c *gin.Context) (requestBody []byte, err error) {
	if requestBodyCache, _ := c.Get(ctxkey.KeyRequestBody); requestBodyCache != nil {
		return requestBodyCache.([]byte), nil
	}
	requestBody, err = io.ReadAll(c.Request.Body)
	if err != nil {
		return nil, errors.Wrap(err, "read request body failed")
	}
	_ = c.Request.Body.Close()
	c.Set(ctxkey.KeyRequestBody, requestBody)

	return requestBody, nil
}

// UnmarshalBodyReusable unmarshals the request body into the provided pointer while keeping the body reusable.
// It supports JSON and form payloads based on the Content-Type header.
func UnmarshalBodyReusable(c *gin.Context, v any) error {
	requestBody, err := GetRequestBody(c)
	if err != nil {
		return errors.Wrap(err, "get request body failed")
	}

	logger := gmw.GetLogger(c)
	if _, ok := c.Get(ctxkey.RequestModel); !ok {
		preview, truncated := SanitizePayloadForLogging(requestBody, DefaultLogBodyLimit)
		logger.Debug("receive user request",
			zap.String("method", c.Request.Method),
			zap.Int("body_bytes", len(requestBody)),
			zap.Bool("body_truncated", truncated),
			zap.ByteString("request", preview))
	}

	// check v should be a pointer
	if v == nil || reflect.TypeOf(v).Kind() != reflect.Ptr {
		return errors.Errorf("UnmarshalBodyReusable only accept pointer, got %v", reflect.TypeOf(v))
	}

	contentType := c.Request.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "application/json") {
		err = json.Unmarshal(requestBody, v)
	} else {
		c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		err = c.ShouldBind(v)
	}
	if err != nil {
		return errors.Wrap(err, "unmarshal request body failed")
	}

	// Reset request body
	c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
	return nil
}

// SetEventStreamHeaders configures the standard headers required for server-sent event responses.
func SetEventStreamHeaders(c *gin.Context) {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Header().Set("Pragma", "no-cache") // This is for legacy HTTP; I'm pretty sure.
}
