package openai

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/one-api/common"
)

// ResponseToolSearchInputItem holds the reviewed discovery fields of one input
// item while the original input retains its position and unknown wire metadata.
type ResponseToolSearchInputItem struct {
	Index     int               `json:"-"`
	Raw       json.RawMessage   `json:"-"`
	Type      string            `json:"type"`
	Execution string            `json:"execution,omitempty"`
	Role      string            `json:"role,omitempty"`
	CallID    string            `json:"call_id,omitempty"`
	Arguments json.RawMessage   `json:"arguments,omitempty"`
	Tools     []ResponseAPITool `json:"tools,omitempty"`
}

// InspectResponseToolSearchInput decodes discovery input items without altering
// input order or arguments. It rejects ambiguous discriminators before generic
// map decoding and bounds each newly loaded tool catalog before recursive decode.
func InspectResponseToolSearchInput(data []byte) ([]ResponseToolSearchInputItem, error) {
	if len(bytes.TrimSpace(data)) == 0 || bytes.TrimSpace(data)[0] != '[' {
		return nil, nil
	}
	var input []json.RawMessage
	if err := json.Unmarshal(data, &input); err != nil {
		return nil, errors.Wrap(err, "decode Responses discovery input")
	}
	var found []ResponseToolSearchInputItem
	for index, raw := range input {
		var shape struct {
			Type      string          `json:"type"`
			Execution string          `json:"execution"`
			Role      string          `json:"role"`
			CallID    string          `json:"call_id"`
			Arguments json.RawMessage `json:"arguments"`
			Tools     json.RawMessage `json:"tools"`
		}
		if err := common.ValidateUnambiguousJSONRootKeys(raw, shape); err != nil {
			return nil, errors.Wrap(err, "validate Responses discovery item keys")
		}
		if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || trimmed[0] != '{' {
			continue
		}
		if err := json.Unmarshal(raw, &shape); err != nil {
			return nil, errors.Wrap(err, "decode Responses input item type")
		}
		switch strings.ToLower(strings.TrimSpace(shape.Type)) {
		case "tool_search_call", "tool_search_output", "additional_tools":
		default:
			continue
		}
		item := ResponseToolSearchInputItem{Index: index, Raw: append(json.RawMessage(nil), raw...), Type: shape.Type, Execution: shape.Execution, Role: shape.Role, CallID: shape.CallID, Arguments: shape.Arguments}
		if len(shape.Tools) > 0 && !bytes.Equal(bytes.TrimSpace(shape.Tools), []byte("null")) {
			var definitions []json.RawMessage
			if err := json.Unmarshal(shape.Tools, &definitions); err != nil {
				return nil, errors.Wrap(err, "decode discovered tool catalog")
			}
			if len(definitions) > MaxResponseNamespaceTools {
				return nil, errors.New("discovered tool count exceeds limit")
			}
			item.Tools = make([]ResponseAPITool, 0, len(definitions))
			for _, definition := range definitions {
				var tool ResponseAPITool
				if err := json.Unmarshal(definition, &tool); err != nil {
					return nil, errors.Wrap(err, "decode discovered response tool")
				}
				item.Tools = append(item.Tools, tool)
			}
		}
		found = append(found, item)
	}
	return found, nil
}
