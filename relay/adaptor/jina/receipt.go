package jina

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

const rejectionKey = "jina_rejected_before_inference"

// RejectedBeforeInference reports an explicit upstream admission rejection.
// Timeouts, transport errors and server errors are deliberately not refundable.
func RejectedBeforeInference(c *gin.Context) bool { return c != nil && c.GetBool(rejectionKey) }

// ClearRejection removes a prior attempt's rejection marker before routing again.
func ClearRejection(c *gin.Context) { c.Set(rejectionKey, false) }

// recordRejection marks only explicit authentication, routing, validation or
// rate-limit rejections documented by Jina; a generic 400/5xx remains uncertain.
func recordRejection(c *gin.Context, resp *http.Response) {
	if resp == nil {
		return
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusUnprocessableEntity, http.StatusTooManyRequests:
		c.Set(rejectionKey, true)
	}
}

// uniqueObject decodes a JSON object without last-key-wins ambiguity. Duplicate
// root/usage keys and trailing JSON are rejected instead of lowering a charge.
func uniqueObject(body []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	token, err := decoder.Token()
	if err != nil {
		return nil, errors.Wrap(err, "read jina JSON object")
	}
	if token != json.Delim('{') {
		return nil, errors.New("jina response must be a JSON object")
	}
	fields := map[string]json.RawMessage{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, errors.Wrap(err, "read jina JSON key")
		}
		key, ok := token.(string)
		if !ok {
			return nil, errors.New("invalid jina JSON key")
		}
		if _, exists := fields[key]; exists {
			return nil, errors.New("duplicate jina response or usage key")
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, errors.Wrap(err, "read jina JSON value")
		}
		fields[key] = raw
	}
	if _, err := decoder.Token(); err != nil {
		return nil, errors.Wrap(err, "close jina JSON object")
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("trailing data in jina response")
	}
	return fields, nil
}

// tokenCount reads a required nonnegative integer from a receipt. Null, strings,
// fractions, missing fields and overflowing integers are accounting errors.
func tokenCount(fields map[string]json.RawMessage, key string) (int, error) {
	var count *int
	if err := json.Unmarshal(fields[key], &count); err != nil {
		return 0, errors.Wrap(err, "decode jina token count")
	}
	if count == nil || *count < 0 {
		return 0, errors.New("missing or negative jina token count")
	}
	return *count, nil
}

// receiptBody observes raw JSON/SSE usage independently of the shared handlers'
// text-only fallback estimates. It never buffers more than the configured line
// or JSON limit, and delegates stream reading/closing to the original body.
type receiptBody struct {
	mu        sync.Mutex
	closeOnce sync.Once
	closeErr  error
	io.ReadCloser
	stream     bool
	pending    []byte
	receipt    *model.Usage
	invalid    bool
	done       bool
	eof        bool
	finalUsage bool
}

// Read observes bytes returned by the wrapped body and preserves its read result.
// A positive receipt is not final while the transport can still deliver paid work.
func (b *receiptBody) Read(dst []byte) (int, error) {
	n, err := b.ReadCloser.Read(dst)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stream {
		b.observeLines(dst[:n])
	} else if len(b.pending)+n <= 8*1024*1024 {
		b.pending = append(b.pending, dst[:n]...)
	} else {
		b.invalid = true
	}
	if err != nil {
		if err == io.EOF {
			b.eof = true
		} else {
			b.invalid = true
		}
		if b.stream {
			if len(b.pending) > 0 {
				b.observeLine(b.pending)
				b.pending = nil
			}
		} else {
			// Recover any complete counters preceding an interrupted JSON body.
			b.observeJSON(b.pending)
		}
	}
	return n, err
}

// Close collects buffered non-streaming evidence, then closes the transport.
// Closing alone never certifies EOF or an upstream stream-completion sentinel.
// Repeated calls release the underlying response exactly once and return the
// same error.
func (b *receiptBody) Close() error {
	b.closeOnce.Do(func() {
		b.mu.Lock()
		if !b.stream && !b.eof {
			b.observeJSON(b.pending)
		}
		b.mu.Unlock()
		if err := b.ReadCloser.Close(); err != nil {
			b.mu.Lock()
			b.invalid = true
			b.mu.Unlock()
			b.closeErr = errors.Wrap(err, "close jina OCR response")
		}
	})
	return b.closeErr
}

// observeLines scans bounded SSE lines from a read fragment without changing it.
func (b *receiptBody) observeLines(fragment []byte) {
	for len(fragment) > 0 {
		i := bytes.IndexByte(fragment, '\n')
		if i < 0 {
			if len(b.pending)+len(fragment) > 1024*1024 {
				b.invalid = true
				b.pending = nil
				return
			}
			b.pending = append(b.pending, fragment...)
			return
		}
		if len(b.pending)+i > 1024*1024 {
			b.invalid = true
			b.pending = nil
		} else {
			b.pending = append(b.pending, fragment[:i]...)
			b.observeLine(b.pending)
			b.pending = nil
		}
		fragment = fragment[i+1:]
	}
}

