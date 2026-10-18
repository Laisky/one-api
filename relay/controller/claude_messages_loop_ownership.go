package controller

import (
	"bytes"
	"encoding/json"
	"github.com/Laisky/errors/v2"
	"strings"

	"github.com/Laisky/one-api/relay/adaptor/anthropic"
)

// claudeTurnHasClientToolCalls detects any call the gateway does not own before execution.
func claudeTurnHasClientToolCalls(response *anthropic.Response, registry *claudeToolSearchMCPRegistry) bool {
	if response == nil || response.StopReason == nil || *response.StopReason != "tool_use" {
		return false
	}
	for _, block := range response.Content {
		if block.Type == "tool_use" && (registry == nil || !registry.isMCPTool(block.Name)) {
			return true
		}
	}
	return false
}

// claudeTurnHasOpaqueState prevents unsupported conversion of provider-bound blocks.
func claudeTurnHasOpaqueState(response *anthropic.Response) bool {
	if response == nil {
		return false
	}
	for _, block := range response.Content {
		switch strings.ToLower(strings.TrimSpace(block.Type)) {
		case "text", "tool_use":
		case "thinking":
			if block.Signature != nil && *block.Signature != "" {
				return true
			}
		default:
			return true
		}
	}
	return false
}

// claudeMCPExecutionArguments preserves native numbers while keeping explicit typed input edits authoritative.
func claudeMCPExecutionArguments(block anthropic.Content) (map[string]any, error) {
	raw, err := json.Marshal(block)
	if err != nil {
		return nil, errors.Wrap(err, "encode native Claude tool input")
	}
	var wire struct {
		Input json.RawMessage `json:"input"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nil, errors.Wrap(err, "decode native Claude tool input")
	}
	value := bytes.TrimSpace(wire.Input)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return map[string]any{}, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.UseNumber()
	var args map[string]any
	if err := decoder.Decode(&args); err != nil {
		return nil, errors.Wrap(err, "native Claude tool input must be an object")
	}
	return args, nil
}
