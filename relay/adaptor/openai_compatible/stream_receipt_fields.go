package openai_compatible

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/one-api/relay/model"
)

// decodeStreamReceipt decodes the ordinary stream DTO while retaining presence
// of its small usage envelope. Missing JSON counters must not become measured
// zeros merely because Go's integer zero value is zero. No provider fee is inferred.
func decodeStreamReceipt(reader io.Reader, chunk *ChatCompletionsStreamResponse) (bool, error) {
	type plain ChatCompletionsStreamResponse
	envelope := struct {
		*plain
		Usage json.RawMessage `json:"usage"`
	}{plain: (*plain)(chunk)}
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&envelope); err != nil {
		return false, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return false, err
		}
		return false, errors.New("multiple JSON values in stream event")
	}
	raw := bytes.TrimSpace(envelope.Usage)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		chunk.Usage = nil
		return false, nil
	}
	usage := new(model.Usage)
	if err := json.Unmarshal(raw, usage); err != nil {
		return false, err
	}
	var fields struct {
		Input  *int `json:"prompt_tokens"`
		Output *int `json:"completion_tokens"`
	}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return false, err
	}
	complete := fields.Input != nil && fields.Output != nil && *fields.Input >= 0 && *fields.Output >= 0
	if !complete {
		usage.BillingEstimateReason = "stream_usage_missing_counters"
	}
	chunk.Usage = usage
	return complete, nil
}

// nextStreamReceiptCompleteness treats new output after a measured snapshot as
// unmeasured work while ignoring empty control frames. A later full receipt can
// reconcile it; a transport EOF or [DONE] marker alone cannot manufacture usage.
func nextStreamReceiptCompleteness(previous, complete bool, chunk *ChatCompletionsStreamResponse) bool {
	if chunk.Usage != nil {
		return complete
	}
	for _, choice := range chunk.Choices {
		delta := choice.Delta
		if delta.StringContent() != "" || len(delta.ToolCalls) > 0 || delta.ReasoningContent != nil || delta.Reasoning != nil || delta.Thinking != nil {
			return false
		}
	}
	return previous
}

// DecodeStreamReceipt preserves counter presence and the existing native partial-receipt fallback policy.
func DecodeStreamReceipt(reader io.Reader, chunk *ChatCompletionsStreamResponse) (bool, error) {
	complete, err := decodeStreamReceipt(reader, chunk)
	if err == nil && !complete && chunk.Usage != nil && chunk.Usage.BillingEstimateReason == "stream_usage_missing_counters" {
		chunk.Usage.BillingEstimateReason = ""
	}
	return complete, err
}

// NextStreamReceiptCompleteness preserves measured-receipt chronology for native chat streams.
func NextStreamReceiptCompleteness(previous, complete bool, chunk *ChatCompletionsStreamResponse) bool {
	return nextStreamReceiptCompleteness(previous, complete, chunk)
}
