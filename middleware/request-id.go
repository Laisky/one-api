package middleware

import (
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/identity"
)

// RequestId assigns a per-request id, echoes it back as a response header, and
// binds it onto the request-scoped logger.
//
// It also captures the pristine request logger as the rebuild base for
// identity.Bind, so that later identity bindings (auth, channel selection, every
// relay retry) REPLACE the identity fields instead of appending duplicates.
func RequestId() func(c *gin.Context) {
	return func(c *gin.Context) {
		id := helper.GenRequestID()
		c.Set(helper.RequestIdKey, id)
		c.Header(helper.RequestIdKey, id)
		identity.BindBase(c, zap.String("request_id", id))
		c.Next()
	}
}
