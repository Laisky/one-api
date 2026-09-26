package muapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

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
		if err := asyncvideo.PersistTask(c, bindingBody); err != nil {
			if logger := gmw.GetLogger(c); logger != nil {
				logger.Warn("MuAPI async video task binding persistence failed", zap.Error(err), zap.String("task_id", payload.RequestID))
			}
		}
	} else if payload.Status == "" {
		return nil, openai_compatible.ErrorWrapper(errors.New("MuAPI video polling response has no status"), "invalid_video_response", http.StatusBadGateway)
	}

	if logger := gmw.GetLogger(c); logger != nil {
		logger.Debug("MuAPI video response", zap.Bool("creating", creating), zap.String("status", payload.Status), zap.Int("body_bytes", len(body)), zap.Bool("body_logging_suppressed", true))
	}
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
		return nil, openai_compatible.ErrorWrapper(errors.Wrap(err, "write MuAPI video response"), "write_response_body_failed", http.StatusBadGateway)
	}
	return nil, nil
}

// hasMuAPIError reports whether an optional error field contains a meaningful
// error. Parameters: raw is the provider's JSON error field. Return value is
// true when the field indicates a failed creation; missing, null, empty, and
// whitespace-only strings are non-errors, while other JSON values remain errors.
func hasMuAPIError(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return false
	}

	var message string
	if err := json.Unmarshal(trimmed, &message); err == nil {
		return strings.TrimSpace(message) != ""
	}
	return true
}
