package model

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"github.com/Laisky/errors/v2"
)

const responseUsageCaptureLimit = 2 << 20
const responseUsageTextLimit = 64 << 10

// ResponseUsageAccumulator observes one response without retaining its entire
// stream. It is request-local and must not be shared between goroutines.
type ResponseUsageAccumulator struct {
	incomplete                          bool
	stream, decided, finished           bool
	line, event, body                   []byte
	droppingLine, afterCR               bool
	limited, malformed                  bool
	totalBytes                          int
	inputSeen, outputSeen               bool
	input, output, total                int
	claude                              bool
	cacheRead, cacheWrite               int
	cache5m, cache1h                    int
	usage                               Usage
	text, snapshot                      strings.Builder
	textBytes, snapshotBytes            int
	jsonParts                           []*responseUsageJSONPart
	jsonDeltas                          []responseUsageJSONDelta
	jsonPartKeys                        map[string]*responseUsageJSONPart
	jsonCaptureBytes, jsonOverflowBytes int
}

// NewResponseUsageAccumulator creates a bounded observer. eventStream enables
// SSE framing immediately; otherwise the first non-whitespace byte selects JSON
// or SSE, accommodating upstreams that omit their content-type header.
func NewResponseUsageAccumulator(eventStream bool) *ResponseUsageAccumulator {
	return &ResponseUsageAccumulator{stream: eventStream, decided: eventStream}
}

// Observe accounts for response bytes without modifying them or blocking
// passthrough on malformed metadata. Calls after Finish have no effect.
func (a *ResponseUsageAccumulator) Observe(p []byte) {
	if a.finished {
		return
	}
	a.totalBytes += len(p)
	if !a.decided {
		trimmed := bytes.TrimSpace(p)
		if len(trimmed) == 0 {
			return
		}
		a.stream = trimmed[0] != '{' && trimmed[0] != '['
		a.decided = true
	}
	if !a.stream {
		remaining := responseUsageCaptureLimit - len(a.body)
		if len(p) > remaining {
			a.limited = true
			p = p[:remaining]
		}
		a.body = append(a.body, p...)
		return
	}
	for _, b := range p {
		if b == '\n' && a.afterCR {
			a.afterCR = false
			continue
		}
		a.afterCR = b == '\r'
		if b == '\n' || b == '\r' {
			a.consumeLine()
			continue
		}
		if a.droppingLine {
			continue
		}
		if len(a.line) == responseUsageCaptureLimit {
			a.limited, a.droppingLine = true, true
			a.line = a.line[:0]
			a.event = a.event[:0]
			continue
		}
		a.line = append(a.line, b)
	}
}

// consumeLine accepts SSE data, including multiline JSON and legacy upstreams
// that omit blank separators between independently valid JSON data lines.
func (a *ResponseUsageAccumulator) consumeLine() {
	if a.droppingLine {
		a.droppingLine = false
		return
	}
	line := a.line
	a.line = a.line[:0]
	if len(line) == 0 {
		a.consumeEvent()
		return
	}
	if !bytes.HasPrefix(line, []byte("data:")) {
		return
	}
	data := bytes.TrimPrefix(line, []byte("data:"))
	data = bytes.TrimPrefix(data, []byte(" "))
	if bytes.Equal(bytes.TrimSpace(data), []byte("[DONE]")) {
		a.consumeEvent()
		return
	}
	if len(a.event) == 0 {
		if json.Valid(data) {
			a.consumeJSON(data)
			return
		}
		// Preserve a potentially incomplete JSON envelope. A subsequent data
		// line may be a valid JSON value inside it, not an independent event.
		// Reject irreparably malformed prefixes now so legacy streams can
		// recover at their next complete receipt without a blank separator.
		var fragment json.RawMessage
		err := json.NewDecoder(bytes.NewReader(data)).Decode(&fragment)
		if !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
			a.malformed = true
			return
		}
	}
	if len(a.event)+len(data)+1 > responseUsageCaptureLimit {
		a.limited = true
		a.event = a.event[:0]
		return
	}
	// Parse the accumulated envelope only at its boundary, not on each line;
	// repeated whole-prefix parsing would make long multiline events quadratic.
	a.event = append(a.event, data...)
	a.event = append(a.event, '\n')
}

// consumeEvent parses a complete multiline SSE payload and releases its bytes.
func (a *ResponseUsageAccumulator) consumeEvent() {
	if len(a.event) > 0 {
		a.consumeJSON(a.event)
		a.event = a.event[:0]
	}
}

