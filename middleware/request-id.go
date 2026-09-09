package middleware

import (
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/identity"
	"github.com/Laisky/one-api/common/logger/otelbridge"
)

// RequestId assigns a per-request id, echoes it back as a response header, and
// binds it onto the request-scoped logger.
//
// It also captures the pristine request logger as the rebuild base for
// identity.Bind, so that later identity bindings (auth, channel selection, every
// relay retry) REPLACE the identity fields instead of appending duplicates.
//
// It is additionally where OpenTelemetry trace correlation is bound onto the
// request logger (proposal Phase 3 / W3.2, "Test actual exported trace/span IDs
// from gmw.GetLogger(c) in the request path"). This is the right place for two
// reasons: it runs after otelgin has started the request span, so a span
// context exists; and it is already the single point that rebuilds the base
// logger, so every later gmw.GetLogger(c) call inherits the binding with no
// edit at the call site.
//
// The correlation field is invisible to the file and stdout sinks -- see
// otelbridge.SpanContextField -- so application log lines are byte-identical
// whether or not the bridge is enabled.
func RequestId() func(c *gin.Context) {
	// APP_LOG_SINK is startup configuration, so the decision is made once here
	// rather than per request. Binding the correlation field measured at +192
	// B/op and +1 alloc/op on the logger rebuild; that is small, but it is paid
	// by every request of every deployment, and a deployment that never enabled
	// the bridge gets nothing for it. Section 2.1's unchanged-configuration
	// contract is the reason this is a gate and not an unconditional field.
	correlate := config.AppLogOTLPEnabled

	return func(c *gin.Context) {
		id := helper.GenRequestID()
		c.Set(helper.RequestIdKey, id)
		c.Header(helper.RequestIdKey, id)

		if correlate {
			identity.BindBase(c,
				zap.String("request_id", id),
				otelbridge.SpanContextField(gmw.Ctx(c)),
			)
		} else {
			identity.BindBase(c, zap.String("request_id", id))
		}
		c.Next()
	}
}
