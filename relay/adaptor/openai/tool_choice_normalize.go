package openai

import (
	"encoding/json"
	"fmt"
	"strings"
)

// NormalizeToolChoice rewrites tool_choice values into the canonical format expected by OpenAI.
// Returns the normalized value and a flag indicating whether a change was applied.
func NormalizeToolChoice(choice any) (any, bool) {
	if choice == nil {
		return nil, false
	}

	switch typed := choice.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return nil, true
		}
		if trimmed != typed {
			return trimmed, true
		}
		return typed, false
	case map[string]any:
		return normalizeToolChoiceMap(typed)
	case map[string]string:
		converted := make(map[string]any, len(typed))
		for k, v := range typed {
			converted[k] = v
		}
		return normalizeToolChoiceMap(converted)
	default:
		data, err := json.Marshal(choice)
		if err != nil {
			return choice, false
		}
		var asMap map[string]any
		if err := json.Unmarshal(data, &asMap); err != nil {
			return choice, false
		}
		normalized, changed := normalizeToolChoiceMap(asMap)
		if !changed {
			return choice, false
		}
		return normalized, true
	}
}

func normalizeToolChoiceMap(choice map[string]any) (map[string]any, bool) {
	if choice == nil {
		return nil, false
	}

	originalType, _ := choice["type"].(string)
	typeName := strings.ToLower(strings.TrimSpace(originalType))
	if typeName == "" {
		if _, ok := choice["function"].(map[string]any); ok {
			typeName = "function"
		} else if _, ok := choice["name"].(string); ok {
			typeName = "tool"
		}
	}

	if typeName == "tool" {
		name := strings.TrimSpace(stringFromAny(choice["name"]))
		if name == "" {
			if fn, ok := choice["function"].(map[string]any); ok {
				name = strings.TrimSpace(stringFromAny(fn["name"]))
			}
		}
		if name == "" {
			return choice, false
		}
		normalized := map[string]any{
			"type":     "function",
			"function": map[string]any{"name": name},
		}
		if mode, ok := choice["mode"]; ok {
			normalized["mode"] = mode
		}
		if reason, ok := choice["reason"]; ok {
			normalized["reason"] = reason
		}
		return normalized, true
	}

	changed := false
	if typeName == "" {
		name := strings.TrimSpace(stringFromAny(choice["name"]))
		if name == "" {
			return choice, false
		}
		return map[string]any{
			"type":     "function",
			"function": map[string]any{"name": name},
		}, true
	}

	if typeName != "function" {
		choice["type"] = "function"
		changed = true
	}

	var fnMap map[string]any
	switch fn := choice["function"].(type) {
	case map[string]any:
		fnMap = fn
	case string:
		fnMap = map[string]any{}
		if trimmed := strings.TrimSpace(fn); trimmed != "" {
			fnMap["name"] = trimmed
		}
		changed = true
	case nil:
		fnMap = map[string]any{}
		changed = true
	default:
		data, err := json.Marshal(fn)
		if err == nil {
			_ = json.Unmarshal(data, &fnMap)
			changed = true
		} else {
			fnMap = map[string]any{}
			changed = true
		}
	}

	if _, ok := fnMap["name"]; !ok {
		if name := strings.TrimSpace(stringFromAny(choice["name"])); name != "" {
			fnMap["name"] = name
			changed = true
		}
	}

	if len(fnMap) == 0 {
		return choice, changed
	}

	choice["function"] = fnMap
	if _, ok := choice["name"]; ok {
		delete(choice, "name")
		changed = true
	}

	return choice, changed
}

// NormalizeToolChoiceForResponse rewrites tool_choice payloads into the
// canonical structure accepted by the OpenAI Responses API. The API expects
// either a trimmed string ("auto"/"none") or an object shaped like
// {"type":"function","name":"..."}. This helper funnels legacy formats
// (e.g. {"type":"tool","name":"..."}) through NormalizeToolChoice and
// flattens nested function blocks accordingly while preserving auxiliary
// fields like mode or reason.
func NormalizeToolChoiceForResponse(choice any) (any, bool) {
	if choice == nil {
		return nil, false
	}

	normalized, changed := NormalizeToolChoice(choice)
	return normalizeToolChoiceForResponseValue(normalized, changed)
}

func normalizeToolChoiceForResponseValue(value any, alreadyChanged bool) (any, bool) {
	switch typed := value.(type) {
	case nil:
		return nil, alreadyChanged
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return nil, true
		}
		if trimmed != typed {
			return trimmed, true
		}
		return trimmed, alreadyChanged
	case map[string]any:
		changed := alreadyChanged
		typeName := strings.ToLower(strings.TrimSpace(stringFromAny(typed["type"])))
		if typeName == "" || typeName == "tool" {
			typed["type"] = "function"
			changed = true
		} else if typeName != "function" {
			typed["type"] = "function"
			changed = true
		}

		name := strings.TrimSpace(stringFromAny(typed["name"]))
		if name == "" {
			if fn, ok := typed["function"].(map[string]any); ok {
				name = strings.TrimSpace(stringFromAny(fn["name"]))
			} else if fnStr := strings.TrimSpace(stringFromAny(typed["function"])); fnStr != "" {
				name = fnStr
			}
		}
		if name != "" {
			if current := strings.TrimSpace(stringFromAny(typed["name"])); current != name {
				typed["name"] = name
				changed = true
			} else if _, exists := typed["name"]; !exists {
				typed["name"] = name
				changed = true
			}
		} else if _, exists := typed["name"]; exists {
			delete(typed, "name")
			changed = true
		}

		if _, exists := typed["function"]; exists {
			delete(typed, "function")
			changed = true
		}

		return typed, changed
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return value, alreadyChanged
		}
		var asMap map[string]any
		if err := json.Unmarshal(data, &asMap); err != nil {
			return value, alreadyChanged
		}
		return normalizeToolChoiceForResponseValue(asMap, alreadyChanged)
	}
}

func stringFromAny(value any) string {
	if value == nil {
		return ""
	}
	if str, ok := value.(string); ok {
		return str
	}
	return fmt.Sprintf("%v", value)
}
