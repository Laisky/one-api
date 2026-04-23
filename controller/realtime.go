package controller

import (
	"context"
	"net/http"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/graceful"
	"github.com/songquanpeng/one-api/common/tracing"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay"
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/apitype"
	"github.com/songquanpeng/one-api/relay/billing"
	"github.com/songquanpeng/one-api/relay/meta"
	rmodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/pricing"
	quotautil "github.com/songquanpeng/one-api/relay/quota"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

// Realtime session preConsume estimation constants.
// Since we can't know session length upfront (live audio streaming), we estimate
// a conservative minimum charge based on a short audio conversation.
const (
	// realtimePreConsumeSeconds is the estimated session duration (seconds) for
	// pre-consuming quota. 120s (2 minutes) is a conservative minimum.
	realtimePreConsumeSeconds = 120

	// Audio token rates per OpenAI docs:
	// - Input:  1 token per 100ms = 10 tokens/second
	// - Output: 1 token per 50ms  = 20 tokens/second
	realtimeAudioInputTokensPerSec  = 10
	realtimeAudioOutputTokensPerSec = 20
)

// RelayRealtime handles WebSocket Realtime proxying for OpenAI Realtime API.
//
// Billing flow (mirrors text endpoints):
//  1. Pre-consume quota — reserve a conservative estimate BEFORE upgrading WS
//  2. Record provisional log — audit trail in case of crash
//  3. Defer billing audit safety net — catch unreconciled pre-consumption
//  4. Run WebSocket session — proxy all frames, parse usage from response.done
//  5. Post-consume quota — reconcile with actual usage (or keep pre-consumed if 0)
func RelayRealtime(c *gin.Context) {
	lg := gmw.GetLogger(c)
	ctx := gmw.Ctx(c)
	start := time.Now()
	relayMeta := meta.GetByContext(c)

	// Record channel requests in flight
	PrometheusMonitor.RecordChannelRequest(relayMeta, start)

	// ── Step 1: Resolve pricing ─────────────────────────────────────────
	var channelModelRatio map[string]float64
	var channelModelConfigs map[string]model.ModelConfigLocal
	var channelCompletionRatio map[string]float64
	if channelModel, ok := c.Get(ctxkey.ChannelModel); ok {
		if channel, ok := channelModel.(*model.Channel); ok {
			channelModelRatio = channel.GetModelRatioFromConfigs()
			channelModelConfigs = channel.GetModelPriceConfigs()
			channelCompletionRatio = channel.GetCompletionRatioFromConfigs()
		}
	}

	pricingAdaptor := resolveRealtimePricingAdaptor(relayMeta)
	modelName := relayMeta.ActualModelName
	modelRatio := pricing.GetModelRatioWithThreeLayers(modelName, channelModelRatio, pricingAdaptor)
	groupRatio := c.GetFloat64(ctxkey.ChannelRatio)

	// ── Step 2: Pre-consume quota ───────────────────────────────────────
	// Estimate based on a short audio conversation.
	// Use audio pricing when available (much higher than text), fall back to text.
	preConsumedQuota := estimateRealtimePreConsumeQuota(
		modelName, modelRatio, groupRatio, channelModelConfigs, pricingAdaptor)

	// Check user quota before allowing the session
	userQuota, err := model.CacheGetUserQuota(ctx, relayMeta.UserId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"message": "failed to get user quota",
			"type":    "one_api_error",
		}})
		PrometheusMonitor.RecordRelayRequest(c, relayMeta, start, false, 0, 0, 0)
		return
	}
	if userQuota < preConsumedQuota {
		c.JSON(http.StatusForbidden, gin.H{"error": gin.H{
			"message": "insufficient quota for realtime session",
			"type":    "insufficient_quota",
		}})
		PrometheusMonitor.RecordRelayRequest(c, relayMeta, start, false, 0, 0, 0)
		return
	}

	// Check if user has enough quota that we can skip pre-consumption (trusted user)
	tokenQuota := c.GetInt64(ctxkey.TokenQuota)
	tokenQuotaUnlimited := c.GetBool(ctxkey.TokenQuotaUnlimited)
	if userQuota > 100*preConsumedQuota &&
		(tokenQuotaUnlimited || tokenQuota > 100*preConsumedQuota) {
		// Trusted user with plenty of quota — skip pre-consumption
		preConsumedQuota = 0
		lg.Info("realtime: user has enough quota, skip pre-consume",
			zap.Int("user_id", relayMeta.UserId),
			zap.Int64("user_quota", userQuota))
	}

	if preConsumedQuota > 0 {
		if err := model.PreConsumeTokenQuota(ctx, relayMeta.TokenId, preConsumedQuota); err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": gin.H{
				"message": "failed to pre-consume quota: " + err.Error(),
				"type":    "pre_consume_failed",
			}})
			PrometheusMonitor.RecordRelayRequest(c, relayMeta, start, false, 0, 0, 0)
			return
		}
	}

	// ── Step 3: Record provisional log & safety net ─────────────────────
	rtMarkPreConsumed(c, preConsumedQuota)
	defer rtBillingAuditSafetyNet(c)

	provisionalLogId := rtRecordProvisionalLog(c, relayMeta, modelName, preConsumedQuota)
	c.Set(ctxkey.ProvisionalLogId, provisionalLogId)

	// Mark that we are about to forward upstream — prevents refund after this point
	c.Set(ctxkey.UpstreamRequestPossiblyForwarded, true)

	// ── Step 4: Run WebSocket session ───────────────────────────────────
	bizErr, usage := openai.RealtimeHandler(c, relayMeta)
	if bizErr != nil {
		// Handshake/connection error — upstream was NOT reached, safe to refund
		c.Set(ctxkey.UpstreamRequestPossiblyForwarded, false)
		rtReturnPreConsumedQuota(ctx, c, preConsumedQuota, relayMeta.TokenId, "realtime_connect_failed")
		c.JSON(bizErr.StatusCode, gin.H{"error": bizErr.Error})
		PrometheusMonitor.RecordRelayRequest(c, relayMeta, start, false, 0, 0, 0)
		return
	}

	// ── Step 5: Post-consume quota (reconcile) ──────────────────────────
	quotaUsed := postConsumeRealtimeQuota(c, relayMeta, usage, preConsumedQuota,
		modelRatio, groupRatio, channelModelRatio, channelModelConfigs,
		channelCompletionRatio, pricingAdaptor, provisionalLogId)

	// Record metrics with actual usage
	promptTokens, completionTokens := 0, 0
	if usage != nil {
		promptTokens = usage.PromptTokens
		completionTokens = usage.CompletionTokens
	}

	lg.Debug("realtime session closed",
		zap.Int("prompt_tokens", promptTokens),
		zap.Int("completion_tokens", completionTokens),
		zap.Float64("quota_used", quotaUsed))
	PrometheusMonitor.RecordRelayRequest(c, relayMeta, start, true, promptTokens, completionTokens, quotaUsed)
}

