package controller

import (
	"math"

	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/relay/billing/ratio"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/pricing"
)

// getOutputAudioSeconds reads the output audio duration from Gin context.
// Parameters: c is the Gin context for the current request.
// Returns: the positive duration in seconds, or 0 when absent.
func getOutputAudioSeconds(c *gin.Context) float64 {
	if c == nil {
		return 0
	}
	raw, ok := c.Get(ctxkey.OutputAudioSeconds)
	if !ok {
		return 0
	}
	switch v := raw.(type) {
	case float64:
		if v > 0 {
			return v
		}
	case float32:
		if v > 0 {
			return float64(v)
		}
	case int:
		if v > 0 {
			return float64(v)
		}
	case int64:
		if v > 0 {
			return float64(v)
		}
	}
	return 0
}

// getOutputAudioTokens reads the output audio token count from Gin context.
// Parameters: c is the Gin context for the current request.
// Returns: the positive token count, or 0 when absent.
func getOutputAudioTokens(c *gin.Context) int {
	if c == nil {
		return 0
	}
	raw, ok := c.Get(ctxkey.OutputAudioTokens)
	if !ok {
		return 0
	}
	switch v := raw.(type) {
	case int:
		return max(v, 0)
	case int64:
		if v > 0 {
			return int(v)
		}
	case float64:
		if v > 0 {
			return int(v)
		}
	}
	return 0
}

// applyOutputAudioCharges adds per-second or per-token quota usage for audio outputs.
// Parameters: c is the Gin context for the request; usagePtr points to the usage struct to update;
// meta carries model identity and pricing context.
// Returns: nothing; the usage.ToolsCost field is augmented when applicable.
func applyOutputAudioCharges(c *gin.Context, usagePtr **relaymodel.Usage, meta *metalib.Meta) {
	if c == nil || meta == nil || usagePtr == nil {
		return
	}

	seconds := getOutputAudioSeconds(c)
	tokens := getOutputAudioTokens(c)
	if seconds <= 0 && tokens == 0 {
		return
	}

	billingCtx, ok := outputBillingContextFromRequest(c, meta)
	if !ok {
		return
	}
	usage := *usagePtr
	if usage == nil {
		usage = &relaymodel.Usage{}
		*usagePtr = usage
	}

	audioPricing, ok := pricing.ResolveAudioPricing(billingCtx.ModelName, billingCtx.ChannelModelConfigs, billingCtx.PricingAdaptor)
	if !ok || audioPricing == nil || !audioPricing.HasData() {
		if billingCtx.Logger != nil {
			billingCtx.Logger.Debug("output audio billing skipped due to missing pricing metadata",
				zap.String("model", billingCtx.ModelName),
				zap.Float64("seconds", seconds),
				zap.Int("audio_tokens", tokens),
			)
		}
		return
	}

	groupRatio := billingCtx.GroupRatio
	var audioQuota int64
	if seconds > 0 && audioPricing.UsdPerSecond > 0 {
		costUsd := seconds * audioPricing.UsdPerSecond
		audioQuota = int64(math.Ceil(costUsd * ratio.QuotaPerUsd * groupRatio))
	} else if tokens > 0 {
		promptRatio := pricing.DefaultAudioPromptRatio
		completionRatio := pricing.DefaultAudioCompletionRatio
		if audioPricing.PromptRatio > 0 {
			promptRatio = audioPricing.PromptRatio
		}
		if audioPricing.CompletionRatio > 0 {
			completionRatio = audioPricing.CompletionRatio
		}
		modelRatio := pricing.GetModelRatioWithThreeLayers(billingCtx.ModelName, billingCtx.ChannelModelRatio, billingCtx.PricingAdaptor)
		cost := float64(tokens) * promptRatio * completionRatio * modelRatio * groupRatio
		audioQuota = int64(math.Ceil(cost))
	}

	if audioQuota <= 0 {
		if billingCtx.Logger != nil {
			billingCtx.Logger.Debug("output audio billing skipped due to zero quota",
				zap.String("model", billingCtx.ModelName),
				zap.Float64("seconds", seconds),
				zap.Int("audio_tokens", tokens),
				zap.Float64("usd_per_second", audioPricing.UsdPerSecond),
			)
		}
		return
	}

	if usage.PromptTokens == 0 && billingCtx.PromptTokens > 0 {
		usage.PromptTokens = billingCtx.PromptTokens
	}
	if usage.TotalTokens == 0 && (usage.PromptTokens != 0 || usage.CompletionTokens != 0) {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}

	usage.ToolsCost += audioQuota

	if billingCtx.Logger != nil {
		billingCtx.Logger.Debug("output audio billing applied",
			zap.String("model", billingCtx.ModelName),
			zap.Float64("seconds", seconds),
			zap.Int("audio_tokens", tokens),
			zap.Float64("usd_per_second", audioPricing.UsdPerSecond),
			zap.Int64("audio_quota", audioQuota),
			zap.Float64("group_ratio", groupRatio),
			zap.Float64("quota_per_usd", ratio.QuotaPerUsd),
		)
	}
}
