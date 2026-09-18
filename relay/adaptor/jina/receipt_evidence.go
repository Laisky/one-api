package jina

import (
	"bytes"
	"encoding/json"
	"math"

	"github.com/Laisky/one-api/relay/model"
)

// partialReceiptEvidence retains maxima from ambiguous or interrupted usage
// objects. It can raise an estimate but never validate a receipt or reduce a hold.
func partialReceiptEvidence(body []byte) *model.Usage {
	counts := map[string]int{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil
	}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		start := decoder.InputOffset()
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			if token == "usage" {
				// InputOffset is after the key and before its colon. Recover
				// complete counters even when the usage object itself is cut off.
				tail := bytes.TrimSpace(body[start:])
				tail = bytes.TrimSpace(bytes.TrimPrefix(tail, []byte(":")))
				collectReceiptCounts(tail, counts)
			}
			break
		}
		if token == "usage" {
			collectReceiptCounts(raw, counts)
		}
	}
	if len(counts) == 0 {
		return nil
	}
	return &model.Usage{PromptTokens: counts["prompt_tokens"], CompletionTokens: counts["completion_tokens"], TotalTokens: counts["total_tokens"], BillingEstimateReason: "jina_ambiguous_receipt_counters"}
}

// collectReceiptCounts reads maxima from a standalone usage object.
func collectReceiptCounts(raw []byte, counts map[string]int) {
	collectReceiptCountsFromDecoder(json.NewDecoder(bytes.NewReader(raw)), counts)
}

// collectReceiptCountsFromDecoder preserves complete counters before a malformed
// tail and consumes the usage object's closing delimiter when present.
func collectReceiptCountsFromDecoder(decoder *json.Decoder, counts map[string]int) {
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return
	}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return
		}
		key, ok := token.(string)
		if !ok || (key != "total_tokens" && key != "prompt_tokens" && key != "completion_tokens") {
			continue
		}
		var count *int
		if err := json.Unmarshal(value, &count); err != nil || count == nil || *count < 0 {
			continue
		}
		counts[key] = max(counts[key], *count)
	}
	// This is a best-effort evidence parser, not the receipt validator. A
	// truncated closing delimiter does not invalidate previously observed counts.
	if _, err := decoder.Token(); err != nil {
		return
	}
}

// saturatedTokenSum prevents an ambiguous receipt's aggregate from wrapping
// negative. Individual counters remain intact for checked monetary calculation.
func saturatedTokenSum(input, output int) int {
	input, output = max(input, 0), max(output, 0)
	if output > math.MaxInt-input {
		return math.MaxInt
	}
	return input + output
}

// mergeReceiptEstimate returns an estimate no smaller than either its reserved
// allowance or observed evidence. Search work is input-only. For OCR, unknown
// total-token attribution is reserved at both possible rates: channel overrides
// may make input more expensive than output, unlike Jina's published defaults.
func mergeReceiptEstimate(estimate, evidence *model.Usage, search bool) *model.Usage {
	if evidence == nil {
		return estimate
	}
	if estimate == nil {
		estimate = &model.Usage{BillingEstimateReason: "jina_ambiguous_receipt_counters"}
	}
	if search {
		estimate.PromptTokens = max(estimate.PromptTokens, evidence.TotalTokens, evidence.PromptTokens)
		estimate.CompletionTokens = 0
	} else {
		input, output := max(evidence.PromptTokens, 0), max(evidence.CompletionTokens, 0)
		total := max(evidence.TotalTokens, 0)
		estimate.PromptTokens = max(estimate.PromptTokens, input, total-output)
		estimate.CompletionTokens = max(estimate.CompletionTokens, output, total-input)
	}
	estimate.TotalTokens = saturatedTokenSum(estimate.PromptTokens, estimate.CompletionTokens)
	return estimate
}
