package vertexai

import (
	"math"
	"time"

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

var (
	_ adaptor.RequestPricingValidator           = (*Adaptor)(nil)
	_ adaptor.OperatorTariffPolicy              = (*Adaptor)(nil)
	_ pricing.OperatorTariffSupplementValidator = (*Adaptor)(nil)
)

// argonCyberModel is the Vertex Gemini model that needs an operator token and
// cache-read tariff.
const argonCyberModel = "gemini-4-argon-cyber-permissive"

// requiresClaude52Pricing identifies newly listed models without verified Google tariffs.
func requiresClaude52Pricing(name string) bool {
	return anthropic.IsClaude52(name)
}

// RequiresOperatorTariff reports whether modelName lacks a bundled Vertex
// tariff: Claude 5.2, Gemini 4 Argon Cyber, and the Live-only Google models.
// Vertex Live rates are operator-owned (RequiresExplicitRealtimePricing), and
// without this gate the resolver fell through to the global catalog and
// reported the Gemini Developer API price for gemini-3.8-live, which is another
// host's tariff and lacks Vertex's separate image/video input rate.
// Parameters: modelName is the mapped model. Returns: true when only a complete
// channel tariff may bill it, so pricing never borrows Anthropic's or the
// global catalog's price instead.
func (a *Adaptor) RequiresOperatorTariff(modelName string) bool {
	return modelName == argonCyberModel || requiresClaude52Pricing(modelName) ||
		adaptor.IsLiveOnlyGoogleModel(modelName)
}

// ValidateRequestPricing requires complete channel-owned tariffs for Claude 5.2 and Argon Cyber.
// It preserves existing channel data and uses billing's request-start window resolver.
func (a *Adaptor) ValidateRequestPricing(c *gin.Context, m *meta.Meta) error {
	if m == nil {
		return errors.New("Vertex request pricing requires model metadata")
	}
	name := m.ActualModelName
	if name == "" {
		name = m.OriginModelName
	}
	if name == argonCyberModel {
		if err := pricing.RequireExplicitTokenPricing(c, m); err != nil {
			return errors.Wrap(err, "validate Vertex Argon Cyber token pricing")
		}
		value, _ := c.Get(ctxkey.ChannelModel)
		channel := value.(*dbmodel.Channel) // RequireExplicitTokenPricing validated this value.
		return a.ValidateOperatorTariffSupplement(name, channel.GetModelPriceConfigsWithContext(gmw.Ctx(c)), m.StartTime)
	}
	if !requiresClaude52Pricing(name) {
		return nil
	}
	if err := pricing.RequireExplicitTokenPricing(c, m); err != nil {
		return err
	}
	value, _ := c.Get(ctxkey.ChannelModel)
	channel := value.(*dbmodel.Channel) // RequireExplicitTokenPricing validated this value.
	return a.ValidateOperatorTariffSupplement(name, channel.GetModelPriceConfigsWithContext(gmw.Ctx(c)), m.StartTime)
}

// ValidateOperatorTariffSupplement enforces the cache prices Vertex dispatch
// requires on top of the complete token tariff: a positive cache read price for
// Argon Cyber, and positive cache read, 5m write, and 1h write prices for
// Claude 5.2; explicit tier cache prices must be positive when set, and a zero
// tier field inherits the base tariff, matching billing. ValidateRequestPricing
// and the channel editor's pre-save check share it. Parameters: name is the
// mapped model, configs the channel's parsed model_configs, and at the request
// start time. Returns: nil for other models or a complete tariff, otherwise an
// error matching adaptor.ErrRequestPricingNotConfigured.
func (a *Adaptor) ValidateOperatorTariffSupplement(name string, configs map[string]dbmodel.ModelConfigLocal, at time.Time) error {
	valid := func(v float64) bool { return v > 0 && !math.IsNaN(v) && !math.IsInf(v, 0) }
	switch {
	case name == argonCyberModel:
		cfg, found := pricing.ResolveModelConfigRatioOnly(name, configs, nil, at)
		if !found || !valid(cfg.CachedInputRatio) {
			return adaptor.RequestPricingNotConfigured(errors.New("Vertex Argon Cyber requires explicit positive cache read pricing in channel model_configs"))
		}
		for _, tier := range cfg.Tiers {
			if tier.CachedInputRatio != 0 && !valid(tier.CachedInputRatio) {
				return adaptor.RequestPricingNotConfigured(errors.New("Vertex Argon Cyber has invalid explicit tier cache pricing"))
			}
		}
	case requiresClaude52Pricing(name):
		cfg, found := pricing.ResolveModelConfigRatioOnly(name, configs, nil, at)
		if !found || !valid(cfg.CachedInputRatio) || !valid(cfg.CacheWrite5mRatio) || !valid(cfg.CacheWrite1hRatio) {
			return adaptor.RequestPricingNotConfigured(errors.New("Vertex Claude 5.2 requires explicit positive cache read, 5m write, and 1h write pricing in channel model_configs"))
		}
		for _, tier := range cfg.Tiers {
			// Zero tier fields inherit the complete base tariff, matching billing.
			for _, v := range []float64{tier.CachedInputRatio, tier.CacheWrite5mRatio, tier.CacheWrite1hRatio} {
				if v != 0 && !valid(v) {
					return adaptor.RequestPricingNotConfigured(errors.New("Vertex Claude 5.2 has invalid explicit tier cache pricing"))
				}
			}
		}
	}
	return nil
}
