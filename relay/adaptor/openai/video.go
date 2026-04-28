package openai

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	dbmodel "github.com/Laisky/one-api/model"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

const maxLoggedVideoBytes = 64 * 1024
const videoTaskType = "video"

// VideoHandler forwards OpenAI video responses (JSON job metadata or binary content) unchanged to the caller.
// It logs the upstream payload for diagnostics and surfaces provider errors without altering the body.
func VideoHandler(c *gin.Context, resp *http.Response) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
	logger := gmw.GetLogger(c)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}
	if err = resp.Body.Close(); err != nil {
		return ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}

	logFields := []zap.Field{zap.Int("body_bytes", len(body))}
	if len(body) == 0 {
		logger.Debug("video handler upstream response empty", logFields...)
	} else if len(body) <= maxLoggedVideoBytes {
		logFields = append(logFields, zap.ByteString("body", body))
		logger.Debug("video handler upstream response", logFields...)
	} else {
		logFields = append(logFields, zap.ByteString("body_preview", body[:maxLoggedVideoBytes]))
		logger.Debug("video handler upstream response truncated", logFields...)
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
		persistAsyncVideoTask(c, body)
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

func persistAsyncVideoTask(c *gin.Context, body []byte) {
	if c == nil || len(body) == 0 {
		return
	}
	metaInfo := metalib.GetByContext(c)
	if metaInfo == nil || metaInfo.ChannelId == 0 || metaInfo.ChannelType == 0 || metaInfo.UserId == 0 {
		return
	}
	var payload struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		if logger := gmw.GetLogger(c); logger != nil {
			logger.Debug("skip async task binding persistence - unable to parse id",
				zap.Error(err))
		}
		return
	}
	taskID := strings.TrimSpace(payload.ID)
	if taskID == "" {
		return
	}

	var snapshot map[string]any
	if raw, ok := c.Get(ctxkey.AsyncTaskRequestMetadata); ok {
		if typed, ok := raw.(map[string]any); ok {
			snapshot = typed
		}
	}
	requestJSON, err := dbmodel.MarshalRequestMetadata(snapshot)
	if err != nil {
		if logger := gmw.GetLogger(c); logger != nil {
			logger.Warn("failed to marshal async task snapshot", zap.Error(err))
		}
		requestJSON = ""
	}

	requestPath := ""
	if c.Request != nil && c.Request.URL != nil {
		requestPath = c.Request.URL.Path
	}

	binding := &dbmodel.AsyncTaskBinding{
		TaskID:        taskID,
		TaskType:      videoTaskType,
		UserID:        metaInfo.UserId,
		TokenID:       metaInfo.TokenId,
		ChannelID:     metaInfo.ChannelId,
		ChannelType:   metaInfo.ChannelType,
		OriginModel:   metaInfo.OriginModelName,
		ActualModel:   metaInfo.ActualModelName,
		RequestMethod: c.Request.Method,
		RequestPath:   requestPath,
		RequestParams: requestJSON,
	}

	if err := dbmodel.SaveAsyncTaskBinding(gmw.Ctx(c), binding); err != nil {
		if logger := gmw.GetLogger(c); logger != nil {
			logger.Warn("persist async task binding failed", zap.Error(err), zap.String("task_id", taskID))
		}
	}
}
