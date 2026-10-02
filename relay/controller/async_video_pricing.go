package controller

import (
	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
)

// asyncVideoQuotedQuota reuses existing channel/provider pricing resolution and
// decimal quota helpers. Explicit administrator per-call/video overrides win;
// otherwise the adapter must supply a positive request-specific quote.
func asyncVideoQuotedQuota(c *gin.Context, info *meta.Meta, request *relaymodel.VideoRequest, images int) (int64, error) {
	var overrides map[string]model.ModelConfigLocal
	if value, ok := c.Get(ctxkey.ChannelModel); ok {
		if channel, ok := value.(*model.Channel); ok {
			overrides = channel.GetModelPriceConfigsWithContext(gmw.Ctx(c))
		}
	}
	ad := resolvePricingAdaptor(info)
	cfg, configured := pricing.ResolveModelConfig(info.ActualModelName, overrides, ad, info.StartTime)
	group := c.GetFloat64(ctxkey.ChannelRatio)
	if configured && cfg.PerCall != nil {
		return decimalQuotaRate(1000, cfg.PerCall.UsdPerThousandCalls, billingratio.QuotaPerUsd, group)
	}
	var rate *adaptor.VideoPricingConfig
	if configured && cfg.Video != nil && cfg.Video.HasData() {
		rate = cfg.Video
	}
	if rate == nil {
		estimator, ok := ad.(adaptor.VideoPricingEstimator)
		if !ok {
			return 0, errors.New("async video requires a price override or quote estimator")
		}
		var err error
		rate, err = estimator.EstimateVideoPricing(c, info, request)
		if err != nil {
			return 0, errors.Wrap(err, "quote async video")
		}
	}
	if rate == nil {
		return 0, errors.New("async video quote is missing")
	}
	if rate.TotalUsdDecimal != "" {
		return videoQuotaFromTotalDecimal(rate.TotalUsdDecimal, group)
	}
	if rate.TotalUsd > 0 {
		return videoQuotaFromTotal(rate.TotalUsd, group)
	}
	return videoQuota(rate.PerSecondUsd, rate.EffectiveMultiplier(request.RequestedResolution()), request.RequestedDurationSeconds(), rate.InputImageUsd, images, group)
}
