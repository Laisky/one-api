package vertexai

import (
	"math"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/pricing"
)

var _ adaptor.RequestPricingValidator = (*Adaptor)(nil)

// requiresClaude52Pricing identifies newly listed models without verified Google tariffs.
func requiresClaude52Pricing(name string) bool {
	return anthropic.IsClaude52(name)
}

// ValidateRequestPricing requires complete channel-owned tariffs for Claude 5.2.
// It preserves existing channel data and uses billing's request-start window resolver.
func (a *Adaptor) ValidateRequestPricing(c *gin.Context, m *meta.Meta) error {
	if m == nil {
		return errors.New("Vertex request pricing requires model metadata")
	}
	name := m.ActualModelName
	if name == "" {
		name = m.OriginModelName
	}
	if !requiresClaude52Pricing(name) {
		return nil
	}
	if err := pricing.RequireExplicitTokenPricing(c, m); err != nil {
		return err
	}
	value, _ := c.Get(ctxkey.ChannelModel)
	channel := value.(*dbmodel.Channel) // RequireExplicitTokenPricing validated this value.
	configs := channel.GetModelPriceConfigsWithContext(gmw.Ctx(c))
	cfg, found := pricing.ResolveModelConfigRatioOnly(name, configs, nil, m.StartTime)
	valid := func(v float64) bool { return v > 0 && !math.IsNaN(v) && !math.IsInf(v, 0) }
	if !found || !valid(cfg.CachedInputRatio) || !valid(cfg.CacheWrite5mRatio) || !valid(cfg.CacheWrite1hRatio) {
		return errors.New("Vertex Claude 5.2 requires explicit positive cache read, 5m write, and 1h write pricing in channel model_configs")
	}
	for _, tier := range cfg.Tiers {
		// Zero tier fields inherit the complete base tariff, matching billing.
		for _, v := range []float64{tier.CachedInputRatio, tier.CacheWrite5mRatio, tier.CacheWrite1hRatio} {
			if v != 0 && !valid(v) {
				return errors.New("Vertex Claude 5.2 has invalid explicit tier cache pricing")
			}
		}
	}
	return nil
}
