package controller

import (
	"context"
	"fmt"
	"math"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/graceful"
	"github.com/songquanpeng/one-api/common/tracing"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/billing"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	rmodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/pricing"
)

// Billing safety helpers for the top-level controller package.
// These mirror the logic in relay/controller/billing_safety.go but are
// accessible from the top-level controller package (which is a separate
// Go package from relay/controller despite sharing the same name).

// rtMarkPreConsumed records the pre-consumed quota in the gin context
// so the audit safety net can detect unreconciled billing.
func rtMarkPreConsumed(c *gin.Context, amount int64) {
	c.Set(ctxkey.PreConsumedQuotaAmount, amount)
}

// rtMarkBillingReconciled marks that post-billing or refund has completed,
// clearing the audit safety net flag.
func rtMarkBillingReconciled(c *gin.Context) {
	c.Set(ctxkey.BillingReconciled, true)
}

// rtBillingAuditSafetyNet should be deferred at the start of each relay handler
// (after pre-consume). It detects unreconciled pre-consumed quota and attempts
// recovery.
func rtBillingAuditSafetyNet(c *gin.Context) {
	reconciled, _ := c.Get(ctxkey.BillingReconciled)
	if reconciled != nil {
		if r, ok := reconciled.(bool); ok && r {
			return
		}
	}

	preConsumedAny, exists := c.Get(ctxkey.PreConsumedQuotaAmount)
	if !exists {
		return
	}
	preConsumed, ok := preConsumedAny.(int64)
	if !ok || preConsumed <= 0 {
		return
	}

	lg := gmw.GetLogger(c)
	tokenId := c.GetInt(ctxkey.TokenId)
	requestId := c.GetString(ctxkey.RequestId)

	lg.Error("CRITICAL BILLING AUDIT (realtime): pre-consumed quota was not reconciled",
		zap.Int64("pre_consumed_quota", preConsumed),
		zap.Int("token_id", tokenId),
		zap.String("request_id", requestId),
	)

	// Do NOT refund — the request was forwarded upstream, so keeping the
	// pre-consumed amount is the safe default to avoid underbilling.
}

// rtReturnPreConsumedQuota refunds pre-consumed quota when the upstream
// was NOT reached (connection error before forwarding).
func rtReturnPreConsumedQuota(
	ctx context.Context,
	c *gin.Context,
	preConsumedQuota int64,
	tokenID int,
	reason string,
) {
	if preConsumedQuota <= 0 {
		return
	}

	rtMarkBillingReconciled(c)

	// Reconcile provisional log to 0
	if provLogID := c.GetInt(ctxkey.ProvisionalLogId); provLogID > 0 {
		if err := model.ReconcileConsumeLog(ctx, provLogID, 0,
			fmt.Sprintf("refunded: %s", reason), 0, 0, 0, nil); err != nil {
			gmw.GetLogger(c).Warn("failed to reconcile provisional log on refund",
				zap.Error(err), zap.Int("provisional_log_id", provLogID))
		}
	}

	graceful.GoCritical(gmw.BackgroundCtx(c), "realtimeRefund", func(ctx context.Context) {
		billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, tokenID)
	})
}

// rtRecordProvisionalLog writes a provisional consume log entry at pre-consume time.
func rtRecordProvisionalLog(c *gin.Context, meta *metalib.Meta, modelName string, estimatedQuota int64) int {
	if estimatedQuota <= 0 || meta == nil {
		return 0
	}

	requestId := c.GetString(ctxkey.RequestId)
	traceId := tracing.GetTraceIDFromContext(gmw.Ctx(c))

	logEntry := &model.Log{
		UserId:    meta.UserId,
		ChannelId: meta.ChannelId,
		ModelName: modelName,
		TokenName: meta.TokenName,
		IsStream:  true,
		RequestId: requestId,
		TraceId:   traceId,
	}

	return model.RecordProvisionalConsumeLog(gmw.Ctx(c), logEntry, estimatedQuota)
}

