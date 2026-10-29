package controller

import (
	"github.com/Laisky/errors/v2"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/geminiOpenaiCompatible"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/apitype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/gin-gonic/gin"
	"net/http"
)

// isNativeNanoImageBilling identifies native Nano transport without changing other provider tariffs.
// Parameters: meta is the effective request metadata.
// Returns: whether the native transport uses the fixed default image tier.
func isNativeNanoImageBilling(meta *metalib.Meta) bool {
	return meta != nil && (meta.APIType == apitype.Gemini || meta.APIType == apitype.VertexAI) && geminiOpenaiCompatible.IsGeminiNanoBanana21Model(meta.ActualModelName)
}

// validateNativeNanoImageTariff rejects an explicit paid tariff that cannot price the rendered tier.
// Parameters: c supplies channel pricing and meta identifies the mapped model and transport.
// Returns: a controlled configuration error before upstream dispatch, or nil.
func validateNativeNanoImageTariff(c *gin.Context, meta *metalib.Meta) *relaymodel.ErrorWithStatusCode {
	if !isNativeNanoImageBilling(meta) || meta.Mode == relaymode.Embeddings || c.GetFloat64(ctxkey.ChannelRatio) == 0 {
		return nil
	}
	ctx, ok := outputBillingContextFromRequest(c, meta)
	if !ok {
		return nil
	}
	cfg, ok := pricing.ResolveImagePricing(ctx.ModelName, ctx.ChannelModelConfigs, ctx.PricingAdaptor, ctx.RequestTime)
	if !ok || cfg == nil || cfg.PricePerImageUsd <= 0 {
		return nil
	}
	if _, err := nativeNanoImageTierRatio(ctx, cfg); err != nil {
		return openai.ErrorWrapper(errors.Wrap(err, "native Nano Banana 1K standard image tariff is not configured"), "image_pricing_unavailable", http.StatusServiceUnavailable)
	}
	return nil
}

// nativeNanoImageTierRatio resolves an explicit 1K operator override before the configured tier table.
// Parameters: ctx supplies channel ratios and cfg is the selected tariff.
// Returns: the 1K standard multiplier or an error when that tier is unavailable.
func nativeNanoImageTierRatio(ctx outputBillingContext, cfg *adaptor.ImagePricingConfig) (float64, error) {
	if override, ok := getChannelImageTierOverride(ctx.ChannelModelRatio, ctx.ModelName, "1024x1024", "standard"); ok {
		return override, nil
	}
	return getImageCostRatio(&relaymodel.ImageRequest{Model: ctx.ModelName, Size: "1024x1024", Quality: "standard"}, cfg)
}
