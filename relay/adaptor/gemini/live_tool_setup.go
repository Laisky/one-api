package gemini

import (
	"encoding/json"

	"github.com/Laisky/errors/v2"
)

// prepareLiveTools preserves function schemas while enforcing the model's tool
// contract. Parameters: raw is setup.tools and model is the authorized model.
// Returns: native tool declarations or a payload-free validation error. Extended
// Thinking requires NON_BLOCKING functions, per Google's Live thinking guide.
func prepareLiveTools(raw json.RawMessage, model string) (json.RawMessage, error) {
	var tools []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &tools); err != nil || len(tools) > 256 {
		return nil, errors.Wrap(ErrLiveProtocol, "invalid Live tools")
	}
	total := 0
	for _, tool := range tools {
		if len(tool) != 1 || tool["functionDeclarations"] == nil {
			return nil, errors.Wrap(ErrLiveProtocol, "only client-executed function tools are enabled; paid built-in tools require separate usage accounting")
		}
		var functions []map[string]json.RawMessage
		if err := json.Unmarshal(tool["functionDeclarations"], &functions); err != nil || len(functions) == 0 {
			return nil, errors.Wrap(ErrLiveProtocol, "invalid Live function declarations")
		}
		total += len(functions)
		if total > 256 {
			return nil, errors.Wrap(ErrLiveProtocol, "too many Live function declarations")
		}
		for _, function := range functions {
			var name string
			if function == nil || json.Unmarshal(function["name"], &name) != nil || name == "" || len(name) > 1024 {
				return nil, errors.Wrap(ErrLiveProtocol, "invalid Live function name")
			}
			if model == "gemini-3.8-live-extended-thinking" {
				if behavior, exists := function["behavior"]; exists {
					var value string
					if json.Unmarshal(behavior, &value) != nil || value != "NON_BLOCKING" {
						return nil, errors.Wrap(ErrLiveProtocol, "Extended Thinking requires NON_BLOCKING functions")
					}
				}
				function["behavior"] = json.RawMessage(`"NON_BLOCKING"`)
			}
		}
		var err error
		tool["functionDeclarations"], err = json.Marshal(functions)
		if err != nil {
			return nil, errors.Wrap(err, "encode Live function declarations")
		}
	}
	result, err := json.Marshal(tools)
	return result, errors.Wrap(err, "encode Live tools")
}
