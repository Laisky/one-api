package xai

import (
	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/pricing"
)

var _ adaptor.RequestPricingValidator = (*Adaptor)(nil)

// ValidateRequestPricing prevents unverified historical slugs from silently
// using generic fallback prices. Current documented redirects retain their
// existing provider tariffs; legacy deployments can supply an explicit channel
// contract without guessing that a Fast model shares the ordinary model's rate.
func (a *Adaptor) ValidateRequestPricing(c *gin.Context, m *meta.Meta) error {
	if m == nil {
		return errors.New("xAI request pricing requires model metadata")
	}
	name := m.ActualModelName
	if name == "" {
		name = m.OriginModelName
	}
	switch name {
	case "grok-3-fast", "grok-3-mini-fast", "grok-2-1212", "grok-beta", "grok-2", "grok-2-latest", "grok-vision-beta":
		return pricing.RequireExplicitTokenPricing(c, m)
	default:
		return nil
	}
}
