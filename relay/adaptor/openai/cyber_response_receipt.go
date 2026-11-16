package openai

import (
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

// cyberResponseReceipt observes the original response before presentation
// converters discard envelope pricing dimensions or failed-response usage.
// Its bounded accumulator is protected from a stream reader finishing late.
type cyberResponseReceipt struct {
	io.ReadCloser
	mu          sync.Mutex
	accumulator *model.ResponseUsageAccumulator
	stream      bool
}

// observeCyberResponseReceipt wraps resp for the native Cyber model selected by
// info and returns its bounded observer. Other models and hosts return nil.
func observeCyberResponseReceipt(resp *http.Response, info *meta.Meta) *cyberResponseReceipt {
	if info == nil || info.ChannelType != channeltype.OpenAI ||
		!strings.EqualFold(strings.TrimSpace(info.ActualModelName), "gpt-6-cyber") || resp == nil || resp.Body == nil {
		return nil
	}
	eventStream := strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")
	observer := &cyberResponseReceipt{ReadCloser: resp.Body, stream: eventStream || info.IsStream,
		accumulator: model.NewResponseUsageAccumulator(eventStream)}
	resp.Body = observer
	return observer
}

// Read copies upstream bytes into p, observes the same bytes for billing, and
// returns the original read result without changing the client-visible body.
func (r *cyberResponseReceipt) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	r.mu.Lock()
	defer r.mu.Unlock()
	if n > 0 {
		r.accumulator.Observe(p[:n])
	}
	// JSON is parsed atomically, so an incomplete envelope already fails its
	// receipt check. SSE needs an explicit interruption marker for partial events.
	if r.stream && err != nil && !errors.Is(err, io.EOF) {
		r.accumulator.MarkIncomplete()
	}
	return n, err
}

// finish preserves the observed Cyber pricing receipt alongside rendered usage.
// Parameters: c carries converted Claude usage, usage is the handler result,
// and convertedBody keeps the controller's deferred body-rendering contract.
// Returns: the canonical settlement usage, or nil for an unrendered Claude body
// or an error response without any usage evidence.
func (r *cyberResponseReceipt) finish(c *gin.Context, usage *model.Usage, convertedBody bool) *model.Usage {
	r.mu.Lock()
	observed := r.accumulator.Finish(0, nil)
	r.mu.Unlock()
	if usage == nil && c.GetBool(ctxkey.ClaudeMessagesConversion) {
		if stored, exists := c.Get(ctxkey.ConvertedResponseUsage); exists {
			usage, _ = stored.(*model.Usage)
		}
	}
	captureLimited := observed.BillingEstimateReason == "response_usage_capture_limit"
	if captureLimited {
		// The accumulator's byte fallback describes an incomplete envelope,
		// which may include large non-token fields. Retain the handler's
		// measured counters and reservation instead of charging JSON bytes.
		observed.PromptTokens, observed.CompletionTokens, observed.TotalTokens = 0, 0, 0
	}
	// A service tier or an error message alone does not prove paid work. Keep
	// explicit provider errors without counters on the existing refund path.
	if usage == nil && observed.TotalTokens == 0 && observed.ToolsCost == 0 &&
		observed.CacheWrite5mTokens == 0 && observed.CacheWrite1hTokens == 0 && observed.CacheWrite12hTokens == 0 && !captureLimited {
		return nil
	}
	var receipt model.Usage
	if usage != nil {
		receipt = *usage // Preserve server-only floors and other handler evidence.
	}
	if observed.BillingEstimateReason == "" || usage == nil {
		receipt.PromptTokens = observed.PromptTokens
		receipt.CompletionTokens = observed.CompletionTokens
	} else {
		receipt.PromptTokens = max(receipt.PromptTokens, observed.PromptTokens)
		receipt.CompletionTokens = max(receipt.CompletionTokens, observed.CompletionTokens)
	}
	receipt.TotalTokens = receipt.PromptTokens + receipt.CompletionTokens
	if observed.PromptTokensDetails != nil {
		receipt.PromptTokensDetails = observed.PromptTokensDetails
	}
	if observed.CompletionTokensDetails != nil {
		receipt.CompletionTokensDetails = observed.CompletionTokensDetails
	}
	receipt.CacheWrite5mTokens = max(receipt.CacheWrite5mTokens, observed.CacheWrite5mTokens)
	receipt.CacheWrite1hTokens = max(receipt.CacheWrite1hTokens, observed.CacheWrite1hTokens)
	receipt.CacheWrite12hTokens = max(receipt.CacheWrite12hTokens, observed.CacheWrite12hTokens)
	receipt.ToolsCost = max(receipt.ToolsCost, observed.ToolsCost)
	if observed.ServiceTier != "" {
		receipt.ServiceTier = observed.ServiceTier
	}
	if observed.SystemFingerprint != "" {
		receipt.SystemFingerprint = observed.SystemFingerprint
	}
	if receipt.BillingEstimateReason == "" {
		receipt.BillingEstimateReason = observed.BillingEstimateReason
	}
	if c.GetBool(ctxkey.ClaudeMessagesConversion) {
		openai_compatible.SetConvertedClaudeUsage(c, &receipt)
	}
	if convertedBody {
		return nil
	}
	return &receipt
}
