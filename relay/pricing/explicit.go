package pricing

import (
	"math"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/meta"
)

// RequireExplicitTokenPricing admits an exceptional model only when the channel
// supplies a complete, positive token tariff for its actual mapped model. It
// uses the same request-start time-window resolver as billing, never a guessed
// provider price or another provider's global fallback. It does not charge quota.
func RequireExplicitTokenPricing(c *gin.Context, m *meta.Meta) error {
	if c == nil || m == nil {
		return errors.New("explicit token pricing requires request and channel metadata")
	}
	name := m.ActualModelName
	if name == "" {
		name = m.OriginModelName
	}
	missing := func() error {
		return errors.Errorf("model %q requires explicit positive input and output pricing in channel model_configs", name)
	}
	value, ok := c.Get(ctxkey.ChannelModel)
	if !ok {
		return missing()
	}
	channel, ok := value.(*model.Channel)
	if !ok || channel == nil {
		return missing()
	}
	configs := channel.GetModelPriceConfigsWithContext(gmw.Ctx(c))
	if _, ok := configs[name]; !ok {
		return missing()
	}
	cfg, found := ResolveModelConfigRatioOnly(name, configs, nil, m.StartTime)
	if !found || validateExplicitTokenTariff(cfg) != nil {
		return missing()
	}
	return nil
}

// validateExplicitTokenTariff validates the entire active tariff, including
// output-dependent tiers not selectable until completion. Exceptional models
// require complete positive tier prices rather than ambiguous zero sentinels.
func validateExplicitTokenTariff(cfg adaptor.ModelConfig) error {
	valid := func(input, outputMultiplier float64) bool {
		return positiveFinitePrice(input) && positiveFinitePrice(outputMultiplier) && positiveFinitePrice(input*outputMultiplier)
	}
	if !valid(cfg.Ratio, cfg.CompletionRatio) {
		return errors.New("explicit token tariff has invalid input or output prices")
	}
	for _, tier := range cfg.Tiers {
		if !valid(tier.Ratio, tier.CompletionRatio) {
			return errors.New("explicit token tariff has an incomplete or invalid tier")
		}
	}
	return nil
}

// positiveFinitePrice excludes zero, negative, nonfinite, overflow and underflow
// values from the admission calculation without changing configured valid rates.
func positiveFinitePrice(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}