// observeLine extracts SSE data, records upstream completion and rejects any
// later data as ambiguous. A downstream-generated DONE never reaches this reader.
func (b *receiptBody) observeLine(line []byte) {
	if !bytes.HasPrefix(line, []byte("data:")) {
		return
	}
	data := bytes.TrimSpace(line[5:])
	if b.done {
		b.invalid = true
	}
	if bytes.Equal(data, []byte("[DONE]")) {
		b.done = true
		return
	}
	b.observeJSON(data)
}

// observeJSON collects complete OCR receipts, retaining maxima rather than
// summing cumulative stream snapshots or trusting a later decreasing counter.
func (b *receiptBody) observeJSON(body []byte) {
	// Preserve the higher observed evidence even when the receipt is ambiguous.
	if evidence := partialReceiptEvidence(body); evidence != nil {
		if b.receipt == nil {
			b.receipt = &model.Usage{}
		}
		b.receipt.PromptTokens = max(b.receipt.PromptTokens, evidence.PromptTokens)
		b.receipt.CompletionTokens = max(b.receipt.CompletionTokens, evidence.CompletionTokens)
		b.receipt.TotalTokens = max(b.receipt.TotalTokens, evidence.TotalTokens)
	}
	fields, err := uniqueObject(body)
	if err != nil {
		b.invalid = true
		return
	}
	if raw := bytes.TrimSpace(fields["error"]); len(raw) > 0 && !bytes.Equal(raw, []byte("null")) {
		b.invalid = true
	}
	if b.stream {
		var choices []json.RawMessage
		if raw, exists := fields["choices"]; exists {
			if err := json.Unmarshal(raw, &choices); err != nil {
				b.invalid = true
				return
			}
			if len(choices) > 0 {
				// An earlier cumulative receipt no longer covers this new work.
				b.finalUsage = false
			}
		}
	}
	raw, exists := fields["usage"]
	if !exists || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return
	}
	usageFields, err := uniqueObject(raw)
	if err != nil {
		b.invalid = true
		return
	}
	prompt, pErr := tokenCount(usageFields, "prompt_tokens")
	output, oErr := tokenCount(usageFields, "completion_tokens")
	total, tErr := tokenCount(usageFields, "total_tokens")
	if pErr != nil || oErr != nil || tErr != nil || prompt > MaxBillingTokens || output > MaxBillingTokens || total > MaxBillingTokens {
		b.invalid = true
		return
	}
	if total != prompt+output {
		b.invalid = true
		return
	}
	if b.receipt == nil {
		b.receipt = &model.Usage{}
	}
	b.receipt.PromptTokens = max(b.receipt.PromptTokens, prompt)
	b.receipt.CompletionTokens = max(b.receipt.CompletionTokens, output)
	b.receipt.TotalTokens = max(b.receipt.TotalTokens, saturatedTokenSum(b.receipt.PromptTokens, b.receipt.CompletionTokens))
	b.finalUsage = true
}

// handleOCRResponse uses the normal chat/Messages transport but bills only a raw
// completed upstream receipt or an explicitly labelled conservative allowance.
func handleOCRResponse(c *gin.Context, resp *http.Response, m *meta.Meta) (*model.Usage, *model.ErrorWithStatusCode) {
	if resp == nil || resp.Body == nil {
		return EstimatedUsage(c, "jina_missing_ocr_body"), openai.ErrorWrapper(errors.New("jina OCR body missing"), "invalid_upstream_response", http.StatusBadGateway)
	}
	observer := &receiptBody{ReadCloser: resp.Body, stream: m.IsStream || strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")}
	resp.Body = observer
	_, apiErr := openai_compatible.HandleClaudeMessagesResponse(c, resp, m, func(c *gin.Context, resp *http.Response, promptTokens int, modelName string) (*model.ErrorWithStatusCode, *model.Usage) {
		if m.IsStream {
			return openai_compatible.StreamHandler(c, resp, promptTokens, modelName)
		}
		return openai_compatible.Handler(c, resp, promptTokens, modelName)
	})
	// Shared handlers can return early on transport or parsing errors without
	// closing the body. Stop the upstream work before selecting a final receipt.
	// Close is idempotent because successful shared handlers may close it first.
	if closeErr := observer.Close(); closeErr != nil && apiErr == nil {
		apiErr = openai.ErrorWrapper(closeErr, "close_response_body_failed", http.StatusBadGateway)
	}
	measured, invalid := observer.snapshot()
	if measured != nil && measured.TotalTokens > 0 && !invalid {
		return measured, apiErr
	}
	estimate := mergeReceiptEstimate(EstimatedUsage(c, "jina_missing_or_invalid_ocr_receipt"), measured, false)
	if apiErr == nil {
		apiErr = openai.ErrorWrapper(errors.New("jina OCR usage was not verifiable; conservative reservation retained"), "unverified_jina_usage", http.StatusBadGateway)
	}
	return estimate, apiErr
}

// snapshot returns a locked value copy and whether the receipt is unverified.
// HTTP EOF alone does not complete SSE; a final receipt followed by upstream DONE
// is required. A cancelled reader still unwinding cannot authorize a refund.
func (b *receiptBody) snapshot() (*model.Usage, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	invalid := b.invalid || (b.stream && (!b.done || !b.finalUsage)) || (!b.stream && !b.eof)
	if b.receipt == nil {
		return nil, invalid
	}
	copy := *b.receipt
	return &copy, invalid
}
