// Package asyncvideo persists video jobs without importing any provider adaptor.
package asyncvideo

import (
	"encoding/json"
	"strings"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	dbmodel "github.com/Laisky/one-api/model"
	metalib "github.com/Laisky/one-api/relay/meta"
)

const videoTaskType = "video"

// PersistTask binds an async video-generation task id to the channel
// that created it, so follow-up status/content requests can be pinned to the
// original upstream. The input is an internal {id} envelope, not necessarily
// the provider response. Callers retain their provider-specific wire contract.
func PersistTask(c *gin.Context, body []byte) {
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
		UserUUID:      dbmodel.StringPtrIfNotEmpty(metaInfo.UserUUID),
		TokenID:       metaInfo.TokenId,
		TokenUUID:     dbmodel.StringPtrIfNotEmpty(metaInfo.TokenUUID),
		ChannelID:     metaInfo.ChannelId,
		ChannelUUID:   dbmodel.StringPtrIfNotEmpty(metaInfo.ChannelUUID),
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
