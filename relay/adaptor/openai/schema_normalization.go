package openai

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"

	"github.com/Laisky/one-api/relay/adaptor/geminiOpenaiCompatible"
	"github.com/Laisky/one-api/relay/channeltype"
)

// schemaNumericBounds records the numeric subset verified for a destination.
type schemaNumericBounds struct{ inclusive, exclusive bool }

// numericBoundsForModel selects documented bounds for registered structured-output models.
// Unknown destinations and fine-tuned OpenAI models retain conservative adaptation.
func numericBoundsForModel(channelType int, modelName string) schemaNumericBounds {
	modelName = normalizedModelName(modelName)
	switch channelType {
	case channeltype.OpenAI:
		if strings.HasPrefix(modelName, "ft:") {
			return schemaNumericBounds{}
		}
		cfg, ok := ModelRatios[modelName]
		if ok && slices.Contains(cfg.SupportedFeatures, "structured_outputs") {
			return schemaNumericBounds{inclusive: true, exclusive: true}
		}
	case channeltype.GeminiOpenAICompatible, channeltype.Gemini, channeltype.VertextAI:
		cfg, ok := geminiOpenaiCompatible.ModelRatios[modelName]
		if ok && slices.Contains(cfg.SupportedFeatures, "structured_outputs") {
			return schemaNumericBounds{inclusive: true}
		}
	}
	return schemaNumericBounds{}
}

// NormalizeStructuredJSONSchema conservatively adapts a schema with unknown
// model capabilities. It retains the original two-argument API and leaves input
// maps unchanged. Qualified request paths use NormalizeStructuredJSONSchemaForModel.
func NormalizeStructuredJSONSchema(schema map[string]any, channelType int) (map[string]any, bool) {
	return NormalizeStructuredJSONSchemaForModel(schema, channelType, "")
}

// NormalizeStructuredJSONSchemaForModel adapts numeric keywords to a verified
// provider/model subset and keeps Azure's established root default. Input maps
// remain unchanged; only modified schema branches are copied.
func NormalizeStructuredJSONSchemaForModel(schema map[string]any, channelType int, modelName string) (map[string]any, bool) {
	if schema == nil {
		return nil, false
	}
	normalized, changed := normalizeNumericSchema(schema, numericBoundsForModel(channelType, modelName))
	result := normalized.(map[string]any)
	if channelType == channeltype.Azure {
		val, ok := result["additionalProperties"].(bool)
		if !ok || val {
			if !changed {
				result = maps.Clone(result)
			}
			result["additionalProperties"] = false
			changed = true
		}
	}
	return result, changed
}

// normalizeNumericSchema visits schema nodes, not property names or literal JSON
// instances. Changed branches are copied so callers retain their original schemas.
func normalizeNumericSchema(node any, support schemaNumericBounds) (any, bool) {
	switch typed := node.(type) {
	case map[string]any:
		result := typed
		changed := false
		set := func(key string, value any, remove bool) {
			if !changed {
				result = maps.Clone(typed)
				changed = true
			}
			if remove {
				delete(result, key)
			} else {
				result[key] = value
			}
		}
		for key, value := range typed {
			lower := strings.ToLower(key)
			if ((lower == "minimum" || lower == "maximum") && !support.inclusive) ||
				((lower == "exclusiveminimum" || lower == "exclusivemaximum") && !support.exclusive) {
				set(key, nil, true)
				continue
			}
			if lower == "exclusiveminimum" || lower == "exclusivemaximum" {
				if exclusive, legacy := value.(bool); legacy {
					// Draft 4 Boolean exclusivity qualifies its inclusive bound. Modern
					// schemas use that exact number as the exclusive threshold instead.
					bound := typed["minimum"]
					if lower == "exclusivemaximum" {
						bound = typed["maximum"]
					}
					if exclusive && isSchemaNumber(bound) {
						set(key, bound, false)
					} else {
						// False adds no constraint. Unpaired Boolean forms are invalid
						// legacy schemas; retain the established conservative adaptation.
						set(key, nil, true)
					}
					continue
				}
			}
			var child any
			var modified bool
			switch key {
			case "properties", "patternProperties", "$defs", "definitions", "dependentSchemas", "dependencies":
				child, modified = normalizeSchemaDictionary(value, support)
			case "items", "prefixItems", "additionalItems", "contains", "additionalProperties", "unevaluatedProperties", "unevaluatedItems",
				"propertyNames", "allOf", "anyOf", "oneOf", "not", "if", "then", "else", "contentSchema":
				child, modified = normalizeNumericSchema(value, support)
			}
			if modified {
				set(key, child, false)
			}
		}
		return result, changed
	case []any:
		result := typed
		changed := false
		for i, value := range typed {
			child, modified := normalizeNumericSchema(value, support)
			if modified {
				if !changed {
					result = slices.Clone(typed)
					changed = true
				}
				result[i] = child
			}
		}
		return result, changed
	default:
		return node, false
	}
}

// normalizeSchemaDictionary adapts subschema values while retaining user-defined
// property and definition names, even when those names match a schema keyword.
func normalizeSchemaDictionary(node any, support schemaNumericBounds) (any, bool) {
	typed, ok := node.(map[string]any)
	if !ok {
		return node, false
	}
	result := typed
	changed := false
	for name, value := range typed {
		child, modified := normalizeNumericSchema(value, support)
		if modified {
			if !changed {
				result = maps.Clone(typed)
				changed = true
			}
			result[name] = child
		}
	}
	return result, changed
}

// isSchemaNumber recognizes decoded JSON numbers and programmatically built schemas
// without converting thresholds through floating point.
func isSchemaNumber(value any) bool {
	switch value.(type) {
	case json.Number, float32, float64, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	default:
		return false
	}
}
