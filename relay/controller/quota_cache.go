package controller

import (
	"context"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"

	"github.com/songquanpeng/one-api/model"
)

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
