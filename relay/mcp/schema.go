package mcp

import (
	"encoding/json"
	"math"
	"strings"

	"github.com/Laisky/errors/v2"
)

// ParseInputSchema converts a JSON schema string into a map for validation.
func ParseInputSchema(raw string) (map[string]any, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return nil, errors.Wrap(err, "parse mcp input schema")
	}
	return parsed, nil
}

// ArgumentsMatchSchema validates tool arguments against a JSON schema with best-effort checks.
func ArgumentsMatchSchema(args map[string]any, schema map[string]any) (bool, error) {
	if len(schema) == 0 {
		return true, nil
	}
	return validateValueAgainstSchema(args, schema)
}

// validateValueAgainstSchema checks a JSON value against a JSON schema definition.
func validateValueAgainstSchema(value any, schema map[string]any) (bool, error) {
	if schema == nil {
		return true, nil
	}
	if anyOf := parseSchemaList(schema["anyOf"]); len(anyOf) > 0 {
		for _, entry := range anyOf {
			match, err := validateValueAgainstSchema(value, entry)
			if err != nil {
				return false, err
			}
			if match {
				return true, nil
			}
		}
		return false, nil
	}
	if oneOf := parseSchemaList(schema["oneOf"]); len(oneOf) > 0 {
		matchCount := 0
		for _, entry := range oneOf {
			match, err := validateValueAgainstSchema(value, entry)
			if err != nil {
				return false, err
			}
			if match {
				matchCount++
			}
		}
		return matchCount == 1, nil
	}
	if allOf := parseSchemaList(schema["allOf"]); len(allOf) > 0 {
		for _, entry := range allOf {
			match, err := validateValueAgainstSchema(value, entry)
			if err != nil {
				return false, err
			}
			if !match {
				return false, nil
			}
		}
		return true, nil
	}

	schemaType := schema["type"]
	if schemaType == nil {
		if _, ok := schema["properties"]; ok {
			schemaType = "object"
		}
	}
	if schemaType != nil && !valueMatchesType(value, schemaType) {
		return false, nil
	}

	if isTypeObject(schemaType) {
		obj, ok := value.(map[string]any)
		if !ok {
			return false, nil
		}
		for _, req := range parseRequired(schema["required"]) {
			if _, ok := obj[req]; !ok {
				return false, nil
			}
		}
		properties, _ := schema["properties"].(map[string]any)
		for key, val := range obj {
			propSchema, ok := properties[key].(map[string]any)
			if !ok {
				continue
			}
			match, err := validateValueAgainstSchema(val, propSchema)
			if err != nil {
				return false, err
			}
			if !match {
				return false, nil
			}
		}
	}

	if isTypeArray(schemaType) {
		itemsSchema, _ := schema["items"].(map[string]any)
		values := normalizeArray(value)
		if values == nil {
			return false, nil
		}
		if itemsSchema != nil {
			for _, item := range values {
				match, err := validateValueAgainstSchema(item, itemsSchema)
				if err != nil {
					return false, err
				}
				if !match {
					return false, nil
				}
			}
		}
	}

	return true, nil
}

// parseSchemaList converts a schema list into a slice of schema maps.
func parseSchemaList(raw any) []map[string]any {
	entries, ok := raw.([]any)
	if !ok {
		return nil
	}
	result := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		if schema, ok := entry.(map[string]any); ok {
			result = append(result, schema)
		}
	}
	return result
}

// parseRequired extracts required field names from a JSON schema.
func parseRequired(raw any) []string {
	list, ok := raw.([]any)
	if !ok {
		if strList, ok := raw.([]string); ok {
			return strList
		}
		return nil
	}
	required := make([]string, 0, len(list))
	for _, entry := range list {
		if key, ok := entry.(string); ok {
			required = append(required, key)
		}
	}
	return required
}

// valueMatchesType checks whether a value matches the JSON schema type field.
func valueMatchesType(value any, schemaType any) bool {
	switch typed := schemaType.(type) {
	case string:
		return matchTypeString(value, typed)
	case []any:
		for _, entry := range typed {
			if matchTypeString(value, entry) {
				return true
			}
		}
		return false
	default:
		return true
	}
}

// matchTypeString matches values to JSON schema type strings.
func matchTypeString(value any, schemaType any) bool {
	if schemaType == nil {
		return true
	}
	typeName, ok := schemaType.(string)
	if !ok {
		return true
	}
	switch typeName {
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		return normalizeArray(value) != nil
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "integer":
		return isInteger(value)
	case "number":
		return isNumber(value)
	case "null":
		return value == nil
	default:
		return true
	}
}

// isTypeObject reports whether the schema type includes object.
func isTypeObject(schemaType any) bool {
	return typeIncludes(schemaType, "object")
}

// isTypeArray reports whether the schema type includes array.
func isTypeArray(schemaType any) bool {
	return typeIncludes(schemaType, "array")
}

// typeIncludes checks if a schema type includes a specific entry.
func typeIncludes(schemaType any, target string) bool {
	switch typed := schemaType.(type) {
	case string:
		return typed == target
	case []any:
		for _, entry := range typed {
			if entry == target {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// normalizeArray normalizes array-like values into a []any slice.
func normalizeArray(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	default:
		return nil
	}
}

// isInteger checks if a value is an integer-compatible JSON value.
func isInteger(value any) bool {
	switch typed := value.(type) {
	case int, int32, int64, uint, uint32, uint64:
		return true
	case float64:
		return math.Trunc(typed) == typed
	case json.Number:
		asFloat, err := typed.Float64()
		if err != nil {
			return false
		}
		return math.Trunc(asFloat) == asFloat
	default:
		return false
	}
}

// isNumber checks if a value is numeric.
func isNumber(value any) bool {
	switch value.(type) {
	case int, int32, int64, uint, uint32, uint64, float64, json.Number:
		return true
	default:
		return false
	}
}
