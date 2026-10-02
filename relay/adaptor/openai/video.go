package openai

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/asyncvideo"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// VideoHandler forwards OpenAI video responses (JSON job metadata or binary content) unchanged to the caller.
// It logs only payload shape metadata and surfaces provider errors without altering the body.
func VideoHandler(c *gin.Context, resp *http.Response) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
	logger := gmw.GetLogger(c)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}
	if err = resp.Body.Close(); err != nil {
		return ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}

	logFields := []zap.Field{
		zap.Int("body_bytes", len(body)),
		zap.Bool("body_logging_suppressed", true),
	}
	if len(body) == 0 {
		logger.Debug("video handler upstream response empty", logFields...)
	} else {
		logger.Debug("video handler upstream response", logFields...)
	}

	var maybeError struct {
		Error *relaymodel.Error `json:"error,omitempty"`
	}
	if len(body) > 0 {
		if unmarshalErr := json.Unmarshal(body, &maybeError); unmarshalErr == nil {
			if maybeError.Error != nil && maybeError.Error.Type != "" {
				maybeError.Error.RawError = nil
				return &relaymodel.ErrorWithStatusCode{
					Error:      *maybeError.Error,
					StatusCode: resp.StatusCode,
				}, nil
			}
		}
	}

	if resp.StatusCode < http.StatusBadRequest && c.Request.Method == http.MethodPost {
		PersistAsyncVideoTask(c, body)
	}

	resp.Body = io.NopCloser(bytes.NewReader(body))

	for k, values := range resp.Header {
		for _, v := range values {
			c.Writer.Header().Add(k, v)
		}
	}

	c.Writer.WriteHeader(resp.StatusCode)
	if _, err = io.Copy(c.Writer, resp.Body); err != nil {
		return ErrorWrapper(err, "copy_response_body_failed", http.StatusInternalServerError), nil
	}
	if err = resp.Body.Close(); err != nil {
		return ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}

	return nil, nil
}

// PersistAsyncVideoTask preserves the shared task-binding entrypoint used by
// OpenAI and compatible providers without coupling provider implementations.
func PersistAsyncVideoTask(c *gin.Context, body []byte) {
	asyncvideo.PersistTask(c, body)
}
