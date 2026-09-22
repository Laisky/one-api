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
)

// BindAsyncTaskChannel authorizes video task access before channel distribution.
// Parameters: none. Returns: middleware that pins an owned task to its original
// channel and rejects missing or foreign tasks without disclosing their metadata.
func BindAsyncTaskChannel() gin.HandlerFunc {
	return func(c *gin.Context) {
		req := c.Request
		if req == nil || req.URL == nil {
			c.Next()
			return
		}
		if req.Method != http.MethodGet && req.Method != http.MethodDelete && req.Method != http.MethodPost {
			c.Next()
			return
		}
		if !strings.HasPrefix(req.URL.Path, "/v1/videos/") {
			c.Next()
			return
		}
		videoID := strings.TrimSpace(c.Param("video_id"))
		if videoID == "" {
			// Creation routes have no video_id and do not need an existing task.
			c.Next()
			return
		}

		lg := gmw.GetLogger(c)
		binding, err := model.GetAsyncTaskBindingByTaskID(gmw.Ctx(c), videoID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": gin.H{"type": "invalid_request_error", "message": "Video task not found"}})
				return
			}
			lg.Warn("async task binding lookup failed", zap.Error(err))
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"type": "server_error", "message": "Video task lookup unavailable"}})
			return
		}
		userID := c.GetInt(ctxkey.Id)
		if binding == nil || userID <= 0 || binding.UserID != userID || binding.TaskType != "video" {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": gin.H{"type": "invalid_request_error", "message": "Video task not found"}})
			return
		}

		// Only an authorized caller may touch retention metadata or select a channel.
		if touchErr := model.TouchAsyncTaskBinding(gmw.Ctx(c), videoID); touchErr != nil {
			lg.Debug("async task binding touch failed", zap.Error(touchErr))
		}
		if binding.ChannelID > 0 {
			c.Set(ctxkey.SpecificChannelId, binding.ChannelID)
		}
		if trimmed := strings.TrimSpace(binding.OriginModel); trimmed != "" {
			c.Set(ctxkey.RequestModel, trimmed)
		} else if trimmed := strings.TrimSpace(binding.ActualModel); trimmed != "" {
			c.Set(ctxkey.RequestModel, trimmed)
		}
		lg.Debug("async task binding resolved",
			zap.String("task_id", videoID),
			zap.String("task_type", binding.TaskType),
			zap.Int("channel_id", binding.ChannelID))
		c.Next()
	}
}
