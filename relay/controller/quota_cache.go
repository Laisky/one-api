package controller

import (
	"context"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/model"
)

// getUserQuotaFromContext returns the user's quota from the UserObj stored in
// the gin context. Returns 0 if the user object is not present.
func getUserQuotaFromContext(c *gin.Context) int64 {
	if userObj, exists := c.Get(ctxkey.UserObj); exists {
		if u, ok := userObj.(*model.User); ok {
			return u.Quota
		}
	}
	return 0
}

// getUserObjFromContext returns the *model.User stored in the gin context, or nil.
func getUserObjFromContext(c *gin.Context) *model.User {
	if userObj, exists := c.Get(ctxkey.UserObj); exists {
		if u, ok := userObj.(*model.User); ok {
			return u
		}
	}
	return nil
}

// syncUserQuotaCacheAfterPreConsume best-effort synchronizes cached user quota after
// a successful database pre-consume operation.
func syncUserQuotaCacheAfterPreConsume(ctx context.Context, userID int, quota int64, source string) {
	if userID <= 0 || quota <= 0 {
		return
	}
	if err := model.CacheDecreaseUserQuota(ctx, userID, quota); err != nil {
		gmw.GetLogger(ctx).Warn("failed to sync user quota cache after pre-consume",
			zap.Int("user_id", userID),
			zap.Int64("quota", quota),
			zap.String("source", source),
			zap.Error(err),
		)
	}
}
