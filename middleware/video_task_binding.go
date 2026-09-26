package middleware

import (
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/asyncvideo"
)

// BindAsyncTaskChannel resolves asynchronous task metadata (e.g., video jobs) before channel distribution.
// When a task id is present without an explicit model, this middleware pins the request to the original channel.
func BindAsyncTaskChannel() gin.HandlerFunc {
	return func(c *gin.Context) {
		req := c.Request
		if req == nil {
			c.Next()
			return
		}

		method := req.Method
		if method != http.MethodGet && method != http.MethodDelete && method != http.MethodPost {
			c.Next()
			return
		}

		path := req.URL.Path
		if !strings.HasPrefix(path, "/v1/videos/") {
			c.Next()
			return
		}

		// POST /v1/videos/<id>/remix should still supply a model; existing flow covers it.
		// We only need binding for retrieval endpoints containing a video id parameter.
		videoID := strings.TrimSpace(c.Param("video_id"))
		if videoID == "" {
			c.Next()
			return
		}

		lg := gmw.GetLogger(c)
		if _, retryErr := asyncvideo.RetryPendingTaskBinding(gmw.Ctx(c), videoID, c.GetInt(ctxkey.Id)); retryErr != nil {
			AbortWithError(c, http.StatusInternalServerError, errors.Wrap(retryErr, "recover video task binding"))
			return
		}
		binding, err := model.GetAsyncTaskBindingByTaskID(gmw.Ctx(c), videoID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				AbortWithError(c, http.StatusNotFound, errors.New("video task not found"))
			} else {
				AbortWithError(c, http.StatusInternalServerError, errors.Wrap(err, "resolve video task"))
			}
			return
		}
		if binding.UserID <= 0 || binding.UserID != c.GetInt(ctxkey.Id) ||
			(binding.UserUUID != nil && *binding.UserUUID != "" && *binding.UserUUID != c.GetString(ctxkey.UserUUID)) {
			AbortWithError(c, http.StatusNotFound, errors.New("video task not found"))
			return
		}
		boundModel := strings.TrimSpace(binding.OriginModel)
		if boundModel == "" {
			boundModel = strings.TrimSpace(binding.ActualModel)
		}
		if allowed := c.GetString(ctxkey.AvailableModels); allowed != "" && !IsModelInList(boundModel, allowed) {
			AbortWithError(c, http.StatusForbidden, errors.New("token does not allow the video task model"))
			return
		}
		if binding.ChannelID <= 0 || boundModel == "" {
			AbortWithError(c, http.StatusNotFound, errors.New("video task not found"))
			return
		}

		// Update access time but do not fail the request if the touch operation encounters a transient error.
		if touchErr := model.TouchAsyncTaskBinding(gmw.Ctx(c), videoID); touchErr != nil && lg != nil {
			if !errors.Is(touchErr, gorm.ErrRecordNotFound) {
				lg.Debug("async task binding touch failed", zap.String("task_id", videoID), zap.Error(touchErr))
			}
		}

		if binding.ChannelID > 0 {
			c.Set(ctxkey.SpecificChannelId, binding.ChannelID)
		}
		if trimmed := strings.TrimSpace(binding.OriginModel); trimmed != "" {
			c.Set(ctxkey.RequestModel, trimmed)
		} else if trimmed := strings.TrimSpace(binding.ActualModel); trimmed != "" {
			c.Set(ctxkey.RequestModel, trimmed)
		}

		if lg != nil {
			lg.Debug("async task binding resolved",
				zap.String("task_id", videoID),
				zap.String("task_type", binding.TaskType),
				zap.Int("channel_id", binding.ChannelID),
				zap.String("model", c.GetString(ctxkey.RequestModel)))
		}

		c.Next()
	}
}
