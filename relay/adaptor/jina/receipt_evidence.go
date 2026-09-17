package jina

import (
	"bytes"
	"encoding/json"

	"github.com/Laisky/one-api/relay/model"
)

// partialReceiptEvidence retains maxima from ambiguous usage objects without
// treating them as verified receipts. It is used only to raise an estimate, never
// to lower a reservation or to accept a malformed response as successful.
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
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			break
		}
		if token != "usage" {
			continue
		}
		collectReceiptCounts(raw, counts)
	}
	if len(counts) == 0 {
		return nil
	}
	return &model.Usage{PromptTokens: counts["prompt_tokens"], CompletionTokens: counts["completion_tokens"], TotalTokens: counts["total_tokens"], BillingEstimateReason: "jina_ambiguous_receipt_counters"}
}

// collectReceiptCounts reads all occurrences of recognized usage counters and
// keeps nonnegative integer maxima, including values preceding malformed tails.
func collectReceiptCounts(raw []byte, counts map[string]int) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
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
}
