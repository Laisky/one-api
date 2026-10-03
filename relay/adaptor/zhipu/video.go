package zhipu

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/asyncvideo"
	"github.com/Laisky/one-api/relay/model"
)

// VideoHandler validates native task envelopes, pins accepted jobs and preserves
// their JSON. An accepted creation remains billable after a client disconnect;
// polling does not reserve or consume generation quota.
func VideoHandler(c *gin.Context, resp *http.Response) (*model.ErrorWithStatusCode, *model.Usage) {
	const limit = 1 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	closeErr := resp.Body.Close()
	if err != nil {
		return openai.ErrorWrapper(errors.Wrap(err, "read video response"), "invalid_video_response", http.StatusBadGateway), nil
	}
	if closeErr != nil {
		return openai.ErrorWrapper(errors.Wrap(closeErr, "close video response"), "invalid_video_response", http.StatusBadGateway), nil
	}
	for _, key := range []string{"Retry-After", "X-Request-Id"} {
		if value := resp.Header.Get(key); value != "" {
			c.Header(key, value)
		}
	}
	if len(body) > limit {
		return openai.ErrorWrapper(errors.New("video response exceeds limit"), "invalid_video_response", http.StatusBadGateway), nil
	}
	// Preserve the provider's actionable code/message for both HTTP failures
	// and error envelopes carried by an otherwise successful HTTP response.
	var providerError struct {
		Error *struct {
			Code    json.RawMessage `json:"code"`
			Message string          `json:"message"`
		} `json:"error"`
	}
	if decodeErr := json.Unmarshal(body, &providerError); decodeErr == nil && providerError.Error != nil && providerError.Error.Message != "" {
		status := resp.StatusCode
		if status >= 200 && status < 300 {
			status = http.StatusBadGateway
		}
		code := string(providerError.Error.Code)
		var codeText string
		if codeErr := json.Unmarshal(providerError.Error.Code, &codeText); codeErr == nil {
			code = codeText
		}
		return &model.ErrorWithStatusCode{Error: model.Error{Message: providerError.Error.Message, Type: model.ErrorTypeZhipu, Code: code}, StatusCode: status}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return openai.ErrorWrapper(errors.Errorf("BigModel video upstream returned HTTP %d", resp.StatusCode), "upstream_video_error", resp.StatusCode), nil
	}
	var payload struct {
		ID         string          `json:"id"`
		TaskStatus string          `json:"task_status"`
		Error      json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return openai.ErrorWrapper(errors.Wrap(err, "decode video response"), "invalid_video_response", http.StatusBadGateway), nil
	}
	if len(payload.Error) != 0 && string(payload.Error) != "null" {
		return openai.ErrorWrapper(errors.New("BigModel returned a video error envelope"), "upstream_video_error", http.StatusBadGateway), nil
	}
	creating := c.Request.Method == http.MethodPost
	if payload.TaskStatus != "PROCESSING" && payload.TaskStatus != "SUCCESS" && payload.TaskStatus != "FAIL" {
		return openai.ErrorWrapper(errors.New("unknown native video task status"), "invalid_video_response", http.StatusBadGateway), nil
	}
	if creating {
		if !validVideoID(payload.ID) || payload.TaskStatus == "FAIL" {
			return openai.ErrorWrapper(errors.New("video creation was not accepted"), "invalid_video_response", http.StatusBadGateway), nil
		}
		c.Set(adaptor.AsyncVideoAcceptedKey, true)
		asyncvideo.PersistTask(c, body)
	}
	if lg := gmw.GetLogger(c); lg != nil {
		lg.Debug("BigModel video response", zap.Bool("creating", creating), zap.String("status", payload.TaskStatus), zap.Int("body_bytes", len(body)))
	}
	c.Header("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	if _, err := c.Writer.Write(body); err != nil {
		return openai.ErrorWrapper(errors.Wrap(err, "write video response"), "write_response_body_failed", http.StatusBadGateway), nil
	}
	return nil, nil
}
