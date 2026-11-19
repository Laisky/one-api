package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// validateClaudeRequestPricing applies the provider's shared tariff contract
// before quota admission. A missing or incomplete channel tariff is a channel
// configuration fault, not a client error: it answers 503
// model_pricing_not_configured so the relay loop fails over to another
// (possibly priced) channel and never penalises channel health, matching the
// Chat Completions and Responses dispatch contract. Other validator refusals
// (a retired model, missing metadata) keep the historical 400. Parameters: c
// carries the channel, m the mapped model, a the serving adaptor. Returns: the
// refusal, or nil when admission may proceed.
func validateClaudeRequestPricing(c *gin.Context, m *meta.Meta, a adaptor.Adaptor) *relaymodel.ErrorWithStatusCode {
	err := adaptor.ValidateRequestPricing(a, c, m)
	switch {
	case err == nil:
		return nil
	case adaptor.IsRequestPricingNotConfigured(err):
		return openai.ErrorWrapper(err, "model_pricing_not_configured", http.StatusServiceUnavailable)
	default:
		return openai.ErrorWrapper(err, "invalid_model_pricing", http.StatusBadRequest)
	}
}

// settleUnforwardedClaudeReservation handles terminal local preparation failures before the audit fallback.
// It never refunds possibly forwarded work, submitted response settlement, or an already-owned reservation.
func settleUnforwardedClaudeReservation(c *gin.Context, amount int64, tokenID int) {
	if c == nil || amount <= 0 || c.GetBool(ctxkey.BillingReconciled) ||
		c.GetBool(ctxkey.UpstreamRequestPossiblyForwarded) || c.GetBool(responseSettlementKey) {
		return
	}
	refundAdmissionReservation(c, amount, tokenID, "claude_local_admission_rejected")
}
