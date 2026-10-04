package model

import (
	"encoding/json"
	"strconv"
	"strings"
)

// ResponseOutputJSONDoneFields keeps raw application leaves separate from protocol wrappers.
type ResponseOutputJSONDoneFields struct {
	JSON     json.RawMessage
	PartJSON json.RawMessage
	PartText string
	Output   json.RawMessage
	Text     string
	Delta    json.RawMessage
}

// ExtractResponseOutputJSONDone is the shared wire contract for done-event delivery and fallback accounting.
// Application JSON leaves remain raw; protocol fields retain their location-specific extraction precedence.
func ExtractResponseOutputJSONDone(event ResponseOutputJSONDoneFields) json.RawMessage {
	if len(event.JSON) > 0 {
		return responseJSONClone(event.JSON)
	}
	if len(event.PartJSON) > 0 {
		return responseJSONClone(event.PartJSON)
	}
	if event.PartText != "" {
		return responseJSONNormalizeString(event.PartText)
	}
	if len(event.Output) > 0 {
		if payload := responseJSONProtocolBlob(event.Output); len(payload) > 0 {
			return payload
		}
	}
	if event.Text != "" {
		return responseJSONNormalizeString(event.Text)
	}
	if len(event.Delta) > 0 {
		if partial := responseJSONProtocolString(event.Delta, "json", "partial_json", "text"); partial != "" {
			return responseJSONNormalizeString(partial)
		}
	}
	return nil
}

// responseJSONProtocolString extracts protocol strings using the supplied field precedence.
func responseJSONProtocolString(raw json.RawMessage, keys ...string) string {
	if len(raw) == 0 {
		return ""
	}

	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		return str
	}

	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err == nil {
		for _, key := range keys {
			if key == "" {
				continue
			}
			if val, ok := obj[key]; ok {
				switch v := val.(type) {
				case string:
					return v
				case []byte:
					return string(v)
				default:
					if b, err := json.Marshal(v); err == nil {
						return string(b)
					}
				}
			}
		}
	}

	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return ""
	}
	if unquoted, err := strconv.Unquote(trimmed); err == nil {
		trimmed = unquoted
	}
	return trimmed
}

// responseJSONProtocolBlob unwraps output containers with the converter fallback semantics.
func responseJSONProtocolBlob(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var node any
	if err := json.Unmarshal(raw, &node); err != nil {
		return responseJSONClone(raw)
	}
	if payload := responseJSONProtocolNode(node); len(payload) > 0 {
		return payload
	}
	return responseJSONClone(raw)
}

// responseJSONProtocolNode selects the first extractable output value.
func responseJSONProtocolNode(node any) json.RawMessage {
	switch v := node.(type) {
	case map[string]any:
		if val, ok := v["json"]; ok {
			if payload := responseJSONProtocolNode(val); len(payload) > 0 {
				return payload
			}
		}
		if val, ok := v["text"]; ok {
			if payload := responseJSONProtocolNode(val); len(payload) > 0 {
				return payload
			}
		}
		if val, ok := v["content"]; ok {
			if payload := responseJSONProtocolNode(val); len(payload) > 0 {
				return payload
			}
		}
		if b, err := json.Marshal(v); err == nil {
			return b
		}
	case []any:
		for _, child := range v {
			if payload := responseJSONProtocolNode(child); len(payload) > 0 {
				return payload
			}
		}
		if b, err := json.Marshal(v); err == nil {
			return b
		}
	case string:
		return responseJSONNormalizeString(v)
	case json.RawMessage:
		return responseJSONClone(v)
	}
	return nil
}

// responseJSONNormalizeString trims protocol strings before optional unquoting.
func responseJSONNormalizeString(text string) json.RawMessage {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	if len(trimmed) >= 2 && ((trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"') || (trimmed[0] == '\'' && trimmed[len(trimmed)-1] == '\'')) {
		if unquoted, err := strconv.Unquote(trimmed); err == nil {
			trimmed = unquoted
		}
	}
	bytes := []byte(trimmed)
	dup := make([]byte, len(bytes))
	copy(dup, bytes)
	return json.RawMessage(dup)
}

// responseJSONClone preserves raw application JSON without numeric decoding.
func responseJSONClone(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	dup := make([]byte, len(raw))
	copy(dup, raw)
	return json.RawMessage(dup)
}