// applyRealtimeAudioSurcharge adds a surcharge to usage.ToolsCost for audio
// tokens in realtime sessions. quota.Compute bills ALL tokens at the uniform
// text rate. Audio tokens cost significantly more (8-20x), so this function
// computes the delta between audio pricing and text pricing, and adds it
// as a ToolsCost surcharge.
//
// Formula:
//
//	prompt_surcharge  = audio_input_tokens  * modelRatio * groupRatio * (AudioPromptRatio - 1)
//	output_surcharge  = audio_output_tokens * modelRatio * groupRatio * (AudioPromptRatio * AudioCompletionRatio - CompletionRatio)
//	total_surcharge   = prompt_surcharge + output_surcharge
//
// The "-1" and "-CompletionRatio" terms subtract the text-rate charge that
// Compute() already includes, so we only add the difference.
func applyRealtimeAudioSurcharge(
	usage *rmodel.Usage,
	modelName string,
	modelRatio float64,
	groupRatio float64,
	channelModelRatio map[string]float64,
	channelModelConfigs map[string]model.ModelConfigLocal,
	pricingAdaptor adaptor.Adaptor,
	lg glog.Logger,
) {
	if usage == nil {
		return
	}

	// Collect audio token counts from details
	var audioInputTokens, audioOutputTokens int
	if d := usage.PromptTokensDetails; d != nil {
		audioInputTokens = d.AudioTokens
	}
	if d := usage.CompletionTokensDetails; d != nil {
		audioOutputTokens = d.AudioTokens
	}
	if audioInputTokens <= 0 && audioOutputTokens <= 0 {
		return // no audio tokens, no surcharge needed
	}

	// Resolve audio pricing
	audioCfg, ok := pricing.ResolveAudioPricing(modelName, channelModelConfigs, pricingAdaptor)
	if !ok || audioCfg == nil || !audioCfg.HasData() {
		if lg != nil {
			lg.Warn("realtime audio surcharge: no audio pricing config found, audio tokens billed at text rate",
				zap.String("model", modelName),
				zap.Int("audio_input_tokens", audioInputTokens),
				zap.Int("audio_output_tokens", audioOutputTokens))
		}
		return
	}

	promptRatio := audioCfg.PromptRatio
	if promptRatio <= 0 {
		promptRatio = pricing.DefaultAudioPromptRatio
	}
	completionRatio := audioCfg.CompletionRatio
	if completionRatio <= 0 {
		completionRatio = pricing.DefaultAudioCompletionRatio
	}

	// Get text completion ratio for the model (what Compute uses)
	textCompletionRatio := pricing.GetCompletionRatioWithThreeLayers(modelName, nil, pricingAdaptor)

	// Prompt surcharge: audio is promptRatio times text, so the delta is (promptRatio - 1)
	promptSurcharge := float64(audioInputTokens) * modelRatio * groupRatio * (promptRatio - 1)

	// Output surcharge: audio output = modelRatio * promptRatio * completionRatio * groupRatio
	//                   text  output = modelRatio * textCompletionRatio * groupRatio
	//                   delta        = modelRatio * groupRatio * (promptRatio*completionRatio - textCompletionRatio)
	outputSurcharge := float64(audioOutputTokens) * modelRatio * groupRatio * (promptRatio*completionRatio - textCompletionRatio)

	totalSurcharge := int64(math.Ceil(promptSurcharge + outputSurcharge))
	if totalSurcharge <= 0 {
		return
	}

	usage.ToolsCost += totalSurcharge

	if lg != nil {
		lg.Info("realtime audio surcharge applied",
			zap.String("model", modelName),
			zap.Int("audio_input_tokens", audioInputTokens),
			zap.Int("audio_output_tokens", audioOutputTokens),
			zap.Float64("audio_prompt_ratio", promptRatio),
			zap.Float64("audio_completion_ratio", completionRatio),
			zap.Float64("text_completion_ratio", textCompletionRatio),
			zap.Int64("surcharge_quota", totalSurcharge))
	}
}
