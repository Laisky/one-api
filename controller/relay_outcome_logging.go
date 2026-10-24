package controller

import (
	"context"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	rcontroller "github.com/Laisky/one-api/relay/controller"
	"github.com/Laisky/one-api/relay/model"
)

// relayOutcomeObservation records helper invocations and the existing retry decision without changing dispatch or settlement.
type relayOutcomeObservation struct {
	attempts             int
	failure              *model.ErrorWithStatusCode
	retryOutcome         string
	errorResponseWritten bool
}

// invoke delegates one helper invocation and emits local metadata; an invocation is not proof of a wire attempt or accepted inference.
func (o *relayOutcomeObservation) invoke(c *gin.Context, mode int) *model.ErrorWithStatusCode {
	o.attempts++
	failure := invokeRelayHelper(c, mode)
	outcome := "succeeded"
	if failure != nil {
		outcome = "failed"
		o.retryOutcome = "budget_exhausted"
	} else if o.attempts > 1 {
		o.retryOutcome = "succeeded"
	}
	lg := gmw.GetLogger(c)
	lg.Debug("relay attempt completed", zap.String("log_stage", "relay_attempt"), zap.Int("relay_attempt", o.attempts), zap.String("attempt_outcome", outcome))
	return failure
}

// logFinal records the controller outcome and actual writer state, separately from asynchronous accounting or client receipt.
func (o *relayOutcomeObservation) logFinal(c *gin.Context) {
	lg := gmw.GetLogger(c)
	outcome := "helper_succeeded"
	if o.failure != nil {
		outcome = "helper_failed"
	}
	clientOutcome := "uncommitted"
	fields := []zap.Field{zap.String("log_stage", "relay_final"), zap.String("relay_outcome", outcome), zap.String("retry_outcome", o.retryOutcome), zap.Int("relay_attempts", o.attempts), zap.Bool("billing_reconciled_flag", c.GetBool(ctxkey.BillingReconciled)), zap.Bool("upstream_request_possibly_forwarded", c.GetBool(ctxkey.UpstreamRequestPossiblyForwarded))}
	if c.Writer.Written() {
		clientOutcome = "response_written"
		if o.failure != nil {
			clientOutcome = "committed_response_preserved"
		}
		if o.errorResponseWritten {
			clientOutcome = "error_response_written"
		}
		fields = append(fields, zap.Int("client_status_code", c.Writer.Status()))
	}
	fields = append(fields, zap.String("client_outcome", clientOutcome))
	if o.failure != nil {
		fields = append(fields, zap.Int("normalized_status_code", o.failure.StatusCode))
	}
	// Initial successful traffic retains DEBUG volume; failure and recovery outcomes remain available at INFO export thresholds.
	if o.failure != nil || o.attempts > 1 {
		lg.Info("relay request completed", fields...)
	} else {
		lg.Debug("relay request completed", fields...)
	}
}

// relayObservedRetryStop describes existing loop guards without changing their decisions or interpreting billing flags as durable settlement.
func relayObservedRetryStop(c *gin.Context, allowed bool) string {
	if !allowed || c.GetInt(ctxkey.SpecificChannelId) != 0 {
		return "policy_veto"
	}
	if videoReplayUnsafe(c) {
		return "accepted_work"
	}
	if !rcontroller.BillingAllowsRetry(c) {
		if c.Writer.Written() {
			return "output_committed"
		}
		return "billing_veto"
	}
	if c.Request.Context().Err() == context.Canceled {
		return "request_canceled"
	}
	if c.Request.Context().Err() == context.DeadlineExceeded {
		return "request_deadline"
	}
	return "budget_exhausted"
}
