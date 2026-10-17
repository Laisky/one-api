package openai

import (
	"bytes"
	"encoding/json"

	"github.com/Laisky/errors/v2"
)

// outputItemWire omits OutputItem methods when decoding its common fields.
type outputItemWire OutputItem

// UnmarshalJSON decodes common output fields while selecting type-specific
// argument and tool representations. Parameters: data is one upstream item.
// Returns: a wrapped error for malformed fields; ordinary function arguments
// remain strings and MCP definitions retain their historical representation.
func (o *OutputItem) UnmarshalJSON(data []byte) error {
	var raw struct {
		outputItemWire
		Arguments json.RawMessage `json:"arguments"`
		Tools     json.RawMessage `json:"tools"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return errors.Wrap(err, "decode response output item")
	}
	decoded := OutputItem(raw.outputItemWire)
	if decoded.Type == "tool_search_call" {
		// Codex rust-v0.163.0 models tool-search arguments as serde_json::Value.
		// Retain its exact JSON number/string/object shape, including null in
		// provisional items, without widening ordinary function arguments.
		decoded.ToolSearchArguments = append(json.RawMessage(nil), raw.Arguments...)
	} else if len(raw.Arguments) > 0 {
		if err := json.Unmarshal(raw.Arguments, &decoded.Arguments); err != nil {
			return errors.Wrap(err, "decode response function arguments")
		}
	}
	if decoded.Type == "tool_search_output" {
		if len(raw.Tools) > 0 {
			// Bound the catalog before constructing recursive tool DTOs.
			var definitions []json.RawMessage
			if err := json.Unmarshal(raw.Tools, &definitions); err != nil {
				return errors.Wrap(err, "decode response tool-search catalog")
			}
			if len(definitions) > MaxResponseNamespaceTools {
				return errors.New("response tool-search catalog count exceeds limit")
			}
			if err := json.Unmarshal(raw.Tools, &decoded.ToolSearchTools); err != nil {
				return errors.Wrap(err, "decode response tool-search definitions")
			}
			snapshot, err := json.Marshal(decoded.ToolSearchTools)
			if err != nil {
				return errors.Wrap(err, "snapshot response tool-search definitions")
			}
			decoded.toolSearchToolsRaw = append(json.RawMessage(nil), raw.Tools...)
			decoded.toolSearchToolsSnapshot = snapshot
		}
	} else if len(raw.Tools) > 0 {
		if err := json.Unmarshal(raw.Tools, &decoded.Tools); err != nil {
			return errors.Wrap(err, "decode response MCP definitions")
		}
	}
	*o = decoded
	return nil
}

// marshalToolSearchOutputItem encodes structured discovery fields without
// changing ordinary function/MCP callers. Parameters: item is a search call or
// output. Returns: the JSON wire item, or a wrapped serialization error.
func marshalToolSearchOutputItem(item OutputItem) ([]byte, error) {
	var wire any
	if item.Type == "tool_search_call" {
		wire = struct {
			outputItemWire
			Arguments json.RawMessage `json:"arguments,omitempty"`
		}{outputItemWire: outputItemWire(item), Arguments: item.ToolSearchArguments}
	} else {
		typed, err := json.Marshal(item.ToolSearchTools)
		if err != nil {
			return nil, errors.Wrap(err, "marshal typed response tool-search definitions")
		}
		tools := json.RawMessage(typed)
		if len(item.toolSearchToolsRaw) > 0 && bytes.Equal(typed, item.toolSearchToolsSnapshot) {
			// Use the original bytes only while no public typed field changed.
			// This keeps opaque extensions and integers outside float64 precision
			// intact in state round trips without undoing programmatic mutations.
			tools = item.toolSearchToolsRaw
		} else if item.ToolSearchTools == nil {
			tools = json.RawMessage("[]")
		}
		wire = struct {
			outputItemWire
			Tools json.RawMessage `json:"tools"`
		}{outputItemWire: outputItemWire(item), Tools: tools}
	}
	data, err := json.Marshal(wire)
	if err != nil {
		return nil, errors.Wrap(err, "marshal response tool-search item")
	}
	return data, nil
}
