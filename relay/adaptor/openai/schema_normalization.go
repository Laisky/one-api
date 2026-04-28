package openai

import (
	"strings"

	"github.com/Laisky/one-api/relay/channeltype"
)

var disallowedSchemaKeys = map[string]struct{}{
	"minimum":          {},
	"maximum":          {},
	"exclusiveminimum": {},
	"exclusivemaximum": {},
}

// NormalizeStructuredJSONSchema removes schema keywords that are rejected by upstream providers
// and applies channel-specific defaults (e.g. Azure requiring additionalProperties=false).
// The schema map is modified in place; the returned map reference is the same input.
func NormalizeStructuredJSONSchema(schema map[string]any, channelType int) (map[string]any, bool) {
	if schema == nil {
		return nil, false
	}

	changed := scrubUnsupportedSchemaKeywords(schema)

	if channelType == channeltype.Azure {
		if val, exists := schema["additionalProperties"]; !exists {
			schema["additionalProperties"] = false
			changed = true
		} else if boolVal, ok := val.(bool); !ok || boolVal {
			schema["additionalProperties"] = false
			changed = true
		}
	}

	return schema, changed
}

// scrubUnsupportedSchemaKeywords recursively drops keys that commonly trigger upstream schema validation errors.
func scrubUnsupportedSchemaKeywords(node any) bool {
	changed := false

	switch typed := node.(type) {
	case map[string]any:
		for key, val := range typed {
			if _, banned := disallowedSchemaKeys[strings.ToLower(key)]; banned {
				delete(typed, key)
				changed = true
				continue
			}
			if scrubUnsupportedSchemaKeywords(val) {
				changed = true
			}
		}
	case []any:
		for _, item := range typed {
			if scrubUnsupportedSchemaKeywords(item) {
				changed = true
			}
		}
	}

	return changed
}
