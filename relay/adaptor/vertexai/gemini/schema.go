package vertexai

import (
	"maps"
	"slices"

	"github.com/Laisky/one-api/relay/adaptor/gemini"
	"github.com/Laisky/one-api/relay/model"
)

// functionDeclaration describes a Vertex function using its JSON Schema wire contract.
// The JSON Value field is mutually exclusive with the typed OpenAPI parameters field.
type functionDeclaration struct {
	Name                 string `json:"name"`
	Description          string `json:"description,omitempty"`
	ParametersJSONSchema any    `json:"parametersJsonSchema,omitempty"`
}

// configureJSONSchemas restores the caller's schemas after shared Gemini content conversion.
// Parameters: converted is the native Vertex request and source is the original request.
// It changes only Vertex schema fields and returns no values. Backend validation remains
// authoritative: a JSON Value preserves keywords but does not guarantee model enforcement.
func configureJSONSchemas(converted *gemini.ChatRequest, source *model.GeneralOpenAIRequest) {
	var declarations []functionDeclaration
	if len(source.Tools) > 0 {
		for _, tool := range source.Tools {
			if tool.Function != nil {
				declarations = append(declarations, vertexFunctionDeclaration(*tool.Function))
			}
		}
	} else {
		for _, function := range source.Functions {
			declarations = append(declarations, vertexFunctionDeclaration(function))
		}
	}
	if len(declarations) > 0 {
		converted.Tools = []gemini.ChatTools{{FunctionDeclarations: declarations}}
	}
	if source.ResponseFormat != nil && source.ResponseFormat.JsonSchema != nil {
		converted.GenerationConfig.ResponseSchema = nil
		if schema := source.ResponseFormat.JsonSchema.Schema; schema != nil {
			converted.GenerationConfig.ResponseJSONSchema = schema
		}
		converted.GenerationConfig.ResponseMimeType = "application/json"
	}
}

// vertexFunctionDeclaration converts one function to the Vertex JSON Schema contract.
// Parameters: function contains the original schema and optional legacy required names.
// Returns: a declaration preserving the schema, with legacy requirements conjoined when
// the schema is an object with a valid required list. Invalid schema values stay intact
// so the backend can reject them rather than silently repairing or weakening constraints.
func vertexFunctionDeclaration(function model.Function) functionDeclaration {
	declaration := functionDeclaration{Name: function.Name, Description: function.Description,
		ParametersJSONSchema: function.Parameters}
	if len(function.Required) == 0 {
		return declaration
	}
	schema, ok := function.Parameters.(map[string]any)
	if !ok || schema == nil {
		return declaration
	}
	if value, exists := schema["required"]; exists && value == nil {
		return declaration
	}
	var required []string
	switch value := schema["required"].(type) {
	case nil:
	case []string:
		required = slices.Clone(value)
	case []any:
		for _, item := range value {
			name, ok := item.(string)
			if !ok {
				return declaration
			}
			required = append(required, name)
		}
	default:
		return declaration
	}
	seen := make(map[string]struct{}, len(required))
	for _, name := range required {
		seen[name] = struct{}{}
	}
	for _, name := range function.Required {
		if _, exists := seen[name]; !exists {
			required = append(required, name)
			seen[name] = struct{}{}
		}
	}
	copied := maps.Clone(schema)
	copied["required"] = required
	declaration.ParametersJSONSchema = copied
	return declaration
}
