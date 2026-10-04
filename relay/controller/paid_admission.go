package controller

import (
	"net/http"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// reservePaidRequestQuota atomically reserves a validated quote against the durable
// owner and finite-token balances. Cached balances may reject work early, but
// never authorize spending or replace the shared database reservation. A zero
// quote remains free; failures return no held quota and an API error.
func reservePaidRequestQuota(c *gin.Context, meta *metalib.Meta, quote int64, stage string) (int64, *relaymodel.ErrorWithStatusCode) {
	if quote < 0 {
		return 0, openai.ErrorWrapper(errors.New("request quota must be nonnegative"), "invalid_request_quota", http.StatusBadRequest)
	}
	if quote == 0 {
		return 0, nil
	}
	ctx := gmw.Ctx(c)
	userQuota, err := model.CacheGetUserQuota(ctx, meta.UserId)
	if err != nil {
		return 0, openai.ErrorWrapper(err, "get_user_quota_failed", http.StatusInternalServerError)
	}
	if userQuota < quote {
		return 0, openai.ErrorWrapper(errors.New("user quota is not enough"), "insufficient_user_quota", http.StatusForbidden)
	}
	if err := model.PreConsumeTokenQuota(ctx, meta.TokenId, quote); err != nil {
		return 0, openai.ErrorWrapper(err, "pre_consume_token_quota_failed", http.StatusForbidden)
	}
	syncUserQuotaCacheAfterPreConsume(ctx, meta.UserId, quote, stage)
	lg := gmw.GetLogger(c)
	lg.Debug("reserved durable request quota", zap.Int64("quota", quote), zap.String("stage", stage))
	return quote, nil
}
