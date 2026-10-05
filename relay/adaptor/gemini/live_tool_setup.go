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

// liveBlockingFunctions lists the functions the provider waits for before it
// continues the requesting turn. Parameters: setup is the model-pinned setup
// frame forwarded upstream. Returns: names explicitly declared BLOCKING. Keys
// are read exactly, as the provider reads them. Any other declaration (absent,
// UNSPECIFIED, NON_BLOCKING, an unknown value, or a name declared twice with
// different behaviors) is non-blocking, so its result is never assumed to be
// consumed by the turn that requested it: the default differs by model
// (Gemini 3.8 Live defaults to NON_BLOCKING, the API reference to BLOCKING).
func liveBlockingFunctions(setup []byte) map[string]bool {
	blocking, nonBlocking := make(map[string]bool), make(map[string]bool)
	var root, frame map[string]json.RawMessage
	if json.Unmarshal(setup, &root) != nil || json.Unmarshal(root["setup"], &frame) != nil {
		return blocking
	}
	var tools []map[string]json.RawMessage
	if json.Unmarshal(frame["tools"], &tools) != nil {
		return blocking
	}
	for _, tool := range tools {
		var functions []map[string]json.RawMessage
		if json.Unmarshal(tool["functionDeclarations"], &functions) != nil {
			continue
		}
		for _, function := range functions {
			var name string
			if json.Unmarshal(function["name"], &name) != nil || name == "" {
				continue
			}
			if liveBehaviorBlocks(function["behavior"]) {
				blocking[name] = true
			} else {
				nonBlocking[name] = true
			}
		}
	}
	for name := range nonBlocking {
		delete(blocking, name)
	}
	return blocking
}

// liveBehaviorBlocks reports whether a FunctionDeclaration behavior value is
// explicitly blocking. Parameters: raw is the exact "behavior" value, nil when
// absent. Returns: true only for the BLOCKING enum name or its number (1).
func liveBehaviorBlocks(raw json.RawMessage) bool {
	var name string
	if json.Unmarshal(raw, &name) == nil {
		return name == "BLOCKING"
	}
	var number json.Number
	return json.Unmarshal(raw, &number) == nil && number.String() == "1"
}
