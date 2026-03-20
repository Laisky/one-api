package controller

import (
	"context"
	"net/http"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/graceful"
	"github.com/songquanpeng/one-api/common/metrics"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/channeltype"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/pricing"
)

// maybeHandleResponseAPIWebSocket handles websocket upgrades for /v1/responses.
// When the client connects via WebSocket, this function handles the full lifecycle
// including pre-consume quota, WS proxy, and post-billing reconciliation.
//
// Parameters:
//   - c: request context.
//   - meta: relay metadata resolved from middleware.
//
// Returns:
//   - bool: true when this request was a websocket upgrade and has been handled.
//   - *relaymodel.ErrorWithStatusCode: business error when websocket handling fails.
func maybeHandleResponseAPIWebSocket(c *gin.Context, meta *metalib.Meta) (bool, *relaymodel.ErrorWithStatusCode) {
	if !websocket.IsWebSocketUpgrade(c.Request) {
		return false, nil
	}

	if meta == nil {
		return true, openai.ErrorWrapper(errors.New("missing relay meta"), "invalid_meta", http.StatusBadRequest)
	}

	if meta.ChannelType != channeltype.OpenAI {
		return true, openai.ErrorWrapper(
			errors.New("response websocket is only supported for OpenAI channels"),
			"response_websocket_only_supported_for_openai_channel",
			http.StatusBadRequest,
		)
	}

	if !supportsNativeResponseAPI(meta) {
		return true, openai.ErrorWrapper(
			errors.New("response websocket is not supported for this channel"),
			"response_websocket_not_supported_for_channel",
			http.StatusBadRequest,
		)
	}

	lg := gmw.GetLogger(c)

	// --- Pre-consume quota ---
	// For WebSocket, we don't have the request body yet (it arrives as a WS message),
	// so we estimate based on a conservative default. The post-billing will reconcile.
	channelModelRatio, channelCompletionRatio := getChannelRatios(c)
	channelModelConfigs := getChannelModelConfigs(c)
	pricingAdaptor := resolvePricingAdaptor(meta)
	modelRatio := pricing.GetModelRatioWithThreeLayers(meta.ActualModelName, channelModelRatio, pricingAdaptor)
	completionRatio := pricing.GetCompletionRatioWithThreeLayers(meta.ActualModelName, channelCompletionRatio, pricingAdaptor)
	groupRatio := c.GetFloat64(ctxkey.ChannelRatio)
	ratio := modelRatio * groupRatio
	outputRatio := ratio * completionRatio

	// Estimate a conservative pre-consume amount based on typical WS request size
	estimatedPromptTokens := config.PreconsumeTokenForBackgroundRequest
	if estimatedPromptTokens <= 0 {
		estimatedPromptTokens = 2000
	}
	meta.PromptTokens = estimatedPromptTokens
	preConsumedQuota, bizErr := preConsumeResponseAPIQuota(c, &openai.ResponseAPIRequest{
		Model: meta.ActualModelName,
	}, estimatedPromptTokens, ratio, outputRatio, false, meta)
	if bizErr != nil {
		lg.Warn("preConsumeResponseAPIQuota failed for websocket",
			zap.Error(bizErr.RawError),
			zap.Int("status_code", bizErr.StatusCode))
		return true, bizErr
	}
	markPreConsumed(c, preConsumedQuota)
	defer billingAuditSafetyNet(c)

	provisionalLogId := recordProvisionalLog(c, meta, meta.ActualModelName, preConsumedQuota)
	c.Set(ctxkey.ProvisionalLogId, provisionalLogId)

	// --- Execute WS proxy ---
	bizErrResult, usage := openai.ResponseAPIWebSocketHandler(c, meta)
	if bizErrResult != nil {
		// WS proxy failed - refund pre-consumed quota
		markBillingReconciled(c)
		graceful.GoCritical(gmw.BackgroundCtx(c), "returnPreConsumedQuota", func(ctx context.Context) {
			_ = returnPreConsumedQuotaConservative(ctx, c, preConsumedQuota, c.GetInt(ctxkey.TokenId), "ws_proxy_failed")
		})
		return true, bizErrResult
	}

	if usage == nil {
		usage = &relaymodel.Usage{}
	}

	lg.Debug("response api websocket completed with usage",
		zap.Int("prompt_tokens", usage.PromptTokens),
		zap.Int("completion_tokens", usage.CompletionTokens),
		zap.Int("total_tokens", usage.TotalTokens),
		zap.Int("user_id", meta.UserId),
		zap.String("model", meta.ActualModelName),
	)

	// --- Post-billing ---
	//
	// !! ZERO-USAGE GUARD !!
	//
	// OpenAI's WebSocket Response API does NOT reliably include usage
	// (token counts) in its streaming events. When usage is absent,
	// PromptTokens and CompletionTokens remain zero.
	//
	// If we were to proceed with post-billing using zero usage, the quota
	// calculation would produce quota=0, resulting in:
	//   quotaDelta = 0 - preConsumedQuota = -preConsumedQuota (negative)
	// This negative delta would REFUND the pre-consumed amount, making
	// the entire upstream request FREE — which is incorrect.
	//
	// Therefore: when usage is zero, we SKIP post-billing entirely and
	// let the pre-consumed quota stand as the final charge. The provisional
	// log entry remains with the estimated amount for audit visibility.
	//
	if usage.PromptTokens == 0 && usage.CompletionTokens == 0 {
		lg.Warn("response api websocket returned zero usage, keeping pre-consumed quota as-is",
			zap.Int64("pre_consumed_quota", preConsumedQuota),
			zap.Int("user_id", meta.UserId),
			zap.String("model", meta.ActualModelName),
			zap.String("request_id", c.GetString(ctxkey.RequestId)),
		)
		markBillingReconciled(c)
		return true, nil
	}

	markBillingReconciled(c)
	requestId := c.GetString(ctxkey.RequestId)

	graceful.GoCritical(gmw.BackgroundCtx(c), "postBilling", func(ctx context.Context) {
		billingTimeout := time.Duration(config.BillingTimeoutSec) * time.Second
		ctx, cancel := context.WithTimeout(gmw.BackgroundCtx(c), billingTimeout)
		defer cancel()

		done := make(chan bool, 1)
		var quota int64

		go func() {
			quota = postConsumeResponseAPIQuota(ctx, usage, meta,
				&openai.ResponseAPIRequest{Model: meta.ActualModelName},
				preConsumedQuota, modelRatio, channelModelRatio, groupRatio,
				channelModelConfigs, channelCompletionRatio)

			quotaId := c.GetInt(ctxkey.Id)
			if requestId != "" {
				if err := model.UpdateUserRequestCostQuotaByRequestID(quotaId, requestId, quota); err != nil {
					lg.Error("update user request cost failed", zap.Error(err), zap.String("request_id", requestId))
				}
			}
			done <- true
		}()

		select {
		case <-done:
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				estimatedQuota := float64(usage.PromptTokens+usage.CompletionTokens) * ratio
				lg.Error("CRITICAL BILLING TIMEOUT (websocket)",
					zap.String("model", meta.ActualModelName),
					zap.String("requestId", requestId),
					zap.Int("userId", meta.UserId),
					zap.Int64("estimatedQuota", int64(estimatedQuota)),
				)
				metrics.GlobalRecorder.RecordBillingTimeout(meta.UserId, meta.ChannelId, meta.ActualModelName, estimatedQuota, time.Since(meta.StartTime))
			}
		}
	})

	return true, nil
}
