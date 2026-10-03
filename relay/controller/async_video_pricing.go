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
func asyncVideoQuotedQuota(c *gin.Context, info *meta.Meta, request *relaymodel.VideoRequest, images int) (int64, string, error) {
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
		quota, err := decimalQuotaRate(1000, cfg.PerCall.UsdPerThousandCalls, billingratio.QuotaPerUsd, group)
		return quota, "", err
	}
	var rate *adaptor.VideoPricingConfig
	factor := ""
	if configured && cfg.Video != nil && cfg.Video.HasData() {
		rate = cfg.Video
	}
	if rate == nil {
		estimator, ok := ad.(adaptor.VideoPricingEstimator)
		if !ok {
			return 0, "", errors.New("async video requires a price override or quote estimator")
		}
		var err error
		rate, err = estimator.EstimateVideoPricing(c, info, request)
		if err != nil {
			return 0, "", errors.Wrap(err, "quote async video")
		}
		factor, err = model.AsyncCostMultiplier(billingratio.QuotaPerUsd, group)
		if err != nil {
			return 0, "", err
		}
	}
	if rate == nil {
		return 0, "", errors.New("async video quote is missing")
	}
	if rate.TotalUsdDecimal != "" {
		quota, err := videoQuotaFromTotalDecimal(rate.TotalUsdDecimal, group)
		return quota, factor, err
	}
	if rate.TotalUsd > 0 {
		quota, err := videoQuotaFromTotal(rate.TotalUsd, group)
		return quota, factor, err
	}
	quota, err := videoQuota(rate.PerSecondUsd, rate.EffectiveMultiplier(request.RequestedResolution()), request.RequestedDurationSeconds(), rate.InputImageUsd, images, group)
	return quota, factor, err
}
