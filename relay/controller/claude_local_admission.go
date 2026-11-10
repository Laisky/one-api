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

// validateClaudeRequestPricing applies the provider's shared tariff contract before quota admission.
func validateClaudeRequestPricing(c *gin.Context, m *meta.Meta, a adaptor.Adaptor) *relaymodel.ErrorWithStatusCode {
	if validator, ok := a.(adaptor.RequestPricingValidator); ok {
		if err := validator.ValidateRequestPricing(c, m); err != nil {
			return openai.ErrorWrapper(err, "invalid_model_pricing", http.StatusBadRequest)
		}
	}
	return nil
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
