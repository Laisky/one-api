package controller

import (
	stderrors "errors"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/model"
)

// deliveryTracker records the first failed or short write to the downstream
// client during one relay attempt. Adaptors differ in how they report writer
// results: some return a provider-shaped error and some ignore the result
// entirely. Observing the writer at the relay boundary gives every adaptor the
// same delivery-failure attribution without trusting its error shape.
type deliveryTracker struct {
	gin.ResponseWriter
	err error
}

// Write forwards p and retains the first failed or short downstream write.
func (w *deliveryTracker) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	if err != nil && w.err == nil {
		w.err = err
	}
	return n, err
}

// WriteString routes string writes through the same checked boundary as Write.
func (w *deliveryTracker) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

// Unwrap exposes the wrapped writer to http.ResponseController so deadline and
// flush controls keep reaching the underlying connection.
func (w *deliveryTracker) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// trackDelivery installs a delivery tracker for one relay attempt.
//
// Parameters:
//   - c: the request context whose writer is observed.
//
// Returns:
//   - *deliveryTracker: the installed tracker.
//   - func(): restores the previous writer when the tracker is still installed.
func trackDelivery(c *gin.Context) (*deliveryTracker, func()) {
	tracker := &deliveryTracker{ResponseWriter: c.Writer}
	c.Writer = tracker
	return tracker, func() {
		if current, ok := c.Writer.(*deliveryTracker); ok && current == tracker {
			c.Writer = tracker.ResponseWriter
		}
	}
}

// attributeDeliveryFailure reports a failed downstream write as a delivery
// failure. A helper that concealed the failure behind a success is turned into
// a failure, because the client did not receive the complete response; a
// helper failure caused by the recorded write is re-attributed from the
// provider to the gateway, so a broken client connection never penalizes the
// channel's health. Settlement already performed by the helper is unchanged:
// output was committed, so the relay loop cannot replay the attempt.
//
// Parameters:
//   - c: the request context; its cancellation is preserved in the cause.
//   - tracker: the attempt's delivery tracker.
//   - failure: the helper result; nil means the helper reported success.
//
// Returns:
//   - *model.ErrorWithStatusCode: the attempt result with delivery attribution.
func attributeDeliveryFailure(c *gin.Context, tracker *deliveryTracker, failure *model.ErrorWithStatusCode) *model.ErrorWithStatusCode {
	if tracker == nil || tracker.err == nil || model.IsDeliveryFailure(failure) {
		return failure
	}
	code := model.DeliveryCodeResponse
	if strings.HasPrefix(c.Writer.Header().Get("Content-Type"), "text/event-stream") {
		code = model.DeliveryCodeStream
	}
	if failure == nil {
		cause := tracker.err
		if c.Request != nil {
			if canceled := c.Request.Context().Err(); canceled != nil {
				// Keep the caller's cancellation identity so the failure is
				// reported as a client abort rather than a gateway fault.
				cause = stderrors.Join(cause, canceled)
			}
		}
		gmw.GetLogger(c).Debug("relay helper concealed a downstream delivery failure",
			zap.String("delivery_code", code))
		return model.NewDeliveryError(cause, code)
	}
	if isClientContextCancel(failure.StatusCode, failure.RawError) || !errors.Is(failure.RawError, tracker.err) {
		return failure
	}
	gmw.GetLogger(c).Debug("relay failure caused by a downstream write re-attributed to delivery",
		zap.String("delivery_code", code),
		zap.String("adaptor_error_code", strings.TrimSpace(errorCodeString(failure))),
		zap.Int("adaptor_status_code", failure.StatusCode))
	return model.NewDeliveryError(failure.RawError, code)
}

// errorCodeString renders a relay error code for diagnostics.
func errorCodeString(failure *model.ErrorWithStatusCode) string {
	if failure == nil || failure.Code == nil {
		return ""
	}
	if code, ok := failure.Code.(string); ok {
		return code
	}
	return ""
}
