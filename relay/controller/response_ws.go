package controller

import (
	"context"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
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

	// Refuse handshakes that did not pin a model. Without a bound model the
	// per-event WS guard skips enforcement (the guard's backward-compat
	// branch), which would let a client send response.create events with
	// any model and still be billed at whatever default the channel
	// resolves to. Requiring `?model=` at handshake collapses that gap so
	// billing is always pinned for the lifetime of the WS connection.
	if strings.TrimSpace(meta.ActualModelName) == "" {
		return true, openai.ErrorWrapper(
			errors.New("response websocket handshake requires a `model` query parameter so billing can be pinned"),
			"response_websocket_missing_model_query",
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

	provisionalLogId := recordProvisionalLog(c, meta, userVisibleModelName(meta, meta.ActualModelName), preConsumedQuota)
	c.Set(ctxkey.ProvisionalLogId, provisionalLogId)

	// --- Execute WS proxy ---
	bizErrResult, usage := openai.ResponseAPIWebSocketHandler(c, meta)
	if bizErrResult != nil {
		// WS proxy failed - refund pre-consumed quota. scheduleConservativeRefund
		// captures the token id on the request goroutine (value param) and marks
		// billing reconciled synchronously, so neither the spawned refund goroutine
		// nor the deferred billingAuditSafetyNet races gin's recycling of *c.
		scheduleConservativeRefund(c, preConsumedQuota, c.GetInt(ctxkey.TokenId), "ws_proxy_failed")
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
	// Capture quotaId on the request goroutine BEFORE spawning the post-billing
	// goroutine. *gin.Context is recycled via sync.Pool once this handler returns,
	// so reading c.GetInt(ctxkey.Id) from inside the goroutine would race the next
	// request's c.reset(). This mirrors response.go which also pre-captures quotaId
	// next to requestId before its postBilling GoCritical.
	quotaId := c.GetInt(ctxkey.Id)

	scheduleWebSocketPostBilling(c, lg, meta, usage, quotaId, requestId, preConsumedQuota,
		modelRatio, channelModelRatio, groupRatio, channelModelConfigs, channelCompletionRatio, ratio)

	return true, nil
}

// wsPostBillingGateForTest, when non-nil, blocks the asynchronous post-billing
// goroutine spawned by scheduleWebSocketPostBilling until the channel is closed.
// It exists solely to make the captured-vs-recycled quotaId guarantee
// deterministically testable and is always nil in production builds.
var wsPostBillingGateForTest chan struct{}

// wsPostBillingObservedQuotaIDForTest, when non-nil, receives the quotaId the
// post-billing goroutine actually uses when finalizing the request cost. Tests
// use it to assert the goroutine observed the value captured at spawn time
// rather than a value the request context was mutated to afterwards. It is
// always nil in production builds.
var wsPostBillingObservedQuotaIDForTest chan int

// scheduleWebSocketPostBilling reconciles the final WebSocket Response API cost in a
// lifecycle-managed critical goroutine so a slow DB never blocks the handler.
//
// quotaId and requestId are passed as value params captured on the request
// goroutine BEFORE this is called. *gin.Context is recycled via sync.Pool the
// instant the handler returns, so the spawned goroutine must never read
// request-scoped values off *c; doing so would race the next request's
// c.reset()/Set (concurrent map access).
func scheduleWebSocketPostBilling(
	c *gin.Context,
	lg glog.Logger,
	meta *metalib.Meta,
	usage *relaymodel.Usage,
	quotaId int,
	requestId string,
	preConsumedQuota int64,
	modelRatio float64,
	channelModelRatio map[string]float64,
	groupRatio float64,
	channelModelConfigs map[string]model.ModelConfigLocal,
	channelCompletionRatio map[string]float64,
	ratio float64,
) {
	gate := wsPostBillingGateForTest
	// detachForBilling hands the goroutine a non-cancelled, c-free context carrying a
	// snapshot of the request's billing identifiers, so postConsumeResponseAPIQuota
	// resolves request id / provisional log id / trace id from the snapshot rather than
	// off a *gin.Context that gin recycles once the handler returns. The gate + finalize
	// stay inside the work callback so the existing post-billing race tests
	// (wsPostBillingGateForTest / wsPostBillingObservedQuotaIDForTest) keep working.
	runPostBillingWithTimeout(detachForBilling(c), "postBilling", lg, postBillingTimeoutInfo{
		userID:              meta.UserId,
		channelID:           meta.ChannelId,
		model:               meta.ActualModelName,
		requestID:           requestId,
		startTime:           meta.StartTime,
		estimatedQuota:      func() float64 { return float64(usage.PromptTokens+usage.CompletionTokens) * ratio },
		guardTimeoutLog:     func() bool { return true },
		logMessage:          "CRITICAL BILLING TIMEOUT (websocket)",
		includeElapsedField: false,
	}, func(ctx context.Context) {
		quota := postConsumeResponseAPIQuota(ctx, usage, meta,
			&openai.ResponseAPIRequest{Model: meta.ActualModelName},
			preConsumedQuota, modelRatio, channelModelRatio, groupRatio,
			channelModelConfigs, channelCompletionRatio)

		if gate != nil {
			<-gate
		}
		finalizeWebSocketRequestCost(lg, quotaId, requestId, quota)
	})
}

// finalizeWebSocketRequestCost reconciles the request-cost record with the final
// billed quota. quotaId is a value param so callers capture it on the request
// goroutine, never off a recycled *gin.Context inside a background goroutine.
func finalizeWebSocketRequestCost(lg glog.Logger, quotaId int, requestId string, quota int64) {
	if wsPostBillingObservedQuotaIDForTest != nil {
		// Test observation seam: report the quotaId actually used and skip the real
		// DB write so the test stays deterministic and database-free. Always nil in
		// production builds.
		wsPostBillingObservedQuotaIDForTest <- quotaId
		return
	}
	if requestId != "" {
		if err := model.UpdateUserRequestCostQuotaByRequestID(quotaId, requestId, quota); err != nil {
			lg.Error("update user request cost failed", zap.Error(err), zap.String("request_id", requestId))
		}
	}
}
