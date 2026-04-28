package structuredjson

import (
	"sort"
	"strings"

	"github.com/Laisky/one-api/relay/model"
)

// EnsureInstruction injects a system message that instructs the model to emit JSON
// matching the provided schema. Callers should clear ResponseFormat when downgrading
// structured output requests for providers without native JSON mode support.
func EnsureInstruction(request *model.GeneralOpenAIRequest) {
	if request == nil || request.ResponseFormat == nil || request.ResponseFormat.JsonSchema == nil {
		return
	}

	schema := request.ResponseFormat.JsonSchema
	instruction := buildInstruction(schema)

	if len(request.Messages) > 0 && request.Messages[0].Role == "system" && request.Messages[0].IsStringContent() {
		existing := request.Messages[0].StringContent()
		trimmed := strings.TrimSpace(existing)
		if trimmed == "" {
			request.Messages[0].Content = instruction
			return
		}
		if strings.Contains(existing, instruction) {
			return
		}
		request.Messages[0].Content = existing + "\n\n" + instruction
		return
	}

	request.Messages = append([]model.Message{{Role: "system", Content: instruction}}, request.Messages...)
}

func buildInstruction(schema *model.JSONSchema) string {
	if schema == nil || schema.Schema == nil {
		return "Respond ONLY with a compact JSON object containing the required fields. Do not include commentary, markdown, or additional keys."
	}

	properties, _ := schema.Schema["properties"].(map[string]any)
	if len(properties) == 0 {
		return "Respond ONLY with a compact JSON object containing the required fields. Do not include commentary, markdown, or additional keys."
	}

	segments := make([]string, 0, len(properties))
	for name, raw := range properties {
		var builder strings.Builder
		builder.WriteString(name)
		if propMap, ok := raw.(map[string]any); ok {
			var details []string
			if typeStr, ok := propMap["type"].(string); ok && typeStr != "" {
				details = append(details, typeStr)
			}
			if desc, ok := propMap["description"].(string); ok && desc != "" {
				details = append(details, desc)
			}
			if len(details) > 0 {
				builder.WriteString(" (")
				builder.WriteString(strings.Join(details, ", "))
				builder.WriteString(")")
			}
		}
		segments = append(segments, builder.String())
	}

	sort.Strings(segments)
	summary := strings.Join(segments, "; ")
	message := "Respond ONLY with a compact JSON object containing exactly these fields: " + summary + ". Do not include commentary, markdown, or additional keys."

	if schema.Description != "" {
		message = schema.Description + "\n\n" + message
	}

	return message
}
