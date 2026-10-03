package groq

import (
	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/pricing"
)

var _ adaptor.RequestPricingValidator = (*Adaptor)(nil)

// ValidateRequestPricing prevents retired Compound models and unpriced
// enterprise models from reaching Groq. MiniMax's direct API price is not a Groq
// contract tariff. Current public-rate models retain their existing behavior.
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
	case "minimaxai/minimax-m2.7":
		return pricing.RequireExplicitTokenPricing(c, m)
	default:
		return nil
	}
}
