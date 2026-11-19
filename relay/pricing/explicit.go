package pricing

import (
	"math"
	"time"

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
	value, ok := c.Get(ctxkey.ChannelModel)
	if !ok {
		return missingExplicitTokenTariff(name)
	}
	channel, ok := value.(*model.Channel)
	if !ok || channel == nil {
		return missingExplicitTokenTariff(name)
	}
	return ValidateExplicitTokenTariffConfigs(name, channel.GetModelPriceConfigsWithContext(gmw.Ctx(c)), m.StartTime)
}

// ValidateExplicitTokenTariffConfigs is the request-free core of
// RequireExplicitTokenPricing: it checks that configs, a channel's parsed
// model_configs, carries a complete positive token tariff for name at time at,
// using only layer 1 (no provider defaults or global fallback). The channel
// editor uses it to warn before an operator saves a channel that dispatch
// would refuse. Parameters: name is the mapped model, configs may be nil, and
// at selects active time windows. Returns: nil when the tariff is complete, or
// an error matching adaptor.ErrRequestPricingNotConfigured.
func ValidateExplicitTokenTariffConfigs(name string, configs map[string]model.ModelConfigLocal, at time.Time) error {
	if _, ok := configs[name]; !ok {
		return missingExplicitTokenTariff(name)
	}
	cfg, found := ResolveModelConfigRatioOnly(name, configs, nil, at)
	if !found || validateExplicitTokenTariff(cfg) != nil {
		return missingExplicitTokenTariff(name)
	}
	return nil
}

// missingExplicitTokenTariff builds the refusal shared by every explicit
// token-tariff check. Parameters: name is the mapped model. Returns: an error
// that names the model and the channel field to configure and matches
// adaptor.ErrRequestPricingNotConfigured.
func missingExplicitTokenTariff(name string) error {
	return adaptor.RequestPricingNotConfigured(
		errors.Errorf("model %q requires explicit positive input and output pricing in channel model_configs", name))
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

// ValidateOperatorTariff enforces adaptor.OperatorTariffPolicy before paid
// dispatch. Parameters: c carries the selected channel, m identifies the mapped
// model, and provider is the dispatching adaptor. Returns: nil when the provider
// has a qualified tariff for the model, otherwise RequireExplicitTokenPricing's
// verdict, so an unpriced model never reaches the upstream.
func ValidateOperatorTariff(c *gin.Context, m *meta.Meta, provider adaptor.Adaptor) error {
	if m == nil {
		return errors.New("operator tariff validation requires model metadata")
	}
	name := m.ActualModelName
	if name == "" {
		name = m.OriginModelName
	}
	if !adaptor.RequiresOperatorTariff(provider, name) {
		return nil
	}
	return RequireExplicitTokenPricing(c, m)
}

// OperatorTariffSupplementValidator is optionally implemented by an
// adaptor.OperatorTariffPolicy provider whose dispatch requires channel prices
// beyond the complete token tariff, such as explicit cache read and write
// rates. It is request-free so the channel editor can warn before save with the
// same rule dispatch enforces.
type OperatorTariffSupplementValidator interface {
	// ValidateOperatorTariffSupplement checks the provider-specific fields of
	// name's channel tariff in configs at time at, assuming the token tariff was
	// already validated, and returns nil when dispatch would admit it or an
	// error matching adaptor.ErrRequestPricingNotConfigured otherwise.
	ValidateOperatorTariffSupplement(name string, configs map[string]model.ModelConfigLocal, at time.Time) error
}
