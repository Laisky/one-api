package muapi

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

const maxMuAPIVideoResponseBytes = 1 << 20

// handleVideoResponse validates and forwards MuAPI's native task envelope.
// Parameters: c carries the client method and task-binding context, and resp is
// the upstream response. Return values are nil usage plus a client-facing error.
func (a *Adaptor) handleVideoResponse(c *gin.Context, resp *http.Response) (*model.Usage, *model.ErrorWithStatusCode) {
	if resp == nil || resp.Body == nil {
		return nil, openai_compatible.ErrorWrapper(errors.New("MuAPI video response is empty"), "invalid_video_response", http.StatusBadGateway)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxMuAPIVideoResponseBytes+1))
	closeErr := resp.Body.Close()
	if err != nil {
		return nil, openai_compatible.ErrorWrapper(errors.Wrap(err, "read MuAPI video response"), "invalid_video_response", http.StatusBadGateway)
	}
	if closeErr != nil {
		return nil, openai_compatible.ErrorWrapper(errors.Wrap(closeErr, "close MuAPI video response"), "invalid_video_response", http.StatusBadGateway)
	}
	if len(body) > maxMuAPIVideoResponseBytes {
		return nil, openai_compatible.ErrorWrapper(errors.New("MuAPI video response exceeds JSON size limit"), "invalid_video_response", http.StatusBadGateway)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, openai_compatible.ErrorWrapper(errors.Errorf("MuAPI video request returned HTTP %d", resp.StatusCode), "upstream_video_error", resp.StatusCode)
	}

	var payload struct {
		RequestID string          `json:"request_id"`
		Status    string          `json:"status"`
		Error     json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, openai_compatible.ErrorWrapper(errors.Wrap(err, "decode MuAPI video response"), "invalid_video_response", http.StatusBadGateway)
	}

	creating := c != nil && c.Request != nil && c.Request.Method == http.MethodPost
	if creating {
		if !validMuAPITaskID(payload.RequestID) || hasMuAPIError(payload.Error) {
			return nil, openai_compatible.ErrorWrapper(errors.New("MuAPI video creation response has no valid request_id"), "invalid_video_response", http.StatusBadGateway)
		}
		bindingBody, err := json.Marshal(struct {
			ID string `json:"id"`
		}{ID: payload.RequestID})
		if err != nil {
			return nil, openai_compatible.ErrorWrapper(errors.Wrap(err, "encode MuAPI task binding"), "invalid_video_response", http.StatusInternalServerError)
		}
		c.Set(adaptor.AsyncVideoAcceptedKey, true)
		asyncvideo.PersistTask(c, bindingBody)
	} else if payload.Status == "" {
		return nil, openai_compatible.ErrorWrapper(errors.New("MuAPI video polling response has no status"), "invalid_video_response", http.StatusBadGateway)
	}

	if logger := gmw.GetLogger(c); logger != nil {
		logger.Debug("MuAPI video response", zap.Bool("creating", creating), zap.String("status", payload.Status), zap.Int("body_bytes", len(body)), zap.Bool("body_logging_suppressed", true))
	}
	for _, key := range []string{"Content-Type", "Retry-After", "X-Request-Id", "X-MuAPI-Cost-USD", "X-MuAPI-Cost-Credits"} {
		if value := resp.Header.Get(key); value != "" {
			c.Header(key, value)
		}
	}
	if c.Writer.Header().Get("Content-Type") == "" {
		c.Header("Content-Type", "application/json")
	}
	c.Writer.WriteHeader(resp.StatusCode)
	if _, err := c.Writer.Write(body); err != nil {
		return nil, openai_compatible.ErrorWrapper(errors.Wrap(err, "write MuAPI video response"), "write_response_body_failed", http.StatusBadGateway)
	}
	return nil, nil
}

// hasMuAPIError reports whether an optional error field contains a non-null
// value. Parameters: raw is the provider's JSON error field. Return value is
// true when the field indicates a failed creation.
func hasMuAPIError(raw json.RawMessage) bool {
	trimmed := string(raw)
	return len(raw) > 0 && trimmed != "" && trimmed != "null"
}
