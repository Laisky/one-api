package mcp

import (
	"encoding/json"

	"github.com/Laisky/errors/v2"
)

// ModelVisibleJSON returns the model-facing content of a tool result without
// transport metadata, continuation state, interaction requests, or extensions.
// The receiver and its Raw bytes remain unchanged. Raw-only legacy results are
// decoded into a private value; encoding or decoding failures are returned.
func (c *CallToolResult) ModelVisibleJSON() ([]byte, error) {
	if c == nil {
		return []byte(`{"content":null}`), nil
	}
	source := *c
	// Typed results are authoritative, including explicit structured nulls.
	// Only the historical Raw-only construction needs a compatibility decode.
	if len(source.Raw) > 0 && source.Content == nil && source.StructuredContent == nil &&
		!source.structuredContentPresent && !source.IsError && source.ResultType == "" &&
		source.InputRequests == nil && source.RequestState == "" && source.Meta == nil &&
		source.AdditionalFields == nil {
		if err := DecodeJSON(source.Raw, &source); err != nil {
			return nil, errors.Wrap(err, "decode raw-only mcp result for model projection")
		}
	}

	// Content and structuredContent are the tool's deliberate model-visible
	// payloads. Do not recursively rewrite application data within those fields.
	payload := map[string]any{"content": source.Content}
	if source.StructuredContent != nil || source.structuredContentPresent {
		payload["structuredContent"] = source.StructuredContent
	}
	if source.IsError {
		payload["isError"] = true
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, errors.Wrap(err, "marshal model-visible mcp tool result")
	}
	return encoded, nil
}