// postConsumeRealtimeQuota reconciles actual usage against pre-consumed quota
// after a realtime WebSocket session ends.
//
// Key safety invariant: if usage is zero but pre-consumed quota exists, the
// pre-consumed amount is KEPT (not refunded) to prevent free rides when
// upstream fails to report usage.
func postConsumeRealtimeQuota(
	c *gin.Context,
	relayMeta *meta.Meta,
	usage *rmodel.Usage,
	preConsumedQuota int64,
	modelRatio float64,
	groupRatio float64,
	channelModelRatio map[string]float64,
	channelModelConfigs map[string]model.ModelConfigLocal,
	channelCompletionRatio map[string]float64,
	pricingAdaptor adaptor.Adaptor,
	provisionalLogId int,
) float64 {
	lg := gmw.GetLogger(c)

	if relayMeta.TokenId <= 0 || relayMeta.UserId <= 0 || relayMeta.ChannelId <= 0 {
		lg.Error("realtime billing: meta information incomplete, cannot post consume quota",
			zap.Int("token_id", relayMeta.TokenId),
			zap.Int("user_id", relayMeta.UserId),
			zap.Int("channel_id", relayMeta.ChannelId))
		return 0
	}

	modelName := relayMeta.ActualModelName

	// ── ZERO-USAGE GUARD ────────────────────────────────────────────────
	// If upstream reported no usage but we pre-consumed quota, keep the
	// pre-consumed amount as the charge. This prevents free rides when
	// upstream fails to emit response.done / usage events.
	if usage == nil || (usage.PromptTokens == 0 && usage.CompletionTokens == 0) {
		if preConsumedQuota > 0 {
			lg.Warn("realtime billing: zero usage but pre-consumed quota exists, keeping pre-consumed amount",
				zap.Int64("pre_consumed_quota", preConsumedQuota),
				zap.String("model", modelName))
		}
		// Mark billing reconciled — the pre-consumed amount is the final charge
		rtMarkBillingReconciled(c)
		return float64(preConsumedQuota)
	}

	// ── Apply audio token surcharge ─────────────────────────────────────
	// quota.Compute bills all tokens at uniform text rate. Realtime sessions
	// contain audio tokens that cost significantly more. Add the delta as a
	// surcharge to usage.ToolsCost so it's included in the total quota.
	applyRealtimeAudioSurcharge(usage, modelName, modelRatio, groupRatio,
		channelModelRatio, channelModelConfigs, pricingAdaptor, lg)

	// ── Compute actual quota from usage ─────────────────────────────────
	computeResult := quotautil.Compute(quotautil.ComputeInput{
		Usage:                  usage,
		ModelName:              modelName,
		ModelRatio:             modelRatio,
		ChannelModelRatio:      channelModelRatio,
		GroupRatio:             groupRatio,
		ChannelModelConfigs:    channelModelConfigs,
		ChannelCompletionRatio: channelCompletionRatio,
		PricingAdaptor:         pricingAdaptor,
	})

	totalQuota := computeResult.TotalQuota
	if computeResult.PromptTokens+computeResult.CompletionTokens == 0 {
		totalQuota = 0
	}

	// quotaDelta = actual - preConsumed
	// Positive means we need to charge more; negative means we over-reserved.
	quotaDelta := totalQuota - preConsumedQuota

	requestId := c.GetString(ctxkey.RequestId)
	traceId := tracing.GetTraceID(c)

	lg.Info("realtime session billing",
		zap.String("model", modelName),
		zap.Int("prompt_tokens", computeResult.PromptTokens),
		zap.Int("completion_tokens", computeResult.CompletionTokens),
		zap.Int64("total_quota", totalQuota),
		zap.Int64("pre_consumed_quota", preConsumedQuota),
		zap.Int64("quota_delta", quotaDelta),
		zap.Float64("model_ratio", computeResult.UsedModelRatio),
		zap.Float64("group_ratio", groupRatio),
		zap.Float64("completion_ratio", computeResult.UsedCompletionRatio),
		zap.String("request_id", requestId),
		zap.String("trace_id", traceId))

	// Mark billing reconciled so the safety net doesn't fire
	rtMarkBillingReconciled(c)

	// Run billing in a critical goroutine so graceful shutdown waits for it
	graceful.GoCritical(gmw.BackgroundCtx(c), "realtimePostBilling", func(ctx context.Context) {
		billingTimeout := time.Duration(config.BillingTimeoutSec) * time.Second
		ctx, cancel := context.WithTimeout(ctx, billingTimeout)
		defer cancel()

		userAPIFormat := ""
		if relayMeta.Mode != relaymode.Unknown {
			userAPIFormat = relaymode.String(relayMeta.Mode)
		}
		billing.PostConsumeQuotaDetailed(billing.QuotaConsumeDetail{
			Ctx:               ctx,
			TokenId:           relayMeta.TokenId,
			QuotaDelta:        quotaDelta,
			TotalQuota:        totalQuota,
			UserId:            relayMeta.UserId,
			ChannelId:         relayMeta.ChannelId,
			PromptTokens:      computeResult.PromptTokens,
			CompletionTokens:  computeResult.CompletionTokens,
			ModelRatio:        computeResult.UsedModelRatio,
			GroupRatio:        groupRatio,
			ModelName:         modelName,
			TokenName:         relayMeta.TokenName,
			IsStream:          true,
			StartTime:         relayMeta.StartTime,
			CompletionRatio:   computeResult.UsedCompletionRatio,
			RequestId:         requestId,
			TraceId:           traceId,
			ProvisionalLogId:  provisionalLogId,
			UserAPIFormat:     userAPIFormat,
			UpstreamAPIFormat: apitype.String(relayMeta.APIType),
			UpstreamEndpoint:  relayMeta.UpstreamRequestURL,
		})
	})

	return float64(totalQuota)
}

