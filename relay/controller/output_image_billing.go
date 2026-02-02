package controller

import (
	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/billing/ratio"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/pricing"
)

// getOutputImageCount reads the output image count stored on the Gin context.
// Parameters: c is the Gin context for the current request.
// Returns: the parsed image count, or 0 if none is recorded.
func getOutputImageCount(c *gin.Context) int {
	if c == nil {
		return 0
	}
	raw, ok := c.Get(ctxkey.OutputImageCount)
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

// hasLocalImagePricing reports whether the channel-level image pricing config has any data.
// Parameters: local is the channel-scoped image pricing config (may be nil).
// Returns: true when any billing-related field is present.
func hasLocalImagePricing(local *model.ImagePricingLocal) bool {
	if local == nil {
		return false
	}
	if local.PricePerImageUsd > 0 || local.PromptRatio > 0 || local.PromptTokenLimit > 0 || local.MinImages > 0 || local.MaxImages > 0 {
		return true
	}
	if local.DefaultSize != "" || local.DefaultQuality != "" {
		return true
	}
	if len(local.SizeMultipliers) > 0 || len(local.QualityMultipliers) > 0 || len(local.QualitySizeMultipliers) > 0 {
		return true
	}
	return false
}

// getChannelModelPricingFromContext extracts channel-scoped model ratios and pricing configs from context.
// Parameters: c is the Gin context for the current request.
// Returns: the model ratio overrides map and model pricing config map (either may be nil).
func getChannelModelPricingFromContext(c *gin.Context) (map[string]float64, map[string]model.ModelConfigLocal) {
	if c == nil {
		return nil, nil
	}
	raw, ok := c.Get(ctxkey.ChannelModel)
	if !ok {
		return nil, nil
	}
	channel, ok := raw.(*model.Channel)
	if !ok || channel == nil {
		return nil, nil
	}
	return channel.GetModelRatioFromConfigs(), channel.GetModelPriceConfigs()
}

// applyOutputImageCharges adds per-image quota usage for chat/response outputs that include images.
// Parameters: c is the Gin context for the request; usagePtr points to the usage struct to update;
// meta carries model identity and pricing context.
// Returns: nothing; the usage.ToolsCost field is augmented when applicable.
func applyOutputImageCharges(c *gin.Context, usagePtr **relaymodel.Usage, meta *metalib.Meta) {
	if c == nil || meta == nil || usagePtr == nil {
		return
	}

	imageCount := getOutputImageCount(c)
	if imageCount == 0 {
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

	imagePricing, ok := pricing.ResolveImagePricing(billingCtx.ModelName, billingCtx.ChannelModelConfigs, billingCtx.PricingAdaptor)
	if !ok || imagePricing == nil || imagePricing.PricePerImageUsd <= 0 {
		if billingCtx.Logger != nil {
			channelHasConfig := false
			channelHasImage := false
			if billingCtx.ChannelModelConfigs != nil {
				if cfg, exists := billingCtx.ChannelModelConfigs[billingCtx.ModelName]; exists {
					channelHasConfig = true
					channelHasImage = hasLocalImagePricing(cfg.Image)
				}
			}
			providerHasImage := false
			if billingCtx.PricingAdaptor != nil {
				if defaults := billingCtx.PricingAdaptor.GetDefaultModelPricing(); defaults != nil {
					if cfg, exists := defaults[billingCtx.ModelName]; exists {
						if cfg.Image != nil && cfg.Image.HasData() {
							providerHasImage = true
						}
					}
				}
			}
			globalHasImage := false
			if cfg, exists := pricing.GetGlobalModelConfig(billingCtx.ModelName); exists {
				if cfg.Image != nil && cfg.Image.HasData() {
					globalHasImage = true
				}
			}
			billingCtx.Logger.Debug("output image billing skipped due to missing pricing metadata",
				zap.String("model", billingCtx.ModelName),
				zap.Int("image_count", imageCount),
				zap.Bool("channel_has_config", channelHasConfig),
				zap.Bool("channel_has_image", channelHasImage),
				zap.Bool("provider_has_image", providerHasImage),
				zap.Bool("global_has_image", globalHasImage),
			)
		}
		return
	}

	size := imagePricing.DefaultSize
	if size == "" {
		size = "1024x1024"
	}
	quality := imagePricing.DefaultQuality
	if quality == "" {
		quality = "standard"
	}

	imageRequest := &relaymodel.ImageRequest{
		Model:   billingCtx.ModelName,
		Size:    size,
		Quality: quality,
		N:       imageCount,
	}

	imageCostRatio, err := getImageCostRatio(imageRequest, imagePricing)
	if err != nil {
		if billingCtx.Logger != nil {
			billingCtx.Logger.Debug("output image billing skipped due to invalid image tier",
				zap.String("model", billingCtx.ModelName),
				zap.String("size", size),
				zap.String("quality", quality),
				zap.Error(errors.Wrap(err, "resolve image tier")),
			)
		}
		return
	}
	if override, ok := getChannelImageTierOverride(billingCtx.ChannelModelRatio, billingCtx.ModelName, size, quality); ok {
		imageCostRatio = override
	}

	groupRatio := billingCtx.GroupRatio
	imageQuota := calculateImageBaseQuota(imagePricing.PricePerImageUsd, 0, imageCostRatio, groupRatio, imageCount)
	if imageQuota <= 0 {
		if billingCtx.Logger != nil {
			billingCtx.Logger.Debug("output image billing skipped due to zero quota",
				zap.String("model", billingCtx.ModelName),
				zap.Int("image_count", imageCount),
				zap.Float64("unit_usd", imagePricing.PricePerImageUsd*imageCostRatio),
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

	usage.ToolsCost += imageQuota

	if billingCtx.Logger != nil {
		billingCtx.Logger.Debug("output image billing applied",
			zap.String("model", billingCtx.ModelName),
			zap.Int("image_count", imageCount),
			zap.String("size", size),
			zap.String("quality", quality),
			zap.Float64("unit_usd", imagePricing.PricePerImageUsd*imageCostRatio),
			zap.Float64("image_tier", imageCostRatio),
			zap.Int64("image_quota", imageQuota),
			zap.Float64("group_ratio", groupRatio),
			zap.Float64("quota_per_usd", ratio.QuotaPerUsd),
		)
	}
}
