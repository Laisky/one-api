package azure

import (
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/pricing"
)

var (
	_ adaptor.OperatorTariffPolicy    = (*Adaptor)(nil)
	_ adaptor.RequestPricingValidator = (*Adaptor)(nil)
)

// RequiresOperatorTariff reports whether modelName is a native-only OpenAI model
// for which Foundry publishes no contract. Parameters: modelName is the mapped
// deployment model. Returns: true when only a channel tariff may bill it.
func (a *Adaptor) RequiresOperatorTariff(modelName string) bool {
	return openai.IsNativeOnlyModel(modelName)
}

// ValidateRequestPricing rejects dispatch of a native-only OpenAI model unless
// the channel supplies a complete operator tariff. Parameters: c carries the
// channel and m the mapped model. Returns: nil when dispatch may proceed.
func (a *Adaptor) ValidateRequestPricing(c *gin.Context, m *meta.Meta) error {
	return pricing.ValidateOperatorTariff(c, m, a)
}