// resolveRealtimePricingAdaptor returns the pricing adaptor for a realtime session
// using the same two-layer lookup as text endpoints.
func resolveRealtimePricingAdaptor(relayMeta *meta.Meta) adaptor.Adaptor {
	if a := relay.GetAdaptor(relayMeta.APIType); a != nil {
		return a
	}
	return relay.GetAdaptor(relayMeta.ChannelType)
}

// RelayRealtimeSessions handles POST /v1/realtime/sessions by proxying to the
// upstream OpenAI Realtime Sessions API to create ephemeral tokens for WebRTC clients.
func RelayRealtimeSessions(c *gin.Context) {
	start := time.Now()
	relayMeta := meta.GetByContext(c)

	PrometheusMonitor.RecordChannelRequest(relayMeta, start)

	if bizErr, err := openai.RealtimeSessionsHandler(c, relayMeta); bizErr != nil {
		if !c.Writer.Written() {
			c.JSON(bizErr.StatusCode, gin.H{"error": bizErr.Error})
		}
		PrometheusMonitor.RecordRelayRequest(c, relayMeta, start, false, 0, 0, 0)
		if err != nil {
			gmw.GetLogger(c).Error("realtime sessions error", zap.Error(err))
		}
		return
	}

	PrometheusMonitor.RecordRelayRequest(c, relayMeta, start, true, 0, 0, 0)
}