// Finish returns canonical usage, estimating only missing counters. countText
// may select the caller's tokenizer; nil selects a labelled byte-based estimate.
// Repeated calls do not consume or add the same stream bytes again.
func (a *ResponseUsageAccumulator) Finish(promptTokens int, countText func(string) int) *Usage {
	if !a.finished {
		if a.stream {
			a.consumeLine()
			a.consumeEvent()
		} else if len(a.body) > 0 && !a.limited {
			a.consumeJSON(a.body)
		}
		a.finished = true
		a.line, a.event, a.body = nil, nil, nil
	}
	usage := a.usage
	usage.PromptTokens = a.input
	if a.claude {
		usage.PromptTokens += a.cacheRead + max(a.cacheWrite, a.cache5m+a.cache1h)
		usage.CacheWrite1hTokens = a.cache1h
		usage.CacheWrite5mTokens = max(a.cache5m, a.cacheWrite-a.cache1h)
		if a.cacheRead > 0 {
			usage.PromptTokensDetails = mergeResponsePromptDetails(usage.PromptTokensDetails, &UsagePromptTokensDetails{CachedTokens: a.cacheRead})
		}
	}
	if !a.inputSeen {
		usage.PromptTokens = max(usage.PromptTokens, promptTokens)
	}
	usage.CompletionTokens = a.output
	if !a.outputSeen || a.incomplete {
		text, size := a.completionEvidence()
		if countText == nil {
			usage.CompletionTokens = max(usage.CompletionTokens, responseUsageEstimate(size))
		} else {
			usage.CompletionTokens = max(usage.CompletionTokens, max(0, countText(text))+responseUsageEstimate(size-len(text)))
		}
		if a.total > usage.PromptTokens {
			usage.CompletionTokens = max(usage.CompletionTokens, a.total-usage.PromptTokens)
		}
		if a.limited && size == 0 {
			usage.CompletionTokens = max(usage.CompletionTokens, responseUsageEstimate(a.totalBytes))
		}
	}
	usage.NormalizeCachedTokens()
	usage.NormalizeCacheWriteTokens()
	if usage.PromptTokensDetails != nil {
		details := *usage.PromptTokensDetails
		usage.PromptTokensDetails = &details
		if a.inputSeen && details.CachedTokens > usage.PromptTokens {
			details.CachedTokens = usage.PromptTokens
			a.malformed = true
		}
		if !a.inputSeen {
			usage.PromptTokens = max(usage.PromptTokens, details.CachedTokens)
		}
	}
	if usage.CompletionTokensDetails != nil {
		details := *usage.CompletionTokensDetails
		usage.CompletionTokensDetails = &details
		if !a.outputSeen || a.incomplete {
			usage.CompletionTokens = max(usage.CompletionTokens, details.ReasoningTokens+details.AudioTokens)
		}
	}
	usage.PromptTokens = max(0, usage.PromptTokens)
	usage.CompletionTokens = max(0, usage.CompletionTokens)
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	if !a.inputSeen || !a.outputSeen {
		usage.BillingEstimateReason = "response_usage_missing_counters"
	}
	// A later complete receipt recovers from unrelated malformed/oversized content;
	// do not retain a reservation as an estimate when both counters are measured.
	if a.malformed && (!a.inputSeen || !a.outputSeen) {
		usage.BillingEstimateReason = "response_usage_malformed_metadata"
	}
	if a.limited && (!a.inputSeen || !a.outputSeen) {
		usage.BillingEstimateReason = "response_usage_capture_limit"
	}
	if a.incomplete {
		usage.BillingEstimateReason = "response_usage_incomplete_stream"
	}
	return &usage
}

// MarkIncomplete records a transport interruption so partial cumulative output
// cannot suppress the larger amount of completion text already observed.
func (a *ResponseUsageAccumulator) MarkIncomplete() {
	a.incomplete = true
}

// ParseResponseUsage parses an already captured JSON or SSE response with the
// same normalization and fallback rules as the incremental network observer.
func ParseResponseUsage(body []byte, promptTokens int, countText func(string) int) *Usage {
	acc := NewResponseUsageAccumulator(false)
	acc.Observe(body)
	return acc.Finish(promptTokens, countText)
}

// responseUsageEstimate estimates nonempty text bytes without rounding to zero.
func responseUsageEstimate(size int) int {
	if size <= 0 {
		return 0
	}
	return max(1, int(float64(size)*0.38))
}

// consumeJSON extracts usage and output from a protocol envelope. Malformed
// metadata is recorded without preventing later valid stream receipts.
func (a *ResponseUsageAccumulator) consumeJSON(data []byte) {
	var root map[string]json.RawMessage
	if json.Unmarshal(data, &root) != nil || root == nil {
		a.malformed = true
		return
	}
	kind := responseUsageString(root["type"])
	claude := strings.HasPrefix(kind, "message_") || strings.HasPrefix(kind, "content_block_") || kind == "message"
	a.consumeUsage(root["usage"], claude, kind == "message_start" || kind == "response.created" || kind == "response.in_progress")
	for _, key := range []string{"message", "response"} {
		var nested map[string]json.RawMessage
		if json.Unmarshal(root[key], &nested) == nil && nested != nil {
			a.consumeUsage(nested["usage"], key == "message", kind == "message_start" || kind == "response.created" || kind == "response.in_progress")
			if key == "response" {
				a.consumeOutput(nested, true)
			}
		}
	}
	if value := responseUsageString(root["service_tier"]); value != "" {
		a.usage.ServiceTier = value
	}
	if value := responseUsageString(root["system_fingerprint"]); value != "" {
		a.usage.SystemFingerprint = value
	}
	a.consumeOutput(root, !a.stream)
}

