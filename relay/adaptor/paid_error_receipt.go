package adaptor

import (
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

// MaxPaidErrorReceiptBytes bounds the prefix of a non-2xx upstream body that is
// inspected for a billable usage receipt. Provider error envelopes are small;
// a larger body is never parsed, so a hostile upstream cannot force unbounded
// buffering, and it keeps the existing retained-hold failure path.
const MaxPaidErrorReceiptBytes = 1 << 20

// PaidErrorReceiptReader is implemented by adaptors whose provider can report
// billable work in a non-2xx response. Controllers consult it with a bounded,
// restored copy of the error body before refunding or replaying the attempt:
// a receipt turns the attempt into a settled, non-replayable charge, while nil
// keeps the provider error on the generic retained-hold and retry path.
type PaidErrorReceiptReader interface {
	// PaidErrorReceipt returns the usage to settle for a non-2xx response, or
	// nil when the body proves no billable work. Like DoResponse, it records any
	// billable built-in tool invocations the receipt lists on c, so tool fees
	// settle exactly as for a 2xx response. Parameters: c is the attempt's
	// context, m its relay metadata, statusCode the upstream HTTP status, and
	// body the bounded error body, which implementations must not retain or
	// mutate.
	PaidErrorReceipt(c *gin.Context, m *meta.Meta, statusCode int, body []byte) *model.Usage
}
