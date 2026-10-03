package adaptor

import (
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/meta"
)

// RequestPricingValidator optionally enforces a provider's tariff admission
// policy before REST dispatch. Returning an error guarantees DoRequestHelper has
// not contacted the upstream or marked the request as possibly forwarded.
// Implementations must not charge quota; controllers own reservation/settlement.
type RequestPricingValidator interface {
	ValidateRequestPricing(c *gin.Context, m *meta.Meta) error
}
