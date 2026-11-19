package ali

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

// RequiresOperatorTariff reports whether modelName is a legacy or trial catalog
// entry without a customer tariff. Parameters: modelName is the mapped model.
// Returns: true when only explicit channel input/output prices may bill it.
func (a *Adaptor) RequiresOperatorTariff(modelName string) bool {
	switch modelName {
	case "qwen-audio-chat", "qwen-audio-turbo", "qwen2.5-0.5b-instruct", "qwen2.5-1.5b-instruct", "qwen2.5-math-1.5b-instruct", "qwen2-audio-instruct":
		return true
	default:
		return false
	}
}

// ValidateRequestPricing separates provider trial availability from customer
// tariff policy. These legacy/trial catalog entries require explicit channel
// input/output prices instead of a fabricated public price or an implicit free
// request. Normal priced models keep the existing provider defaults.
func (a *Adaptor) ValidateRequestPricing(c *gin.Context, m *meta.Meta) error {
	if m == nil {
		return errors.New("Ali request pricing requires model metadata")
	}
	return pricing.ValidateOperatorTariff(c, m, a)
}
