package gemini

import "strings"

// The helpers below adapt OpenAI-style JSON schemas (tool parameters and
// structured outputs) to the subset of OpenAPI schema Gemini accepts.

// cleanJsonSchemaForGemini removes unsupported fields and converts types for Gemini API compatibility
func cleanJsonSchemaForGemini(schema any) any {
	switch v := schema.(type) {
	case map[string]any:
		cleaned := make(map[string]any)

		// List of supported fields in Gemini (from official documentation)
		supportedFields := map[string]bool{
			"anyOf": true, "enum": true, "format": true, "items": true,
			"maximum": true, "maxItems": true, "minimum": true, "minItems": true,
			"nullable": true, "properties": true, "propertyOrdering": true,
			"required": true, "type": true,
		}

		// Type mapping from lowercase to uppercase (Gemini requirement)
		typeMapping := map[string]string{
			"object":  "OBJECT",
			"array":   "ARRAY",
			"string":  "STRING",
			"number":  "NUMBER",
			"integer": "INTEGER",
			"boolean": "BOOLEAN",
			"null":    "NULL",
		}

		// Format mapping from OpenAI to Gemini supported formats
		// Based on error message: only 'enum' and 'date-time' are supported for STRING type
		formatMapping := map[string]string{
			"date":      "date-time", // Convert unsupported "date" to "date-time"
			"time":      "date-time", // Convert unsupported "time" to "date-time"
			"date-time": "date-time", // Keep supported "date-time"
			"duration":  "date-time", // Convert to supported format
			"enum":      "enum",      // Keep supported "enum"
		}

		for key, value := range v {
			// Skip unsupported fields like additionalProperties, description, strict
			if !supportedFields[key] {
				continue
			}

			switch key {
			case "type":
				// Convert type to uppercase if it's a string
				if typeStr, ok := value.(string); ok {
					if mappedType, exists := typeMapping[strings.ToLower(typeStr)]; exists {
						cleaned[key] = mappedType
					} else {
						cleaned[key] = strings.ToUpper(typeStr)
					}
				} else {
					cleaned[key] = value
				}
			case "format":
				// Map format values to Gemini-supported formats
				if formatStr, ok := value.(string); ok {
					if mappedFormat, exists := formatMapping[formatStr]; exists {
						cleaned[key] = mappedFormat
					}
					// Skip unsupported formats that have no mapping
				}
			case "properties":
				// Handle properties object - recursively clean each property
				if props, ok := value.(map[string]any); ok {
					cleanedProps := make(map[string]any)
					for propKey, propValue := range props {
						cleanedProps[propKey] = cleanJsonSchemaForGemini(propValue)
					}
					cleaned[key] = cleanedProps
				} else {
					cleaned[key] = value
				}
			case "items":
				// Handle array items schema - recursively clean
				cleaned[key] = cleanJsonSchemaForGemini(value)
			default:
				// For other supported fields, recursively clean if they're objects/arrays
				cleaned[key] = cleanJsonSchemaForGemini(value)
			}
		}
		return cleaned
	case []any:
		// Clean arrays recursively
		cleaned := make([]any, len(v))
		for i, item := range v {
			cleaned[i] = cleanJsonSchemaForGemini(item)
		}
		return cleaned
	default:
		// Return primitive values as-is
		return v
	}
}

// cleanFunctionParameters recursively removes additionalProperties and other unsupported fields from function parameters
func cleanFunctionParameters(params any) any {
	cleaned := cleanFunctionParametersInternal(params, true)

	// Gemini function declarations require parameters.type to be OBJECT (uppercase).
	// Force the top-level schema type to OBJECT to avoid Vertex 400 errors.
	if cleanedMap, ok := cleaned.(map[string]any); ok {
		cleanedMap["type"] = "OBJECT"
		return cleanedMap
	}

	return cleaned
}

// cleanFunctionParametersInternal recursively removes additionalProperties and other unsupported fields from function parameters
// isTopLevel indicates if we're at the top level where description and strict should be removed
func cleanFunctionParametersInternal(params any, isTopLevel bool) any {
	switch v := params.(type) {
	case map[string]any:
		cleaned := make(map[string]any)

		// Format mapping from OpenAI to Gemini supported formats
		// Based on error message: only 'enum' and 'date-time' are supported for STRING type
		formatMapping := map[string]string{
			"date":      "date-time", // Convert unsupported "date" to "date-time"
			"time":      "date-time", // Convert unsupported "time" to "date-time"
			"date-time": "date-time", // Keep supported "date-time"
			"duration":  "date-time", // Convert to supported format
			"enum":      "enum",      // Keep supported "enum"
		}

		for key, value := range v {
			// Skip additionalProperties at all levels
			if key == "additionalProperties" {
				continue
			}
			// Remove $schema markers; Gemini rejects JSON Schema meta keys
			if key == "$schema" {
				continue
			}
			// Skip description and strict only at top level
			if isTopLevel && (key == "description" || key == "strict") {
				continue
			}

			// Handle format field - map to supported formats
			if key == "format" {
				if formatStr, ok := value.(string); ok {
					if mappedFormat, exists := formatMapping[formatStr]; exists {
						cleaned[key] = mappedFormat
					}
					// Skip unsupported formats that have no mapping
				}
				continue
			}

			// Recursively clean nested objects (not top level anymore)
			cleaned[key] = cleanFunctionParametersInternal(value, false)
		}
		return cleaned
	case []any:
		// Clean arrays recursively
		cleaned := make([]any, len(v))
		for i, item := range v {
			cleaned[i] = cleanFunctionParametersInternal(item, false)
		}
		return cleaned
	default:
		// Return primitive values as-is
		return v
	}
}
