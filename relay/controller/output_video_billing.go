package controller

import (
	"math"
	"strings"

	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/billing/ratio"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/pricing"
)

// getOutputVideoSeconds reads the output video duration from Gin context.
// Parameters: c is the Gin context for the current request.
// Returns: the positive duration in seconds, or 0 when absent.
func getOutputVideoSeconds(c *gin.Context) float64 {
	if c == nil {
		return 0
	}
	raw, ok := c.Get(ctxkey.OutputVideoSeconds)
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

// getOutputVideoResolution reads the output video resolution string from Gin context.
// Parameters: c is the Gin context for the current request.
// Returns: the resolution string, or empty when absent.
func getOutputVideoResolution(c *gin.Context) string {
	if c == nil {
		return ""
	}
	raw, ok := c.Get(ctxkey.OutputVideoResolution)
	if !ok {
		return ""
	}
	if value, ok := raw.(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

// applyOutputVideoCharges adds per-second quota usage for video outputs.
// Parameters: c is the Gin context for the request; usagePtr points to the usage struct to update;
// meta carries model identity and pricing context.
// Returns: nothing; the usage.ToolsCost field is augmented when applicable.
func applyOutputVideoCharges(c *gin.Context, usagePtr **relaymodel.Usage, meta *metalib.Meta) {
	if c == nil || meta == nil || usagePtr == nil {
		return
	}

	seconds := getOutputVideoSeconds(c)
	if seconds <= 0 {
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

	var channelVideoOverride *adaptor.VideoPricingConfig
	if billingCtx.ChannelModelConfigs != nil {
		if cfg, ok := billingCtx.ChannelModelConfigs[billingCtx.ModelName]; ok && cfg.Video != nil {
			channelVideoOverride = convertVideoLocalToAdaptor(cfg.Video)
		}
	}

	videoPricing := pricing.GetVideoPricingWithThreeLayers(billingCtx.ModelName, channelVideoOverride, billingCtx.PricingAdaptor)
	if videoPricing == nil || videoPricing.PerSecondUsd <= 0 {
		if billingCtx.Logger != nil {
			billingCtx.Logger.Debug("output video billing skipped due to missing pricing metadata",
				zap.String("model", billingCtx.ModelName),
				zap.Float64("seconds", seconds),
			)
		}
		return
	}

	resolution := getOutputVideoResolution(c)
	multiplier := videoPricing.EffectiveMultiplier(resolution)
	costUsd := videoPricing.PerSecondUsd * multiplier * seconds
	groupRatio := billingCtx.GroupRatio
	videoQuota := int64(math.Ceil(costUsd * ratio.QuotaPerUsd * groupRatio))
	if videoQuota <= 0 {
		if billingCtx.Logger != nil {
			billingCtx.Logger.Debug("output video billing skipped due to zero quota",
				zap.String("model", billingCtx.ModelName),
				zap.Float64("seconds", seconds),
				zap.String("resolution", resolution),
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

	usage.ToolsCost += videoQuota

	if billingCtx.Logger != nil {
		billingCtx.Logger.Debug("output video billing applied",
			zap.String("model", billingCtx.ModelName),
			zap.Float64("seconds", seconds),
			zap.String("resolution", resolution),
			zap.Float64("per_second_usd", videoPricing.PerSecondUsd),
			zap.Float64("multiplier", multiplier),
			zap.Int64("video_quota", videoQuota),
			zap.Float64("group_ratio", groupRatio),
			zap.Float64("quota_per_usd", ratio.QuotaPerUsd),
		)
	}
}
