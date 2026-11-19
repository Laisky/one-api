package adaptor

import (
	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/meta"
)

// ErrRequestPricingNotConfigured marks a dispatch refused by a
// RequestPricingValidator because the channel lacks a tariff the provider
// requires. It is a channel configuration fault, not an upstream failure: no
// upstream was contacted, another channel may still be priced, and a final
// refusal is answered as a billing-configuration error (HTTP 503). Validators
// mark only tariff refusals (RequestPricingNotConfigured); other refusals, such
// as a retired model or missing request metadata, keep their own semantics.
var ErrRequestPricingNotConfigured = errors.New("request pricing is not configured for this channel")

// requestPricingError carries a validator's own message while matching
// ErrRequestPricingNotConfigured through errors.Is.
type requestPricingError struct {
	err error
}

// Error returns the validator's message. It takes no parameters.
func (e *requestPricingError) Error() string { return e.err.Error() }

// Unwrap exposes the validator's error. It takes no parameters.
func (e *requestPricingError) Unwrap() error { return e.err }

// Is reports whether target is ErrRequestPricingNotConfigured. Parameters:
// target is the sentinel being matched. Returns: true for that sentinel.
func (e *requestPricingError) Is(target error) bool { return target == ErrRequestPricingNotConfigured }

// RequestPricingNotConfigured marks err as a missing or incomplete channel
// tariff so callers can match it with IsRequestPricingNotConfigured while the
// client still sees err's own message. Parameters: err is the validator's
// refusal and may be nil. Returns: the marked error, or nil for a nil err.
func RequestPricingNotConfigured(err error) error {
	if err == nil {
		return nil
	}
	return &requestPricingError{err: err}
}

// ValidateRequestPricing runs provider's RequestPricingValidator, if any, so
// every caller (REST dispatch, Claude Messages local admission) applies the
// same validation. Parameters: provider is the serving adaptor, c carries the
// channel, and m the mapped model. Returns: nil when dispatch may proceed, else
// the wrapped refusal, which keeps any ErrRequestPricingNotConfigured mark.
func ValidateRequestPricing(provider Adaptor, c *gin.Context, m *meta.Meta) error {
	validator, ok := provider.(RequestPricingValidator)
	if !ok {
		return nil
	}
	if err := validator.ValidateRequestPricing(c, m); err != nil {
		return errors.Wrap(err, "validate request pricing")
	}
	return nil
}

// IsRequestPricingNotConfigured reports whether err is a pre-dispatch tariff
// refusal from a RequestPricingValidator. Parameters: err may be nil or
// wrapped. Returns: true when the refusal is a channel pricing configuration fault.
func IsRequestPricingNotConfigured(err error) bool {
	return err != nil && errors.Is(err, ErrRequestPricingNotConfigured)
}

// RequestPricingValidator optionally enforces a provider's tariff admission
// policy before REST dispatch. Returning an error guarantees DoRequestHelper has
// not contacted the upstream or marked the request as possibly forwarded.
// Implementations must not charge quota; controllers own reservation/settlement.
type RequestPricingValidator interface {
	ValidateRequestPricing(c *gin.Context, m *meta.Meta) error
}

// OperatorTariffPolicy optionally declares models a provider can serve but has
// no provider-qualified tariff for. Pricing distinguishes three cases: a model
// with a verified provider tariff (adaptor defaults), a model explicitly priced
// by the operator (channel model_configs), and a model with no provider-qualified
// tariff. For the last case, declared here, pricing resolution never borrows
// provider defaults or another provider's global tariff, and the provider must
// reject dispatch unless the channel carries a complete operator tariff.
type OperatorTariffPolicy interface {
	// RequiresOperatorTariff reports whether modelName is billable only through
	// an operator-configured channel tariff on this provider.
	RequiresOperatorTariff(modelName string) bool
}

// RequiresOperatorTariff reports whether provider declares modelName as payable
// only through an operator-configured channel tariff. Parameters: provider is the
// pricing adaptor and may be nil; modelName is the mapped model ID. Returns:
// false for nil providers and providers that do not implement OperatorTariffPolicy.
func RequiresOperatorTariff(provider Adaptor, modelName string) bool {
	if provider == nil {
		return false
	}
	policy, ok := provider.(OperatorTariffPolicy)
	return ok && policy.RequiresOperatorTariff(modelName)
}
