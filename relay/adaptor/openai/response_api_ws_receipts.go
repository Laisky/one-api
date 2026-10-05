package openai

import (
	"encoding/json"
	"strings"

	rmodel "github.com/Laisky/one-api/relay/model"
)

// isResponseAPIWSTerminalReceipt requires a terminal event and a compatible
// terminal status before usage or ownership can become authoritative. Omitted
// status remains compatible with legacy terminal receipts and existing bridges.
func isResponseAPIWSTerminalReceipt(eventType, status string) bool {
	status = strings.ToLower(strings.TrimSpace(status))
	return isTerminalStreamEventType(eventType) &&
		(status == "" || isTerminalResponseStatus(status))
}

// responseAPIWSUsageCollector tracks unfinished responses separately from IDs
// whose terminal receipts have already contributed measured usage.
type responseAPIWSUsageCollector struct {
	usage   *rmodel.Usage
	counted map[string]struct{}
	pending map[string]struct{}
}

// collect records qualified terminal usage once, retaining per-ID uncertainty
// until that same response produces a terminal usage receipt.
func (c *responseAPIWSUsageCollector) collect(msg []byte) {
	if c.usage == nil {
		return
	}
	var event responseAPIWebSocketEvent
	if err := json.Unmarshal(msg, &event); err != nil || event.Response == nil || event.Response.ID == "" {
		return
	}
	id := event.Response.ID
	if _, exists := c.counted[id]; exists {
		return
	}
	if c.counted == nil {
		c.counted = make(map[string]struct{})
	}
	accumulateResponseAPIUsage(msg, c.usage, c.counted)
	if _, exists := c.counted[id]; exists {
		delete(c.pending, id)
		return
	}
	if c.pending == nil {
		c.pending = make(map[string]struct{})
	}
	c.pending[id] = struct{}{}
}

// finish preserves the reservation when any observed response still lacks an
// authoritative usage receipt, even if a different response completed.
func (c *responseAPIWSUsageCollector) finish() {
	if c.usage != nil && len(c.pending) > 0 {
		c.usage.BillingEstimateReason = "response_stream_incomplete_or_missing_receipt"
	}
}
