package controller

import (
	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay"
	"github.com/songquanpeng/one-api/relay/adaptor"
	metalib "github.com/songquanpeng/one-api/relay/meta"
)

// outputBillingContext bundles shared data required to apply output modality billing.
// Parameters: c is the Gin context; meta is the request metadata.
// Returns: the populated context and a boolean indicating if billing should proceed.
func outputBillingContextFromRequest(c *gin.Context, meta *metalib.Meta) (outputBillingContext, bool) {
	if c == nil || meta == nil {
		if c == nil {
			return outputBillingContext{}, false
		}
		lg := gmw.GetLogger(c)
		if lg != nil {
			lg.Debug("output billing skipped due to missing meta")
		}
		return outputBillingContext{}, false
	}
	lg := gmw.GetLogger(c)

	channelModelRatio, channelModelConfigs := getChannelModelPricingFromContext(c)
	pricingAdaptor := relay.GetAdaptor(meta.ChannelType)
	if pricingAdaptor == nil && meta.APIType != 0 {
		pricingAdaptor = relay.GetAdaptor(meta.APIType)
	}
	if pricingAdaptor == nil {
		lg.Debug("output billing skipped due to missing pricing adaptor",
			zap.String("model", meta.ActualModelName),
			zap.Int("channel_type", meta.ChannelType),
			zap.Int("api_type", meta.APIType),
		)
		return outputBillingContext{}, false
	}

	return outputBillingContext{
		Logger:              lg,
		ChannelModelRatio:   channelModelRatio,
		ChannelModelConfigs: channelModelConfigs,
		PricingAdaptor:      pricingAdaptor,
		GroupRatio:          c.GetFloat64(ctxkey.ChannelRatio),
		ModelName:           meta.ActualModelName,
		PromptTokens:        meta.PromptTokens,
	}, true
}

// outputBillingContext describes dependencies needed to charge output modalities.
type outputBillingContext struct {
	Logger              glog.Logger
	ChannelModelRatio   map[string]float64
	ChannelModelConfigs map[string]model.ModelConfigLocal
	PricingAdaptor      adaptor.Adaptor
	GroupRatio          float64
	ModelName           string
	PromptTokens        int
}
