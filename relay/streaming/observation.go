package streaming

import (
	"context"
	"encoding/json"
	"github.com/Laisky/errors/v2"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/gin-gonic/gin"
)

const protocolObserverKey = "one_api.streaming.protocol_observer"
const upstreamCancelKey = "one_api.streaming.cancel_upstream"

// ClaimProtocolObservation identifies the provider's own pre-presentation
// accounting path. The fallback bridge must not account the same frame again.
func ClaimProtocolObservation(c *gin.Context) { c.Set(protocolObserverKey, true) }

// HasProtocolObservation reports a request-local provider accounting owner.
func HasProtocolObservation(c *gin.Context) bool { return c.GetBool(protocolObserverKey) }

// BindUpstreamCancellation retains a value-only cancel function for bridge aborts.
func BindUpstreamCancellation(c *gin.Context, cancel context.CancelFunc) {
	c.Set(upstreamCancelKey, cancel)
}

// StopUpstream cancels provider I/O without changing the final settlement owner.
func StopUpstream(c *gin.Context) {
	if value, ok := c.Get(upstreamCancelKey); ok {
		if cancel, ok := value.(context.CancelFunc); ok {
			cancel()
		}
	}
}

// CheckAffordability atomically funds newly observed cumulative receipt usage.
func (t *QuotaTracker) CheckAffordability() error { return t.maybeFlush(true) }

// UsageSnapshot returns evidence without billing or mutating the tracker.
func (t *QuotaTracker) UsageSnapshot() *relaymodel.Usage {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.currentUsageLocked()
}

// ObserveMessages funds a complete parsed frame before delivery. A same-frame
// complete measured receipt is authoritative; otherwise count raw text/reasoning/
// tool dimension once with the controller-selected tokenizer, not a byte average.
func (t *QuotaTracker) ObserveMessages(messages []relaymodel.Message, usage *relaymodel.Usage, counters ...func(string, string) int) error {
	if usage != nil && usage.BillingEstimateReason == "" {
		t.UpdateFinalUsage(usage)
		return t.maybeFlush(true)
	}
	count := t.params.TokenCounter
	if len(counters) > 0 {
		count = counters[0]
	}
	if count == nil {
		return errors.New("streaming tokenizer is not configured")
	}
	delta := 0
	for _, message := range messages {
		delta += count(message.StringContent(), t.params.ModelName)
		seen := map[string]bool{}
		for _, part := range []*string{message.ReasoningContent, message.Reasoning, message.Thinking} {
			if part != nil && !seen[*part] {
				seen[*part] = true
				delta += count(*part, t.params.ModelName)
			}
		}
		for _, tool := range message.ToolCalls {
			if tool.Function != nil {
				var text string
				switch value := tool.Function.Arguments.(type) {
				case string:
					text = value
				case nil:
				default:
					raw, err := json.Marshal(value)
					if err != nil {
						return err
					}
					text = string(raw)
				}
				delta += count(text, t.params.ModelName)
			}
		}
	}
	// Partial receipts cannot suppress output in their own frame. Retain both
	// observations even when enforcement fails, for final controller settlement.
	recordErr := t.RecordCompletionTokens(delta)
	if usage != nil {
		t.UpdateFinalUsage(usage)
		if recordErr == nil {
			return t.maybeFlush(true)
		}
	}
	return recordErr
}
