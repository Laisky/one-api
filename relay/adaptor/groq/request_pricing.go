package groq

import (
	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/pricing"
)

var (
	_ adaptor.RequestPricingValidator = (*Adaptor)(nil)
	_ adaptor.OperatorTariffPolicy    = (*Adaptor)(nil)
)

// RequiresOperatorTariff reports whether modelName has no Groq contract tariff.
// MiniMax's direct API price is not a Groq tariff. Parameters: modelName is the
// mapped model. Returns: true when only an explicit channel tariff may bill it.
func (a *Adaptor) RequiresOperatorTariff(modelName string) bool {
	return modelName == "minimaxai/minimax-m2.7"
}

// ValidateRequestPricing prevents retired Compound models and unpriced
// enterprise models from reaching Groq. Current public-rate models retain their
// existing behavior.
// Source: https://console.groq.com/docs/deprecations (Compound: 2026-09-21).
func (a *Adaptor) ValidateRequestPricing(c *gin.Context, m *meta.Meta) error {
	if m == nil {
		return errors.New("Groq request pricing requires model metadata")
	}
	name := m.ActualModelName
	if name == "" {
		name = m.OriginModelName
	}
	switch name {
	case "groq/compound", "groq/compound-mini":
		return errors.Errorf("Groq model %q is retired; select a currently supported model", name)
	default:
		return pricing.ValidateOperatorTariff(c, m, a)
	}
}