// consumeUsage merges cumulative counters field by field. Missing fields never
// erase earlier input/cache metadata and repeated snapshots are never summed.
func (a *ResponseUsageAccumulator) consumeUsage(raw json.RawMessage, claude, provisional bool) {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		a.malformed = true
		return
	}
	for _, key := range []string{"prompt_tokens", "input_tokens"} {
		if value, ok := a.counter(fields, key); ok {
			// Responses lifecycle events can contain placeholder input counters;
			// they must not suppress estimation when the final receipt omits one.
			// Claude message_start input is already measured, including zero.
			a.inputSeen = a.inputSeen || !provisional || claude
			a.input = max(a.input, value)
			break
		}
	}
	for _, key := range []string{"completion_tokens", "output_tokens"} {
		if value, ok := a.counter(fields, key); ok {
			a.outputSeen = a.outputSeen || !provisional
			a.output = max(a.output, value)
			break
		}
	}
	if value, ok := a.counter(fields, "total_tokens"); ok {
		a.total = max(a.total, value)
	}
	if value, ok := a.counter(fields, "cache_read_input_tokens"); ok {
		a.cacheRead = max(a.cacheRead, value)
		claude = true
	}
	if value, ok := a.counter(fields, "cache_creation_input_tokens"); ok {
		a.cacheWrite = max(a.cacheWrite, value)
		claude = true
	}
	var creation map[string]json.RawMessage
	if json.Unmarshal(fields["cache_creation"], &creation) == nil {
		if value, ok := a.counter(creation, "ephemeral_5m_input_tokens"); ok {
			a.cache5m = max(a.cache5m, value)
		}
		if value, ok := a.counter(creation, "ephemeral_1h_input_tokens"); ok {
			a.cache1h = max(a.cache1h, value)
		}
	}
	a.claude = a.claude || claude
	var canonical Usage
	if err := json.Unmarshal(raw, &canonical); err != nil {
		a.malformed = true
		return
	}
	if canonical.PromptTokensDetails != nil {
		a.usage.PromptTokensDetails = mergeResponsePromptDetails(a.usage.PromptTokensDetails, canonical.PromptTokensDetails)
	}
	if canonical.CompletionTokensDetails != nil {
		a.usage.CompletionTokensDetails = mergeResponseCompletionDetails(a.usage.CompletionTokensDetails, canonical.CompletionTokensDetails)
	}
	// Responses uses input/output_tokens_details for the same canonical fields.
	if detail, ok := fields["input_tokens_details"]; ok {
		var parsed UsagePromptTokensDetails
		if json.Unmarshal(detail, &parsed) == nil {
			a.usage.PromptTokensDetails = mergeResponsePromptDetails(a.usage.PromptTokensDetails, &parsed)
		} else {
			a.malformed = true
		}
	}
	if detail, ok := fields["output_tokens_details"]; ok {
		var parsed UsageCompletionTokensDetails
		if json.Unmarshal(detail, &parsed) == nil {
			a.usage.CompletionTokensDetails = mergeResponseCompletionDetails(a.usage.CompletionTokensDetails, &parsed)
		} else {
			a.malformed = true
		}
	}
	a.usage.CachedTokens = max(a.usage.CachedTokens, canonical.CachedTokens, canonical.PromptCacheHitTokens)
	a.usage.ToolsCost = max(a.usage.ToolsCost, canonical.ToolsCost)
	a.usage.CacheWriteTokens = max(a.usage.CacheWriteTokens, canonical.CacheWriteTokens)
	a.usage.CacheWrite5mTokens = max(a.usage.CacheWrite5mTokens, canonical.CacheWrite5mTokens)
	a.usage.CacheWrite1hTokens = max(a.usage.CacheWrite1hTokens, canonical.CacheWrite1hTokens)
	if canonical.ServiceTier != "" {
		a.usage.ServiceTier = canonical.ServiceTier
	}
	if canonical.SystemFingerprint != "" {
		a.usage.SystemFingerprint = canonical.SystemFingerprint
	}
}

// counter validates one nonnegative token counter without accepting overflowing
// JSON numbers. Invalid fields remain absent and are explicitly diagnosed.
func (a *ResponseUsageAccumulator) counter(fields map[string]json.RawMessage, key string) (int, bool) {
	raw, ok := fields[key]
	if !ok || bytes.Equal(raw, []byte("null")) {
		return 0, false
	}
	var value int
	if json.Unmarshal(raw, &value) != nil || value < 0 || uint64(value) > 1<<40 || value > int(^uint(0)>>1)/8 {
		a.malformed = true
		return 0, false
	}
	return value, true
}

// responseUsageString returns a JSON string without interpreting other shapes.
func responseUsageString(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return ""
	}
	return text
}
