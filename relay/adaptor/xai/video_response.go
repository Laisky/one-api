package xai

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/asyncvideo"
	"github.com/Laisky/one-api/relay/model"
)

const maxVideoResponseBytes = 1 << 20

// handleVideoResponse preserves native creation/status JSON, binds request_id
// using the shared task store, and reports failed HTTP/envelope responses to the
// billing controller. It never treats video payloads as chat completions.
func (a *Adaptor) handleVideoResponse(c *gin.Context, resp *http.Response) (*model.Usage, *model.ErrorWithStatusCode) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxVideoResponseBytes+1))
	closeErr := resp.Body.Close()
	if err != nil {
		return nil, openai_compatible.ErrorWrapper(errors.Wrap(err, "read xAI video response"), "invalid_video_response", http.StatusBadGateway)
	}
	if closeErr != nil {
		return nil, openai_compatible.ErrorWrapper(errors.Wrap(closeErr, "close xAI video response"), "invalid_video_response", http.StatusBadGateway)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Do not echo arbitrary upstream bodies: they may contain credentials or
		// reflected input. HTTP status still distinguishes auth/quota failures.
		return nil, openai_compatible.ErrorWrapper(errors.Errorf("xAI video request returned HTTP %d", resp.StatusCode), "upstream_video_error", resp.StatusCode)
	}
	if len(body) > maxVideoResponseBytes {
		return nil, openai_compatible.ErrorWrapper(errors.New("xAI video response exceeds JSON size limit"), "invalid_video_response", http.StatusBadGateway)
	}
	var payload struct {
		RequestID string          `json:"request_id"`
		Status    string          `json:"status"`
		Error     json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, openai_compatible.ErrorWrapper(errors.Wrap(err, "decode xAI video response"), "invalid_video_response", http.StatusBadGateway)
	}
	creating := c.Request.Method == http.MethodPost
	if creating {
		if !validVideoTaskID(payload.RequestID) || (len(payload.Error) > 0 && string(payload.Error) != "null") {
			return nil, openai_compatible.ErrorWrapper(errors.New("xAI video creation response has no valid request_id"), "invalid_video_response", http.StatusBadGateway)
		}
		// This is an internal binding envelope only. The client's native response
		// is never rewritten to an OpenAI id/status schema.
		bindingBody, err := json.Marshal(struct {
			ID string `json:"id"`
		}{ID: payload.RequestID})
		if err != nil {
			return nil, openai_compatible.ErrorWrapper(errors.Wrap(err, "encode xAI task binding"), "invalid_video_response", http.StatusInternalServerError)
		}
		c.Set(adaptor.AsyncVideoAcceptedKey, true)
		asyncvideo.PersistTask(c, bindingBody)
	} else if payload.Status == "" {
		return nil, openai_compatible.ErrorWrapper(errors.New("xAI video polling response has no status"), "invalid_video_response", http.StatusBadGateway)
	}
	if lg := gmw.GetLogger(c); lg != nil {
		lg.Debug("xAI video response", zap.Bool("creating", creating), zap.String("status", payload.Status), zap.Int("body_bytes", len(body)), zap.Bool("body_logging_suppressed", true))
	}
	// Copy representation-independent headers only. The body was read in full;
	// upstream Content-Length/transfer/connection headers must not leak through.
	for _, key := range []string{"Content-Type", "Retry-After", "X-Request-Id"} {
		if value := resp.Header.Get(key); value != "" {
			c.Header(key, value)
		}
	}
	if c.Writer.Header().Get("Content-Type") == "" {
		c.Header("Content-Type", "application/json")
	}
	c.Writer.WriteHeader(resp.StatusCode)
	if _, err := c.Writer.Write(body); err != nil {
		return nil, openai_compatible.ErrorWrapper(errors.Wrap(err, "write xAI video response"), "write_response_body_failed", http.StatusBadGateway)
	}
	return nil, nil
}
