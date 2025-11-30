package openai

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/songquanpeng/one-api/relay/model"
)

// ResponseAPIInput represents the input field that can be either a string or an array
type ResponseAPIInput []any

// UnmarshalJSON implements custom unmarshaling for ResponseAPIInput
// to handle both string and array inputs as per OpenAI Response API specification
func (r *ResponseAPIInput) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as string first
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		*r = ResponseAPIInput{str}
		return nil
	}

	// If string unmarshaling fails, try as array
	var arr []any
	if err := json.Unmarshal(data, &arr); err != nil {
		return errors.Wrap(err, "ResponseAPIInput.UnmarshalJSON: failed to unmarshal as array")
	}
	*r = ResponseAPIInput(arr)
	return nil
}

// MarshalJSON implements custom marshaling for ResponseAPIInput
// If the input contains only one string element, marshal as string
// Otherwise, marshal as array
func (r ResponseAPIInput) MarshalJSON() ([]byte, error) {
	// If there's exactly one element and it's a string, marshal as string
	if len(r) == 1 {
		if str, ok := r[0].(string); ok {
			b, err := json.Marshal(str)
			if err != nil {
				return nil, errors.Wrap(err, "ResponseAPIInput.MarshalJSON: failed to marshal string")
			}
			return b, nil
		}
	}
	// Otherwise, marshal as array
	b, err := json.Marshal([]any(r))
	if err != nil {
		return nil, errors.Wrap(err, "ResponseAPIInput.MarshalJSON: failed to marshal array")
	}
	return b, nil
}

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

// IsModelsOnlySupportedByChatCompletionAPI determines if a model only supports ChatCompletion API
// and should not be converted to Response API format.
// Currently returns false for all models (allowing conversion), but can be implemented later
// to return true for specific models that only support ChatCompletion API.
func IsModelsOnlySupportedByChatCompletionAPI(actualModel string) bool {
	switch {
	case strings.Contains(actualModel, "gpt") && strings.Contains(actualModel, "-search-"),
		strings.Contains(actualModel, "gpt") && strings.Contains(actualModel, "-audio-"):
		return true
	default:
		return false
	}
}

// ResponseAPIRequest represents the OpenAI Response API request structure
// https://platform.openai.com/docs/api-reference/responses
type ResponseAPIRequest struct {
	Input              ResponseAPIInput               `json:"input,omitempty"`                // Optional: Text, image, or file inputs to the model (string or array) - mutually exclusive with prompt
	Model              string                         `json:"model"`                          // Required: Model ID used to generate the response
	Background         *bool                          `json:"background,omitempty"`           // Optional: Whether to run the model response in the background
	Include            []string                       `json:"include,omitempty"`              // Optional: Additional output data to include
	Instructions       *string                        `json:"instructions,omitempty"`         // Optional: System message as the first item in the model's context
	MaxOutputTokens    *int                           `json:"max_output_tokens,omitempty"`    // Optional: Upper bound for the number of tokens
	Metadata           any                            `json:"metadata,omitempty"`             // Optional: Set of 16 key-value pairs
	ParallelToolCalls  *bool                          `json:"parallel_tool_calls,omitempty"`  // Optional: Whether to allow the model to run tool calls in parallel
	PreviousResponseId *string                        `json:"previous_response_id,omitempty"` // Optional: The unique ID of the previous response
	Prompt             *ResponseAPIPrompt             `json:"prompt,omitempty"`               // Optional: Prompt template configuration - mutually exclusive with input
	Reasoning          *model.OpenAIResponseReasoning `json:"reasoning,omitempty"`            // Optional: Configuration options for reasoning models
	ServiceTier        *string                        `json:"service_tier,omitempty"`         // Optional: Latency tier to use for processing
	Store              *bool                          `json:"store,omitempty"`                // Optional: Whether to store the generated model response
	Stream             *bool                          `json:"stream,omitempty"`               // Optional: If set to true, model response data will be streamed
	Temperature        *float64                       `json:"temperature,omitempty"`          // Optional: Sampling temperature
	Text               *ResponseTextConfig            `json:"text,omitempty"`                 // Optional: Configuration options for a text response
	ToolChoice         any                            `json:"tool_choice,omitempty"`          // Optional: How the model should select tools
	Tools              []ResponseAPITool              `json:"tools,omitempty"`                // Optional: Array of tools the model may call
	TopP               *float64                       `json:"top_p,omitempty"`                // Optional: Alternative to sampling with temperature
	Truncation         *string                        `json:"truncation,omitempty"`           // Optional: Truncation strategy
	User               *string                        `json:"user,omitempty"`                 // Optional: Stable identifier for end-users
}

// ResponseAPIPrompt represents the prompt template configuration for Response API requests
type ResponseAPIPrompt struct {
	Id        string         `json:"id"`                  // Required: Unique identifier of the prompt template
	Version   *string        `json:"version,omitempty"`   // Optional: Specific version of the prompt (defaults to "current")
	Variables map[string]any `json:"variables,omitempty"` // Optional: Map of values to substitute in for variables in the prompt
}

// ResponseAPITool represents the tool format for Response API requests
// This differs from the ChatCompletion tool format where function properties are nested
// Supports both function tools and MCP tools
type ResponseAPITool struct {
	Type        string          `json:"type"`                  // Required: "function", "web_search", "mcp", etc.
	Name        string          `json:"name,omitempty"`        // Legacy: function name when function block absent
	Description string          `json:"description,omitempty"` // Legacy: function description when function block absent
	Parameters  map[string]any  `json:"parameters,omitempty"`  // Legacy: function parameters when function block absent
	Function    *model.Function `json:"function,omitempty"`    // Modern function definition (preferred for Response API)

	// Web-search specific configuration
	SearchContextSize *string                 `json:"search_context_size,omitempty"`
	Filters           *model.WebSearchFilters `json:"filters,omitempty"`
	UserLocation      *model.UserLocation     `json:"user_location,omitempty"`

	// MCP-specific fields (for MCP tools)
	ServerLabel     string            `json:"server_label,omitempty"`
	ServerUrl       string            `json:"server_url,omitempty"`
	RequireApproval any               `json:"require_approval,omitempty"`
	AllowedTools    []string          `json:"allowed_tools,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
}

func (t ResponseAPITool) MarshalJSON() ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(t.Type)) {
	case "function":
		fn := sanitizeFunctionForRequest(t)
		payload := map[string]any{"type": "function"}
		if fn != nil {
			if name := strings.TrimSpace(fn.Name); name != "" {
				payload["name"] = name
			}
			if desc := strings.TrimSpace(fn.Description); desc != "" {
				payload["description"] = desc
			}
			if params, ok := fn.Parameters.(map[string]any); ok && len(params) > 0 {
				payload["parameters"] = params
			}
		}
		if fn != nil {
			payload["function"] = fn
		}
		return json.Marshal(payload)
	case "web_search":
		payload := map[string]any{"type": t.Type}
		if t.SearchContextSize != nil {
			payload["search_context_size"] = t.SearchContextSize
		}
		if t.Filters != nil {
			payload["filters"] = t.Filters
		}
		if t.UserLocation != nil {
			payload["user_location"] = t.UserLocation
		}
		return json.Marshal(payload)
	case "mcp":
		payload := map[string]any{"type": t.Type}
		if t.ServerLabel != "" {
			payload["server_label"] = t.ServerLabel
		}
		if t.ServerUrl != "" {
			payload["server_url"] = t.ServerUrl
		}
		if t.RequireApproval != nil {
			payload["require_approval"] = t.RequireApproval
		}
		if len(t.AllowedTools) > 0 {
			payload["allowed_tools"] = t.AllowedTools
		}
		if len(t.Headers) > 0 {
			payload["headers"] = t.Headers
		}
		return json.Marshal(payload)
	default:
		type alias ResponseAPITool
		return json.Marshal(alias(t))
	}
}

func (t *ResponseAPITool) UnmarshalJSON(data []byte) error {
	type rawTool struct {
		Type              string                  `json:"type"`
		Name              string                  `json:"name,omitempty"`
		Description       string                  `json:"description,omitempty"`
		Parameters        map[string]any          `json:"parameters,omitempty"`
		Function          json.RawMessage         `json:"function,omitempty"`
		SearchContextSize *string                 `json:"search_context_size,omitempty"`
		Filters           *model.WebSearchFilters `json:"filters,omitempty"`
		UserLocation      *model.UserLocation     `json:"user_location,omitempty"`
		ServerLabel       string                  `json:"server_label,omitempty"`
		ServerUrl         string                  `json:"server_url,omitempty"`
		RequireApproval   any                     `json:"require_approval,omitempty"`
		AllowedTools      []string                `json:"allowed_tools,omitempty"`
		Headers           map[string]string       `json:"headers,omitempty"`
	}

	var raw rawTool
	if err := json.Unmarshal(data, &raw); err != nil {
		return errors.Wrap(err, "unmarshal response api tool")
	}

	t.Type = raw.Type
	t.Name = raw.Name
	t.Description = raw.Description
	t.Parameters = raw.Parameters
	t.SearchContextSize = raw.SearchContextSize
	t.Filters = raw.Filters
	t.UserLocation = raw.UserLocation
	t.ServerLabel = raw.ServerLabel
	t.ServerUrl = raw.ServerUrl
	t.RequireApproval = raw.RequireApproval
	t.AllowedTools = raw.AllowedTools
	t.Headers = raw.Headers
	t.Function = nil

	if len(raw.Function) > 0 {
		var fn model.Function
		if err := json.Unmarshal(raw.Function, &fn); err != nil {
			return errors.Wrap(err, "unmarshal response api tool.function")
		}
		t.Function = sanitizeDecodedFunction(&fn)
	} else if raw.Type == "function" && (raw.Name != "" || raw.Description != "" || raw.Parameters != nil) {
		t.Function = &model.Function{
			Name:        raw.Name,
			Description: raw.Description,
			Parameters:  raw.Parameters,
		}
	}

	// Keep legacy fields in sync when function block is present
	if t.Function != nil {
		if t.Function.Name != "" {
			t.Name = t.Function.Name
		}
		if t.Function.Description != "" {
			t.Description = t.Function.Description
		}
		if params, ok := t.Function.Parameters.(map[string]any); ok {
			t.Parameters = params
		}
	}

	return nil
}

func sanitizeFunctionForRequest(tool ResponseAPITool) *model.Function {
	fn := tool.Function
	if fn == nil && (tool.Name != "" || tool.Description != "" || tool.Parameters != nil) {
		fn = &model.Function{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.Parameters,
		}
	}
	if fn == nil {
		return nil
	}
	clone := *fn
	clone.Arguments = nil
	return &clone
}

func sanitizeResponseAPIFunctionParameters(params any) any {
	return sanitizeResponseAPIFunctionParametersWithDepth(params, 0)
}

func sanitizeResponseAPIFunctionParametersWithDepth(params any, depth int) any {
	switch v := params.(type) {
	case map[string]any:
		cleaned := make(map[string]any, len(v))
		for key, raw := range v {
			lowerKey := strings.ToLower(key)
			if key == "$schema" || lowerKey == "additionalproperties" {
				continue
			}
			if depth == 0 && (lowerKey == "description" || lowerKey == "strict") {
				continue
			}
			cleaned[key] = sanitizeResponseAPIFunctionParametersWithDepth(raw, depth+1)
		}
		if len(cleaned) == 0 {
			return map[string]any{}
		}
		return cleaned
	case []any:
		cleaned := make([]any, 0, len(v))
		for _, item := range v {
			cleaned = append(cleaned, sanitizeResponseAPIFunctionParametersWithDepth(item, depth+1))
		}
		return cleaned
	default:
		return params
	}
}

func sanitizeResponseAPIJSONSchema(schema any) any {
	return sanitizeResponseAPIFunctionParameters(schema)
}

func sanitizeDecodedFunction(fn *model.Function) *model.Function {
	if fn == nil {
		return nil
	}
	// No special handling required today; keep hook for future sanitation.
	return fn
}

// ResponseAPIRequiredAction represents the required action block in Response API responses
type ResponseAPIRequiredAction struct {
	Type              string                        `json:"type"`
	SubmitToolOutputs *ResponseAPISubmitToolOutputs `json:"submit_tool_outputs,omitempty"`
}

// ResponseAPISubmitToolOutputs contains the tool calls that must be fulfilled by the client
type ResponseAPISubmitToolOutputs struct {
	ToolCalls []ResponseAPIToolCall `json:"tool_calls,omitempty"`
}

// ResponseAPIToolCall represents a single tool call the model wants to execute
type ResponseAPIToolCall struct {
	Id       string                   `json:"id"`
	Type     string                   `json:"type"`
	Function *ResponseAPIFunctionCall `json:"function,omitempty"`
}

// ResponseAPIFunctionCall captures the function invocation details in a tool call
type ResponseAPIFunctionCall struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// WebSearchCallAction captures metadata about a single web search invocation emitted by the OpenAI Responses API.
type WebSearchCallAction struct {
	Type    string                `json:"type,omitempty"`
	Query   string                `json:"query,omitempty"`
	Domains []string              `json:"domains,omitempty"`
	Sources []WebSearchCallSource `json:"sources,omitempty"`
}

// WebSearchCallSource represents an individual source returned by the web search tool.
type WebSearchCallSource struct {
	Url   string `json:"url,omitempty"`
	Title string `json:"title,omitempty"`
}

// ResponseTextConfig represents the text configuration for Response API
type ResponseTextConfig struct {
	Format    *ResponseTextFormat `json:"format,omitempty"`    // Optional: Format configuration for structured outputs
	Verbosity *string             `json:"verbosity,omitempty"` // Optional: Verbosity level (low, medium, high)
}

// ResponseTextFormat represents the format configuration for Response API structured outputs
type ResponseTextFormat struct {
	Type        string         `json:"type"`                  // Required: Format type (e.g., "text", "json_schema")
	Name        string         `json:"name,omitempty"`        // Optional: Schema name for json_schema type
	Description string         `json:"description,omitempty"` // Optional: Schema description
	Schema      map[string]any `json:"schema,omitempty"`      // Optional: JSON schema definition
	Strict      *bool          `json:"strict,omitempty"`      // Optional: Whether to use strict mode
}

// MCPApprovalResponseInput represents the input structure for MCP approval responses
// Used when responding to mcp_approval_request output items to approve or deny MCP tool calls
type MCPApprovalResponseInput struct {
	Type              string `json:"type"`                // Required: Always "mcp_approval_response"
	Approve           bool   `json:"approve"`             // Required: Whether to approve the MCP tool call
	ApprovalRequestId string `json:"approval_request_id"` // Required: ID of the approval request being responded to
}

// convertResponseAPIIDToToolCall converts Response API function call IDs back to ChatCompletion format
// Removes the "fc_" and "call_" prefixes to get the original ID
func convertResponseAPIIDToToolCall(fcID, callID string) string {
	if fcID != "" && strings.HasPrefix(fcID, "fc_") {
		return strings.TrimPrefix(fcID, "fc_")
	}
	if callID != "" && strings.HasPrefix(callID, "call_") {
		return strings.TrimPrefix(callID, "call_")
	}
	// Fallback to using the ID as-is
	if fcID != "" {
		return fcID
	}
	return callID
}

// convertToolCallIDToResponseAPI converts a ChatCompletion tool call ID to Response API format
// The Response API expects IDs with "fc_" prefix for function calls and "call_" prefix for call_id
func convertToolCallIDToResponseAPI(originalID string) (fcID, callID string) {
	if originalID == "" {
		return "", ""
	}

	// If the ID already has the correct prefix, use it as-is
	if strings.HasPrefix(originalID, "fc_") {
		return originalID, strings.Replace(originalID, "fc_", "call_", 1)
	}
	if strings.HasPrefix(originalID, "call_") {
		return strings.Replace(originalID, "call_", "fc_", 1), originalID
	}

	// Otherwise, generate appropriate prefixes
	return "fc_" + originalID, "call_" + originalID
}

func stringifyFunctionCallArguments(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case nil:
		return ""
	default:
		if marshaled, err := json.Marshal(v); err == nil {
			return string(marshaled)
		}
		return fmt.Sprintf("%v", v)
	}
}

func stringifyFunctionCallOutput(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case nil:
		return ""
	default:
		if marshaled, err := json.Marshal(v); err == nil {
			return string(marshaled)
		}
		return fmt.Sprintf("%v", v)
	}
}

// findToolCallName finds the function name for a given tool call ID
// func findToolCallName(toolCalls []model.Tool, toolCallId string) string {
// 	for _, toolCall := range toolCalls {
// 		if toolCall.Id == toolCallId {
// 			return toolCall.Function.Name
// 		}
// 	}
// 	return "unknown_function"
// }

// convertMessageToResponseAPIFormat converts a ChatCompletion message to Response API format
// This function handles the content type conversion from ChatCompletion format to Response API format
func convertMessageToResponseAPIFormat(message model.Message) map[string]any {
	responseMsg := map[string]any{
		"role": message.Role,
	}

	// Determine the appropriate content type based on message role
	// For Response API: user messages use "input_text", assistant messages use "output_text"
	textContentType := "input_text"
	if message.Role == "assistant" {
		textContentType = "output_text"
	}

	// Handle different content types
	switch content := message.Content.(type) {
	case string:
		// Simple string content - convert to appropriate text format based on role
		if content != "" {
			responseMsg["content"] = []map[string]any{
				{
					"type": textContentType,
					"text": content,
				},
			}
		}
	case []model.MessageContent:
		// Structured content - convert each part to Response API format
		var convertedContent []map[string]any
		for _, part := range content {
			switch part.Type {
			case model.ContentTypeText:
				if part.Text != nil && *part.Text != "" {
					item := map[string]any{
						"type": textContentType,
						"text": *part.Text,
					}
					convertedContent = append(convertedContent, sanitizeResponseAPIContentItem(item, textContentType)...)
				}
			case model.ContentTypeImageURL:
				if part.ImageURL != nil && part.ImageURL.Url != "" {
					item := map[string]any{
						"type":      "input_image",
						"image_url": part.ImageURL.Url,
					}
					// Preserve detail if provided
					if part.ImageURL.Detail != "" {
						item["detail"] = part.ImageURL.Detail
					}
					convertedContent = append(convertedContent, sanitizeResponseAPIContentItem(item, textContentType)...)
				}
			case model.ContentTypeInputAudio:
				if part.InputAudio != nil {
					item := map[string]any{
						"type":        "input_audio",
						"input_audio": part.InputAudio,
					}
					convertedContent = append(convertedContent, sanitizeResponseAPIContentItem(item, textContentType)...)
				}
			default:
				// For unknown types, try to preserve as much as possible
				partMap := map[string]any{
					"type": textContentType, // Use appropriate text type based on role
				}
				if part.Text != nil {
					partMap["text"] = *part.Text
				}
				convertedContent = append(convertedContent, sanitizeResponseAPIContentItem(partMap, textContentType)...)
			}
		}
		if len(convertedContent) > 0 {
			responseMsg["content"] = convertedContent
		}
	case []any:
		// Handle generic interface array (from JSON unmarshaling)
		var convertedContent []map[string]any
		for _, item := range content {
			if itemMap, ok := item.(map[string]any); ok {
				convertedItem := make(map[string]any)
				maps.Copy(convertedItem, itemMap)
				// Convert content types to Response API format based on message role
				if itemType, exists := itemMap["type"]; exists {
					switch itemType {
					case "text":
						convertedItem["type"] = textContentType
					case "image_url":
						convertedItem["type"] = "input_image"
						// Flatten image_url object to string and hoist detail per Response API spec
						if iu, ok := itemMap["image_url"].(map[string]any); ok {
							if urlVal, ok2 := iu["url"].(string); ok2 {
								convertedItem["image_url"] = urlVal
							}
							if detailVal, ok2 := iu["detail"].(string); ok2 && detailVal != "" {
								convertedItem["detail"] = detailVal
							}
						} else if urlStr, ok := itemMap["image_url"].(string); ok {
							convertedItem["image_url"] = urlStr
						}
					}
				}
				sanitizedItems := sanitizeResponseAPIContentItem(convertedItem, textContentType)
				if len(sanitizedItems) > 0 {
					convertedContent = append(convertedContent, sanitizedItems...)
				}
			}
		}
		if len(convertedContent) > 0 {
			responseMsg["content"] = convertedContent
		}
	default:
		// Fallback: convert to string and treat as appropriate text type based on role
		if contentStr := fmt.Sprintf("%v", content); contentStr != "" && contentStr != "<nil>" {
			responseMsg["content"] = []map[string]any{
				{
					"type": textContentType,
					"text": contentStr,
				},
			}
		}
	}

	// Add other message fields if present
	if message.Name != nil {
		responseMsg["name"] = *message.Name
	}

	if _, hasContent := responseMsg["content"]; !hasContent {
		return nil
	}

	return responseMsg
}

func sanitizeResponseAPIContentItem(item map[string]any, textContentType string) []map[string]any {
	if item == nil {
		return nil
	}

	itemType, _ := item["type"].(string)

	// Reasoning items from non-OpenAI providers may carry encrypted payloads that OpenAI cannot verify.
	// Convert them into plain text summaries to preserve user-visible context while avoiding upstream errors.
	if itemType == "reasoning" {
		if summaryText := extractReasoningSummaryText(item); summaryText != "" {
			return []map[string]any{{
				"type": textContentType,
				"text": summaryText,
			}}
		}
		if text, ok := item["text"].(string); ok && strings.TrimSpace(text) != "" {
			return []map[string]any{{
				"type": textContentType,
				"text": text,
			}}
		}
		// Drop unverifiable reasoning items if no readable summary is available.
		return nil
	}

	// openai do not support encrypted_content in reasoning history,
	// could cause 400 error
	delete(item, "encrypted_content")

	return []map[string]any{item}
}
func extractReasoningSummaryText(item map[string]any) string {
	summary, ok := item["summary"].([]any)
	if !ok {
		return ""
	}

	var builder strings.Builder
	for _, entry := range summary {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		text, ok := entryMap["text"].(string)
		if !ok {
			continue
		}
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(trimmed)
	}

	return builder.String()
}

// ConvertChatCompletionToResponseAPI converts a ChatCompletion request to Response API format
func ConvertChatCompletionToResponseAPI(request *model.GeneralOpenAIRequest) *ResponseAPIRequest {
	responseReq := &ResponseAPIRequest{
		Model: request.Model,
		Input: make(ResponseAPIInput, 0, len(request.Messages)),
	}

	// Convert messages to input - Response API expects messages directly in the input array
	toolNameByID := make(map[string]string)

	for _, message := range request.Messages {
		switch message.Role {
		case "assistant":
			if convertedMsg := convertMessageToResponseAPIFormat(message); convertedMsg != nil {
				responseReq.Input = append(responseReq.Input, convertedMsg)
			}
			if len(message.ToolCalls) == 0 {
				continue
			}
			for _, toolCall := range message.ToolCalls {
				if toolCall.Function != nil && toolCall.Function.Name != "" && toolCall.Id != "" {
					toolNameByID[toolCall.Id] = toolCall.Function.Name
				}
				fcID, callID := convertToolCallIDToResponseAPI(toolCall.Id)
				item := map[string]any{
					"type": "function_call",
				}
				if fcID != "" {
					item["id"] = fcID
				}
				if callID != "" {
					item["call_id"] = callID
				}
				if toolCall.Function != nil {
					if toolCall.Function.Name != "" {
						item["name"] = toolCall.Function.Name
					}
					if args := toolCall.Function.Arguments; args != "" {
						item["arguments"] = args
					}
				}
				responseReq.Input = append(responseReq.Input, item)
			}
		case "tool":
			fcID, callID := convertToolCallIDToResponseAPI(message.ToolCallId)
			item := map[string]any{
				"type": "function_call_output",
			}
			if fcID != "" {
				item["id"] = fcID
			}
			if callID != "" {
				item["call_id"] = callID
			}
			if output := message.StringContent(); output != "" {
				item["output"] = output
			}
			responseReq.Input = append(responseReq.Input, item)
		default:
			if convertedMsg := convertMessageToResponseAPIFormat(message); convertedMsg != nil {
				responseReq.Input = append(responseReq.Input, convertedMsg)
			}
		}
	}

	// Map other fields
	// Prefer MaxCompletionTokens; fall back to deprecated MaxTokens for compatibility
	if request.MaxCompletionTokens != nil && *request.MaxCompletionTokens > 0 {
		responseReq.MaxOutputTokens = request.MaxCompletionTokens
	} else if request.MaxTokens > 0 {
		responseReq.MaxOutputTokens = &request.MaxTokens
	}

	responseReq.Temperature = request.Temperature
	responseReq.TopP = request.TopP
	responseReq.Stream = &request.Stream
	responseReq.User = &request.User
	responseReq.Store = request.Store
	responseReq.Metadata = request.Metadata

	if request.ServiceTier != nil {
		responseReq.ServiceTier = request.ServiceTier
	}

	if request.ParallelTooCalls != nil {
		responseReq.ParallelToolCalls = request.ParallelTooCalls
	}

	// Handle tools (modern format)
	responseAPITools := make([]ResponseAPITool, 0, len(request.Tools)+len(request.Functions)+1)
	webSearchAdded := false

	if len(request.Tools) > 0 {
		for _, tool := range request.Tools {
			switch tool.Type {
			case "mcp":
				responseAPITools = append(responseAPITools, ResponseAPITool{
					Type:            tool.Type,
					ServerLabel:     tool.ServerLabel,
					ServerUrl:       tool.ServerUrl,
					RequireApproval: tool.RequireApproval,
					AllowedTools:    tool.AllowedTools,
					Headers:         tool.Headers,
				})
			case "web_search":
				responseAPITools = append(responseAPITools, ResponseAPITool{
					Type:              "web_search",
					SearchContextSize: tool.SearchContextSize,
					Filters:           tool.Filters,
					UserLocation:      tool.UserLocation,
				})
				webSearchAdded = true
			default:
				if tool.Function == nil {
					continue
				}
				responseAPITool := ResponseAPITool{
					Type:        tool.Type,
					Name:        tool.Function.Name,
					Description: tool.Function.Description,
					Function: &model.Function{
						Name:        tool.Function.Name,
						Description: tool.Function.Description,
						Parameters:  tool.Function.Parameters,
						Required:    tool.Function.Required,
						Strict:      tool.Function.Strict,
					},
				}
				if tool.Function.Parameters != nil {
					if params, ok := tool.Function.Parameters.(map[string]any); ok {
						responseAPITool.Parameters = params
					}
				}
				responseAPITools = append(responseAPITools, responseAPITool)
			}
		}
		if request.ToolChoice != nil {
			if normalized, changed := NormalizeToolChoiceForResponse(request.ToolChoice); changed {
				responseReq.ToolChoice = normalized
			} else {
				responseReq.ToolChoice = request.ToolChoice
			}
		}
	}

	if len(request.Functions) > 0 {
		for _, function := range request.Functions {
			responseAPITool := ResponseAPITool{
				Type:        "function",
				Name:        function.Name,
				Description: function.Description,
				Function: &model.Function{
					Name:        function.Name,
					Description: function.Description,
					Parameters:  function.Parameters,
					Required:    function.Required,
					Strict:      function.Strict,
				},
			}
			if function.Parameters != nil {
				if params, ok := function.Parameters.(map[string]any); ok {
					responseAPITool.Parameters = params
				}
			}
			responseAPITools = append(responseAPITools, responseAPITool)
		}
		if request.FunctionCall != nil {
			if normalized, changed := NormalizeToolChoiceForResponse(request.FunctionCall); changed {
				responseReq.ToolChoice = normalized
			} else {
				responseReq.ToolChoice = request.FunctionCall
			}
		}
	}

	if !webSearchAdded && request.WebSearchOptions != nil {
		responseAPITools = append(responseAPITools, convertWebSearchOptionsToTool(request.WebSearchOptions))
		webSearchAdded = true
	}

	if len(responseAPITools) > 0 {
		responseReq.Tools = responseAPITools
	}

	// Handle thinking/reasoning
	if isModelSupportedReasoning(request.Model) {
		if responseReq.Reasoning == nil {
			responseReq.Reasoning = &model.OpenAIResponseReasoning{}
		}

		normalizedEffort := normalizeReasoningEffortForModel(request.Model, request.ReasoningEffort)
		responseReq.Reasoning.Effort = normalizedEffort
		request.ReasoningEffort = normalizedEffort

		if responseReq.Reasoning.Summary == nil {
			reasoningSummary := "auto"
			responseReq.Reasoning.Summary = &reasoningSummary
		}
	} else {
		request.ReasoningEffort = nil
	}

	// Handle response format
	if request.ResponseFormat != nil {
		textConfig := &ResponseTextConfig{
			Format: &ResponseTextFormat{
				Type: request.ResponseFormat.Type,
			},
		}

		// Handle structured output with JSON schema
		if request.ResponseFormat.JsonSchema != nil {
			textConfig.Format.Name = request.ResponseFormat.JsonSchema.Name
			textConfig.Format.Description = request.ResponseFormat.JsonSchema.Description
			textConfig.Format.Schema = request.ResponseFormat.JsonSchema.Schema
			textConfig.Format.Strict = request.ResponseFormat.JsonSchema.Strict
		}

		responseReq.Text = textConfig
	}

	// Handle verbosity parameter (GPT-5 series)
	// In Response API, verbosity goes into text.verbosity
	if request.Verbosity != nil {
		if responseReq.Text == nil {
			responseReq.Text = &ResponseTextConfig{}
		}
		responseReq.Text.Verbosity = request.Verbosity
	}

	// Handle system message as instructions
	if len(request.Messages) > 0 && request.Messages[0].Role == "system" {
		systemContent := request.Messages[0].StringContent()
		responseReq.Instructions = &systemContent

		// Remove system message from input since it's now in instructions
		responseReq.Input = responseReq.Input[1:]
	}

	return responseReq
}

// ConvertResponseAPIToChatCompletionRequest converts a Response API request into a
// ChatCompletion request for providers that do not support Response API natively.
func ConvertResponseAPIToChatCompletionRequest(request *ResponseAPIRequest) (*model.GeneralOpenAIRequest, error) {
	if request == nil {
		return nil, errors.New("response api request is nil")
	}

	if request.Prompt != nil {
		return nil, errors.New("prompt templates are not supported for this channel")
	}

	if request.Background != nil && *request.Background {
		return nil, errors.New("background responses are not supported for this channel")
	}

	if normalized, changed := NormalizeToolChoice(request.ToolChoice); changed {
		request.ToolChoice = normalized
	}

	chatReq := &model.GeneralOpenAIRequest{
		Model:       request.Model,
		Store:       request.Store,
		Metadata:    request.Metadata,
		Stream:      request.Stream != nil && *request.Stream,
		Reasoning:   request.Reasoning,
		ServiceTier: request.ServiceTier,
		Temperature: request.Temperature,
		TopP:        request.TopP,
		ToolChoice:  request.ToolChoice,
	}

	if request.MaxOutputTokens != nil {
		chatReq.MaxCompletionTokens = request.MaxOutputTokens
	}
	if request.User != nil {
		chatReq.User = *request.User
	}
	chatReq.ParallelTooCalls = request.ParallelToolCalls

	if request.Text != nil && request.Text.Format != nil {
		chatReq.ResponseFormat = &model.ResponseFormat{Type: request.Text.Format.Type}
		if strings.EqualFold(request.Text.Format.Type, "json_schema") {
			sanitized := sanitizeResponseAPIJSONSchema(request.Text.Format.Schema)
			schemaMap, _ := sanitized.(map[string]any)
			chatReq.ResponseFormat.JsonSchema = &model.JSONSchema{
				Name:        request.Text.Format.Name,
				Description: request.Text.Format.Description,
				Schema:      schemaMap,
			}
			chatReq.ResponseFormat.JsonSchema.Strict = nil
		}
	}

	// Handle verbosity parameter (GPT-5 series)
	// In Response API, verbosity is in text.verbosity; convert to top-level for ChatCompletion
	if request.Text != nil && request.Text.Verbosity != nil {
		chatReq.Verbosity = request.Text.Verbosity
	}

	if len(request.Tools) > 0 {
		chatReq.Tools = convertResponseAPITools(request.Tools)
		if len(chatReq.Tools) == 0 {
			chatReq.Tools = nil
		}
	}

	if chatReq.ToolChoice != nil {
		chatReq.ToolChoice = sanitizeToolChoiceAgainstTools(chatReq.ToolChoice, chatReq.Tools)
	}

	if request.Instructions != nil && *request.Instructions != "" {
		chatReq.Messages = append(chatReq.Messages, model.Message{
			Role:    "system",
			Content: *request.Instructions,
		})
	}

	for _, item := range request.Input {
		switch v := item.(type) {
		case string:
			chatReq.Messages = append(chatReq.Messages, model.Message{Role: "user", Content: v})
		case map[string]any:
			if typeVal, ok := v["type"].(string); ok {
				switch strings.ToLower(typeVal) {
				case "function_call":
					fcID, _ := v["id"].(string)
					callID, _ := v["call_id"].(string)
					normalizedID := convertResponseAPIIDToToolCall(fcID, callID)
					if normalizedID == "" && callID != "" {
						normalizedID = callID
					} else if normalizedID == "" && fcID != "" {
						normalizedID = fcID
					}

					name, _ := v["name"].(string)
					arguments := stringifyFunctionCallArguments(v["arguments"])

					toolCall := model.Tool{
						Id:   normalizedID,
						Type: "function",
						Function: &model.Function{
							Name:      name,
							Arguments: arguments,
						},
					}

					role := "assistant"
					if r, ok := v["role"].(string); ok && r != "" {
						role = r
					}

					chatReq.Messages = append(chatReq.Messages, model.Message{
						Role:      role,
						ToolCalls: []model.Tool{toolCall},
					})
					continue
				case "function_call_output":
					fcID, _ := v["id"].(string)
					callID, _ := v["call_id"].(string)
					normalizedID := convertResponseAPIIDToToolCall(fcID, callID)
					if normalizedID == "" && callID != "" {
						normalizedID = callID
					} else if normalizedID == "" && fcID != "" {
						normalizedID = fcID
					}

					output := stringifyFunctionCallOutput(v["output"])
					if output == "" {
						output = stringifyFunctionCallOutput(v["content"])
					}

					role := "tool"
					if r, ok := v["role"].(string); ok && r != "" {
						role = r
					}

					chatReq.Messages = append(chatReq.Messages, model.Message{
						Role:       role,
						ToolCallId: normalizedID,
						Content:    output,
					})
					continue
				}
			}
			msg, err := responseContentItemToMessage(v)
			if err != nil {
				return nil, errors.Wrap(err, "convert response api content to chat message")
			}
			chatReq.Messages = append(chatReq.Messages, *msg)
		default:
			return nil, errors.Errorf("unsupported input item of type %T", item)
		}
	}

	return chatReq, nil
}

func responseContentItemToMessage(item map[string]any) (*model.Message, error) {
	role := "user"
	if r, ok := item["role"].(string); ok && r != "" {
		role = r
	}

	var namePtr *string
	if name, ok := item["name"].(string); ok && name != "" {
		namePtr = &name
	}

	contentVal, ok := item["content"]
	if !ok {
		return &model.Message{Role: role, Name: namePtr, Content: ""}, nil
	}

	message := &model.Message{Role: role, Name: namePtr}

	switch content := contentVal.(type) {
	case string:
		message.Content = content
	case []any:
		parts := make([]model.MessageContent, 0, len(content))
		textSections := make([]string, 0, len(content))
		hasNonText := false
		for _, raw := range content {
			partMap, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			typeStr, _ := partMap["type"].(string)
			switch typeStr {
			case "input_text", "output_text":
				if text, ok := partMap["text"].(string); ok {
					parts = append(parts, model.MessageContent{Type: model.ContentTypeText, Text: &text})
					textSections = append(textSections, text)
				}
			case "input_image":
				if url, ok := partMap["image_url"].(string); ok {
					image := &model.ImageURL{Url: url}
					if detail, ok := partMap["detail"].(string); ok {
						image.Detail = detail
					}
					parts = append(parts, model.MessageContent{Type: model.ContentTypeImageURL, ImageURL: image})
					hasNonText = true
				}
			case "input_audio":
				if inputAudio, ok := partMap["input_audio"].(map[string]any); ok {
					data, _ := inputAudio["data"].(string)
					format, _ := inputAudio["format"].(string)
					parts = append(parts, model.MessageContent{
						Type:       model.ContentTypeInputAudio,
						InputAudio: &model.InputAudio{Data: data, Format: format},
					})
					hasNonText = true
				}
			case "reasoning":
				if text, ok := partMap["text"].(string); ok && text != "" {
					message.SetReasoningContent(string(model.ReasoningFormatReasoning), text)
				}
			default:
				if text, ok := partMap["text"].(string); ok {
					parts = append(parts, model.MessageContent{Type: model.ContentTypeText, Text: &text})
					textSections = append(textSections, text)
				} else {
					hasNonText = true
				}
			}
		}
		if len(parts) > 0 {
			if !hasNonText && len(textSections) == len(parts) && len(textSections) > 0 {
				message.Content = strings.Join(textSections, "\n")
			} else {
				message.Content = parts
			}
		}
	default:
		return nil, errors.Errorf("unsupported content type %T", contentVal)
	}

	return message, nil
}

func convertWebSearchOptionsToTool(options *model.WebSearchOptions) ResponseAPITool {
	tool := ResponseAPITool{Type: "web_search"}
	if options == nil {
		return tool
	}
	tool.SearchContextSize = options.SearchContextSize
	tool.Filters = options.Filters
	tool.UserLocation = options.UserLocation
	return tool
}

func convertResponseAPITools(tools []ResponseAPITool) []model.Tool {
	if len(tools) == 0 {
		return nil
	}
	converted := make([]model.Tool, 0, len(tools))
	for _, tool := range tools {
		toolType := strings.ToLower(strings.TrimSpace(tool.Type))
		switch toolType {
		case "function":
			fn := sanitizeFunctionForRequest(tool)
			if fn == nil {
				continue
			}
			fn.Strict = nil
			if fn.Parameters != nil {
				sanitized := sanitizeResponseAPIFunctionParameters(fn.Parameters)
				if paramsMap, ok := sanitized.(map[string]any); ok {
					if len(paramsMap) == 0 {
						fn.Parameters = map[string]any{}
					} else {
						fn.Parameters = paramsMap
					}
				} else {
					fn.Parameters = sanitized
				}
			}
			converted = append(converted, model.Tool{
				Type:     "function",
				Function: fn,
			})
		case "web_search", "web_search_preview":
			// Web search tools are not supported when downgrading Response API requests
			// to Chat Completions. Skip them to avoid upstream validation errors.
			continue
		default:
			// Non-function tools (e.g. MCP, code interpreter) cannot be expressed for
			// channels that only understand Chat Completions. Drop them so fallback
			// requests remain compatible.
			continue
		}
	}
	return converted
}

func sanitizeToolChoiceAgainstTools(choice any, tools []model.Tool) any {
	if choice == nil {
		return nil
	}

	normalized, _ := NormalizeToolChoice(choice)
	asMap, ok := normalized.(map[string]any)
	if !ok {
		return normalized
	}

	typeVal, _ := asMap["type"].(string)
	switch strings.ToLower(strings.TrimSpace(typeVal)) {
	case "", "auto", "none":
		return normalized
	case "tool":
		name, _ := asMap["name"].(string)
		if name == "" {
			return normalized
		}
		for _, tool := range tools {
			if tool.Function != nil && tool.Function.Name == name {
				return normalized
			}
		}
		return map[string]any{"type": "auto"}
	case "function":
		functionPayload, _ := asMap["function"].(map[string]any)
		name, _ := functionPayload["name"].(string)
		if name == "" {
			return normalized
		}
		for _, tool := range tools {
			if tool.Function != nil && tool.Function.Name == name {
				return normalized
			}
		}
		return map[string]any{"type": "auto"}
	default:
		return normalized
	}
}

func isChargeableWebSearchAction(item OutputItem) bool {
	if item.Type != "web_search_call" {
		return false
	}
	if item.Action == nil {
		return true
	}
	actionType := strings.ToLower(strings.TrimSpace(item.Action.Type))
	return actionType == "" || actionType == "search"
}

func countWebSearchSearchActions(outputs []OutputItem) int {
	return countNewWebSearchSearchActions(outputs, make(map[string]struct{}))
}

func countNewWebSearchSearchActions(outputs []OutputItem, seen map[string]struct{}) int {
	added := 0
	for _, item := range outputs {
		if !isChargeableWebSearchAction(item) {
			continue
		}
		key := item.Id
		if key == "" && item.Action != nil {
			if item.Action.Query != "" {
				key = item.Action.Query
			} else if len(item.Action.Domains) > 0 {
				key = strings.Join(item.Action.Domains, ",")
			}
		}
		if key == "" {
			key = fmt.Sprintf("anon-%d", len(seen)+added)
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		added++
	}
	return added
}

// ResponseAPIUsage represents the usage information structure for Response API
// Response API uses different field names than Chat Completions API
type ResponseAPIUsage struct {
	InputTokens         int                             `json:"input_tokens"`
	OutputTokens        int                             `json:"output_tokens"`
	TotalTokens         int                             `json:"total_tokens"`
	InputTokensDetails  *ResponseAPIInputTokensDetails  `json:"input_tokens_details,omitempty"`
	OutputTokensDetails *ResponseAPIOutputTokensDetails `json:"output_tokens_details,omitempty"`
}

// ResponseAPIInputTokensDetails models the nested usage block returned by the OpenAI Response API.
// The schema is not stable yet (especially for web-search fields), so we keep a map of additional
// properties while still projecting the common fields into strong types.
type ResponseAPIInputTokensDetails struct {
	CachedTokens int `json:"cached_tokens,omitempty"`
	AudioTokens  int `json:"audio_tokens,omitempty"`
	TextTokens   int `json:"text_tokens,omitempty"`
	ImageTokens  int `json:"image_tokens,omitempty"`
	WebSearch    any `json:"web_search,omitempty"`
	additional   map[string]any
}

// ResponseAPIOutputTokensDetails models the completion-side usage details returned by the Response API.
type ResponseAPIOutputTokensDetails struct {
	ReasoningTokens          int `json:"reasoning_tokens,omitempty"`
	AudioTokens              int `json:"audio_tokens,omitempty"`
	AcceptedPredictionTokens int `json:"accepted_prediction_tokens,omitempty"`
	RejectedPredictionTokens int `json:"rejected_prediction_tokens,omitempty"`
	TextTokens               int `json:"text_tokens,omitempty"`
	CachedTokens             int `json:"cached_tokens,omitempty"`
	additional               map[string]any
}

func (d *ResponseAPIInputTokensDetails) UnmarshalJSON(data []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	// Reset existing values so the struct can be reused.
	*d = ResponseAPIInputTokensDetails{}
	if len(raw) == 0 {
		return nil
	}

	additional := make(map[string]any)
	for key, value := range raw {
		switch key {
		case "cached_tokens":
			d.CachedTokens = coerceNonNegativeInt(value)
		case "audio_tokens":
			d.AudioTokens = coerceNonNegativeInt(value)
		case "text_tokens":
			d.TextTokens = coerceNonNegativeInt(value)
		case "image_tokens":
			d.ImageTokens = coerceNonNegativeInt(value)
		case "web_search":
			d.WebSearch = value
		default:
			additional[key] = value
		}
	}

	if len(additional) > 0 {
		d.additional = additional
	}

	return nil
}

func (d ResponseAPIInputTokensDetails) MarshalJSON() ([]byte, error) {
	if d.additional == nil && d.WebSearch == nil && d.CachedTokens == 0 && d.AudioTokens == 0 && d.TextTokens == 0 && d.ImageTokens == 0 {
		return []byte("{}"), nil
	}

	raw := make(map[string]any, len(d.additional)+6)
	maps.Copy(raw, d.additional)
	if d.CachedTokens != 0 {
		raw["cached_tokens"] = d.CachedTokens
	}
	if d.AudioTokens != 0 {
		raw["audio_tokens"] = d.AudioTokens
	}
	if d.TextTokens != 0 {
		raw["text_tokens"] = d.TextTokens
	}
	if d.ImageTokens != 0 {
		raw["image_tokens"] = d.ImageTokens
	}
	if d.WebSearch != nil {
		raw["web_search"] = d.WebSearch
	}

	return json.Marshal(raw)
}

// WebSearchInvocationCount extracts the number of billable web search invocations recorded in the
// input token details payload returned by the Responses API. The upstream schema is still evolving,
// so this helper defensively inspects several possible representations (numeric counters, strings,
// or nested structures containing request entries).
func (d *ResponseAPIInputTokensDetails) WebSearchInvocationCount() int {
	if d == nil || d.WebSearch == nil {
		return 0
	}
	return extractWebSearchInvocationCount(d.WebSearch)
}

func (d *ResponseAPIOutputTokensDetails) UnmarshalJSON(data []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	*d = ResponseAPIOutputTokensDetails{}
	if len(raw) == 0 {
		return nil
	}

	additional := make(map[string]any)
	for key, value := range raw {
		switch key {
		case "reasoning_tokens":
			d.ReasoningTokens = coerceNonNegativeInt(value)
		case "audio_tokens":
			d.AudioTokens = coerceNonNegativeInt(value)
		case "accepted_prediction_tokens":
			d.AcceptedPredictionTokens = coerceNonNegativeInt(value)
		case "rejected_prediction_tokens":
			d.RejectedPredictionTokens = coerceNonNegativeInt(value)
		case "text_tokens":
			d.TextTokens = coerceNonNegativeInt(value)
		case "cached_tokens":
			d.CachedTokens = coerceNonNegativeInt(value)
		default:
			additional[key] = value
		}
	}

	if len(additional) > 0 {
		d.additional = additional
	}

	return nil
}

func (d ResponseAPIOutputTokensDetails) MarshalJSON() ([]byte, error) {
	if d.additional == nil && d.ReasoningTokens == 0 && d.AudioTokens == 0 && d.AcceptedPredictionTokens == 0 && d.RejectedPredictionTokens == 0 && d.TextTokens == 0 && d.CachedTokens == 0 {
		return []byte("{}"), nil
	}

	raw := make(map[string]any, len(d.additional)+6)
	maps.Copy(raw, d.additional)
	if d.ReasoningTokens != 0 {
		raw["reasoning_tokens"] = d.ReasoningTokens
	}
	if d.AudioTokens != 0 {
		raw["audio_tokens"] = d.AudioTokens
	}
	if d.AcceptedPredictionTokens != 0 {
		raw["accepted_prediction_tokens"] = d.AcceptedPredictionTokens
	}
	if d.RejectedPredictionTokens != 0 {
		raw["rejected_prediction_tokens"] = d.RejectedPredictionTokens
	}
	if d.TextTokens != 0 {
		raw["text_tokens"] = d.TextTokens
	}
	if d.CachedTokens != 0 {
		raw["cached_tokens"] = d.CachedTokens
	}

	return json.Marshal(raw)
}

// extractWebSearchInvocationCount normalizes disparate web search metadata structures into a
// concrete invocation count. OpenAI has experimented with multiple shapes (numeric counters,
// nested maps, or arrays), so we need to support all forms without failing hard on unknown data.
func extractWebSearchInvocationCount(raw any) int {
	switch v := raw.(type) {
	case nil:
		return 0
	case int:
		if v > 0 {
			return v
		}
	case int32:
		if v > 0 {
			return int(v)
		}
	case int64:
		if v > 0 {
			return int(v)
		}
	case float32:
		if v > 0 {
			return int(math.Round(float64(v)))
		}
	case float64:
		if v > 0 {
			return int(math.Round(v))
		}
	case json.Number:
		if f, err := v.Float64(); err == nil {
			return extractWebSearchInvocationCount(f)
		}
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return 0
		}
		if f, err := strconv.ParseFloat(trimmed, 64); err == nil {
			return extractWebSearchInvocationCount(f)
		}
	case []any:
		total := 0
		for _, item := range v {
			if count := extractWebSearchInvocationCount(item); count > 0 {
				total += count
			}
		}
		if total > 0 {
			return total
		}
		if len(v) > 0 {
			return len(v)
		}
	case map[string]any:
		// Prefer well-known counter keys first to avoid double counting.
		candidates := []string{"requests", "request_count", "count", "total_requests", "queries", "query_count", "calls", "invocations"}
		for _, key := range candidates {
			for actualKey, value := range v {
				if strings.EqualFold(actualKey, key) {
					if count := extractWebSearchInvocationCount(value); count > 0 {
						return count
					}
				}
			}
		}
		// As a defensive fallback, inspect remaining values and return the first positive count.
		for _, value := range v {
			if count := extractWebSearchInvocationCount(value); count > 0 {
				return count
			}
		}
	}
	return 0
}

func (d *ResponseAPIInputTokensDetails) toModel() *model.UsagePromptTokensDetails {
	if d == nil {
		return nil
	}

	details := &model.UsagePromptTokensDetails{
		CachedTokens: d.CachedTokens,
		AudioTokens:  d.AudioTokens,
		TextTokens:   d.TextTokens,
		ImageTokens:  d.ImageTokens,
	}
	return details
}

func (d *ResponseAPIOutputTokensDetails) toModel() *model.UsageCompletionTokensDetails {
	if d == nil {
		return nil
	}

	return &model.UsageCompletionTokensDetails{
		ReasoningTokens:          d.ReasoningTokens,
		AudioTokens:              d.AudioTokens,
		AcceptedPredictionTokens: d.AcceptedPredictionTokens,
		RejectedPredictionTokens: d.RejectedPredictionTokens,
		TextTokens:               d.TextTokens,
		CachedTokens:             d.CachedTokens,
	}
}

func newResponseAPIInputTokensDetailsFromModel(details *model.UsagePromptTokensDetails) *ResponseAPIInputTokensDetails {
	if details == nil {
		return nil
	}

	converted := &ResponseAPIInputTokensDetails{
		CachedTokens: details.CachedTokens,
		AudioTokens:  details.AudioTokens,
		TextTokens:   details.TextTokens,
		ImageTokens:  details.ImageTokens,
	}
	return converted
}

func newResponseAPIOutputTokensDetailsFromModel(details *model.UsageCompletionTokensDetails) *ResponseAPIOutputTokensDetails {
	if details == nil {
		return nil
	}

	return &ResponseAPIOutputTokensDetails{
		ReasoningTokens:          details.ReasoningTokens,
		AudioTokens:              details.AudioTokens,
		AcceptedPredictionTokens: details.AcceptedPredictionTokens,
		RejectedPredictionTokens: details.RejectedPredictionTokens,
		TextTokens:               details.TextTokens,
		CachedTokens:             details.CachedTokens,
	}
}

func coerceNonNegativeInt(value any) int {
	const maxInt = int(^uint(0) >> 1)

	switch v := value.(type) {
	case nil:
		return 0
	case int:
		if v < 0 {
			return 0
		}
		return v
	case int8:
		if v < 0 {
			return 0
		}
		return int(v)
	case int16:
		if v < 0 {
			return 0
		}
		return int(v)
	case int32:
		if v < 0 {
			return 0
		}
		return int(v)
	case int64:
		if v < 0 {
			return 0
		}
		return int(v)
	case uint:
		return int(v)
	case uint8:
		return int(v)
	case uint16:
		return int(v)
	case uint32:
		return int(v)
	case uint64:
		if v > uint64(maxInt) {
			return maxInt
		}
		return int(v)
	case float32:
		if v < 0 {
			return 0
		}
		return int(v)
	case float64:
		if v < 0 {
			return 0
		}
		return int(v)
	case json.Number:
		if i, err := v.Int64(); err == nil {
			if i < 0 {
				return 0
			}
			return int(i)
		}
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return 0
		}
		if i, err := strconv.ParseFloat(s, 64); err == nil {
			if i < 0 {
				return 0
			}
			return int(i)
		}
	}

	return 0
}

// ToModelUsage converts ResponseAPIUsage to model.Usage for compatibility.
func (r *ResponseAPIUsage) ToModelUsage() *model.Usage {
	if r == nil {
		return nil
	}

	usage := &model.Usage{
		PromptTokens:     r.InputTokens,
		CompletionTokens: r.OutputTokens,
		TotalTokens:      r.TotalTokens,
	}
	usage.PromptTokensDetails = r.InputTokensDetails.toModel()
	usage.CompletionTokensDetails = r.OutputTokensDetails.toModel()
	return usage
}

// FromModelUsage converts model.Usage to ResponseAPIUsage for compatibility.
func (r *ResponseAPIUsage) FromModelUsage(usage *model.Usage) *ResponseAPIUsage {
	if usage == nil {
		return nil
	}

	converted := &ResponseAPIUsage{
		InputTokens:         usage.PromptTokens,
		OutputTokens:        usage.CompletionTokens,
		TotalTokens:         usage.TotalTokens,
		InputTokensDetails:  newResponseAPIInputTokensDetailsFromModel(usage.PromptTokensDetails),
		OutputTokensDetails: newResponseAPIOutputTokensDetailsFromModel(usage.CompletionTokensDetails),
	}

	return converted
}

// ResponseAPIResponse represents the OpenAI Response API response structure
// https://platform.openai.com/docs/api-reference/responses
type ResponseAPIResponse struct {
	Id                 string                         `json:"id"`                             // Unique identifier for this Response
	Object             string                         `json:"object"`                         // The object type of this resource - always set to "response"
	CreatedAt          int64                          `json:"created_at"`                     // Unix timestamp (in seconds) of when this Response was created
	Status             string                         `json:"status"`                         // The status of the response generation
	Model              string                         `json:"model"`                          // Model ID used to generate the response
	Output             []OutputItem                   `json:"output"`                         // An array of content items generated by the model
	Usage              *ResponseAPIUsage              `json:"usage,omitempty"`                // Token usage details (Response API format)
	Instructions       *string                        `json:"instructions,omitempty"`         // System message as the first item in the model's context
	MaxOutputTokens    *int                           `json:"max_output_tokens,omitempty"`    // Upper bound for the number of tokens
	Metadata           any                            `json:"metadata,omitempty"`             // Set of 16 key-value pairs
	ParallelToolCalls  bool                           `json:"parallel_tool_calls"`            // Whether to allow the model to run tool calls in parallel
	PreviousResponseId *string                        `json:"previous_response_id,omitempty"` // The unique ID of the previous response
	Reasoning          *model.OpenAIResponseReasoning `json:"reasoning,omitempty"`            // Configuration options for reasoning models
	ServiceTier        *string                        `json:"service_tier,omitempty"`         // Latency tier used for processing
	Temperature        *float64                       `json:"temperature,omitempty"`          // Sampling temperature used
	Text               *ResponseTextConfig            `json:"text,omitempty"`                 // Configuration options for text response
	ToolChoice         any                            `json:"tool_choice,omitempty"`          // How the model selected tools
	Tools              []model.Tool                   `json:"tools,omitempty"`                // Array of tools the model may call
	RequiredAction     *ResponseAPIRequiredAction     `json:"required_action,omitempty"`      // Information about next actions required by the client
	TopP               *float64                       `json:"top_p,omitempty"`                // Alternative to sampling with temperature
	Truncation         *string                        `json:"truncation,omitempty"`           // Truncation strategy
	User               *string                        `json:"user,omitempty"`                 // Stable identifier for end-users
	Error              *model.Error                   `json:"error,omitempty"`                // Error object if the response failed
	IncompleteDetails  *IncompleteDetails             `json:"incomplete_details,omitempty"`   // Details about why the response is incomplete
}

// OutputItem represents an item in the response output array
type OutputItem struct {
	Type    string               `json:"type"`              // Type of output item (e.g., "message", "reasoning", "function_call", "mcp_list_tools", "mcp_call", "mcp_approval_request")
	Id      string               `json:"id,omitempty"`      // Unique identifier for this item
	Status  string               `json:"status,omitempty"`  // Status of this item (e.g., "completed")
	Role    string               `json:"role,omitempty"`    // Role of the message (e.g., "assistant")
	Content []OutputContent      `json:"content,omitempty"` // Array of content items
	Summary []OutputContent      `json:"summary,omitempty"` // Array of summary items (for reasoning)
	Action  *WebSearchCallAction `json:"action,omitempty"`  // Action details for web_search_call items

	// Function call fields
	CallId    string `json:"call_id,omitempty"`   // Call ID for function calls
	Name      string `json:"name,omitempty"`      // Function name for function calls
	Arguments string `json:"arguments,omitempty"` // Function arguments for function calls

	// MCP-specific fields
	ServerLabel       string       `json:"server_label,omitempty"`        // Label for the MCP server (for mcp_list_tools, mcp_call, mcp_approval_request)
	Tools             []model.Tool `json:"tools,omitempty"`               // Array of tools from MCP server (for mcp_list_tools)
	ApprovalRequestId *string      `json:"approval_request_id,omitempty"` // ID of approval request (for mcp_call)
	Error             *string      `json:"error,omitempty"`               // Error message if MCP call failed (for mcp_call)
	Output            string       `json:"output,omitempty"`              // Output from MCP tool call (for mcp_call)
}

// OutputContent represents content within an output item
type OutputContent struct {
	Type        string          `json:"type"`                  // Type of content (e.g., "output_text", "summary_text")
	Text        string          `json:"text,omitempty"`        // Text content
	JSON        json.RawMessage `json:"json,omitempty"`        // Structured JSON content for structured outputs
	Annotations []any           `json:"annotations,omitempty"` // Annotations for the content
}

// IncompleteDetails provides details about why a response is incomplete
type IncompleteDetails struct {
	Reason string `json:"reason,omitempty"` // Reason why the response is incomplete
}

// ConvertResponseAPIToChatCompletion converts a Response API response back to ChatCompletion format
// This function follows the same pattern as ResponseClaude2OpenAI in the anthropic adaptor
func ConvertResponseAPIToChatCompletion(responseAPIResp *ResponseAPIResponse) *TextResponse {
	var responseText string
	var reasoningText string
	tools := make([]model.Tool, 0)
	toolCallSeen := make(map[string]bool)

	// Extract content from output array
	for _, outputItem := range responseAPIResp.Output {
		switch outputItem.Type {
		case "message":
			if outputItem.Role == "assistant" {
				for _, content := range outputItem.Content {
					switch content.Type {
					case "output_text":
						responseText += content.Text
					case "output_json":
						if len(content.JSON) > 0 {
							responseText += string(content.JSON)
						} else if content.Text != "" {
							responseText += content.Text
						}
					case "reasoning":
						reasoningText += content.Text
					default:
						// Handle other content types if needed
					}
				}
			}
		case "reasoning":
			// Handle reasoning items separately
			for _, summaryContent := range outputItem.Summary {
				if summaryContent.Type == "summary_text" {
					reasoningText += summaryContent.Text
				}
			}
		case "function_call":
			// Handle function call items
			if outputItem.CallId != "" && outputItem.Name != "" {
				tool := model.Tool{
					Id:   outputItem.CallId,
					Type: "function",
					Function: &model.Function{
						Name:      outputItem.Name,
						Arguments: outputItem.Arguments,
					},
				}
				tools = append(tools, tool)
				toolCallSeen[outputItem.CallId] = true
			}
		case "mcp_list_tools":
			// Handle MCP list tools output - add server tools information to response text
			if outputItem.ServerLabel != "" && len(outputItem.Tools) > 0 {
				responseText += fmt.Sprintf("\nMCP Server '%s' tools imported: %d tools available",
					outputItem.ServerLabel, len(outputItem.Tools))
			}
		case "mcp_call":
			// Handle MCP tool call output - add call result to response text
			if outputItem.Name != "" && outputItem.Output != "" {
				responseText += fmt.Sprintf("\nMCP Tool '%s' result: %s", outputItem.Name, outputItem.Output)
			} else if outputItem.Error != nil && *outputItem.Error != "" {
				responseText += fmt.Sprintf("\nMCP Tool '%s' error: %s", outputItem.Name, *outputItem.Error)
			}
		case "mcp_approval_request":
			// Handle MCP approval request - add approval request info to response text
			if outputItem.ServerLabel != "" && outputItem.Name != "" {
				responseText += fmt.Sprintf("\nMCP Approval Required: Server '%s' requests approval to call '%s'",
					outputItem.ServerLabel, outputItem.Name)
			}
		}
	}

	// Handle reasoning content from reasoning field if present
	if responseAPIResp.Reasoning != nil {
		// Reasoning content would be handled here if needed
	}

	// Include pending tool calls that require client action
	if responseAPIResp.RequiredAction != nil && responseAPIResp.RequiredAction.SubmitToolOutputs != nil {
		for _, call := range responseAPIResp.RequiredAction.SubmitToolOutputs.ToolCalls {
			if call.Function == nil {
				continue
			}

			toolID := call.Id
			if toolID == "" {
				toolID = call.Function.Name
			}
			if toolID != "" && toolCallSeen[toolID] {
				continue
			}

			tool := model.Tool{
				Id:   call.Id,
				Type: strings.ToLower(strings.TrimSpace(call.Type)),
				Function: sanitizeDecodedFunction(&model.Function{
					Name:      call.Function.Name,
					Arguments: call.Function.Arguments,
				}),
			}
			if tool.Type == "" {
				tool.Type = "function"
			}
			tools = append(tools, tool)
			if toolID != "" {
				toolCallSeen[toolID] = true
			}
		}
	}

	// Convert status to finish reason
	finishReason := "stop"
	switch responseAPIResp.Status {
	case "completed":
		finishReason = "stop"
	case "failed":
		finishReason = "stop"
	case "incomplete":
		finishReason = "length"
	case "cancelled":
		finishReason = "stop"
	default:
		finishReason = "stop"
	}

	if len(tools) > 0 {
		finishReason = "tool_calls"
	}

	choice := TextResponseChoice{
		Index: 0,
		Message: model.Message{
			Role:      "assistant",
			Content:   responseText,
			Name:      nil,
			ToolCalls: tools,
		},
		FinishReason: finishReason,
	}

	if reasoningText != "" {
		choice.Message.Reasoning = &reasoningText
	}

	// Create the chat completion response
	fullTextResponse := TextResponse{
		Id:      responseAPIResp.Id,
		Model:   responseAPIResp.Model,
		Object:  "chat.completion",
		Created: responseAPIResp.CreatedAt,
		Choices: []TextResponseChoice{choice},
	}

	// Set usage if available and valid - convert Response API usage fields to Chat Completion format
	if responseAPIResp.Usage != nil {
		if convertedUsage := responseAPIResp.Usage.ToModelUsage(); convertedUsage != nil {
			// Only set usage if it contains meaningful data
			if convertedUsage.PromptTokens > 0 || convertedUsage.CompletionTokens > 0 || convertedUsage.TotalTokens > 0 {
				fullTextResponse.Usage = *convertedUsage
			}
		}
	}
	// Note: If usage is nil or contains no meaningful data, the caller should calculate tokens

	return &fullTextResponse
}

// ConvertResponseAPIStreamToChatCompletion converts a Response API streaming response chunk back to ChatCompletion streaming format
// This function handles individual streaming chunks from the Response API
func ConvertResponseAPIStreamToChatCompletion(responseAPIChunk *ResponseAPIResponse) *ChatCompletionsStreamResponse {
	return ConvertResponseAPIStreamToChatCompletionWithIndex(responseAPIChunk, nil)
}

// ConvertResponseAPIStreamToChatCompletionWithIndex converts a Response API streaming response chunk back to ChatCompletion streaming format
// with optional output_index from streaming events for proper tool call index assignment
func ConvertResponseAPIStreamToChatCompletionWithIndex(responseAPIChunk *ResponseAPIResponse, outputIndex *int) *ChatCompletionsStreamResponse {
	var deltaContent string
	var reasoningText string
	var finishReason *string
	var toolCalls []model.Tool
	toolCallSeen := make(map[string]bool)

	// Extract content from output array
	for _, outputItem := range responseAPIChunk.Output {
		switch outputItem.Type {
		case "message":
			if outputItem.Role == "assistant" {
				for _, content := range outputItem.Content {
					switch content.Type {
					case "output_text":
						deltaContent += content.Text
					case "output_json":
						if len(content.JSON) > 0 {
							deltaContent += string(content.JSON)
						} else if content.Text != "" {
							deltaContent += content.Text
						}
					case "reasoning":
						reasoningText += content.Text
					default:
						// Handle other content types if needed
					}
				}
			}
		case "reasoning":
			// Handle reasoning items separately - extract from summary content
			for _, summaryContent := range outputItem.Summary {
				if summaryContent.Type == "summary_text" {
					reasoningText += summaryContent.Text
				}
			}
		case "function_call":
			// Handle function call items
			if outputItem.CallId != "" && outputItem.Name != "" {
				// Set index for streaming tool calls
				// Use the provided outputIndex from streaming events if available, otherwise use position in slice
				var index int
				if outputIndex != nil {
					index = *outputIndex
				} else {
					index = len(toolCalls)
				}
				tool := model.Tool{
					Id:   outputItem.CallId,
					Type: "function",
					Function: &model.Function{
						Name:      outputItem.Name,
						Arguments: outputItem.Arguments,
					},
					Index: &index, // Set index for streaming delta accumulation
				}
				toolCalls = append(toolCalls, tool)
				toolCallSeen[outputItem.CallId] = true
			}
		// Note: This is currently unavailable in the OpenAI Docs.
		// It's added here for reference because OpenAI's Remote MCP is included in their tools, unlike other Remote MCPs such as Anthropic Claude.
		case "mcp_list_tools":
			// Handle MCP list tools output in streaming - add server tools information as delta content
			if outputItem.ServerLabel != "" && len(outputItem.Tools) > 0 {
				deltaContent += fmt.Sprintf("\nMCP Server '%s' tools imported: %d tools available",
					outputItem.ServerLabel, len(outputItem.Tools))
			}
		case "mcp_call":
			// Handle MCP tool call output in streaming - add call result as delta content
			if outputItem.Name != "" && outputItem.Output != "" {
				deltaContent += fmt.Sprintf("\nMCP Tool '%s' result: %s", outputItem.Name, outputItem.Output)
			} else if outputItem.Error != nil && *outputItem.Error != "" {
				deltaContent += fmt.Sprintf("\nMCP Tool '%s' error: %s", outputItem.Name, *outputItem.Error)
			}
		case "mcp_approval_request":
			// Handle MCP approval request in streaming - add approval request info as delta content
			if outputItem.ServerLabel != "" && outputItem.Name != "" {
				deltaContent += fmt.Sprintf("\nMCP Approval Required: Server '%s' requests approval to call '%s'",
					outputItem.ServerLabel, outputItem.Name)
			}
		}
	}

	// Include required_action tool calls in the streaming delta when present
	if responseAPIChunk.RequiredAction != nil && responseAPIChunk.RequiredAction.SubmitToolOutputs != nil {
		for _, call := range responseAPIChunk.RequiredAction.SubmitToolOutputs.ToolCalls {
			if call.Function == nil {
				continue
			}
			identifier := call.Id
			if identifier == "" {
				identifier = call.Function.Name
			}
			if identifier != "" && toolCallSeen[identifier] {
				continue
			}

			idx := len(toolCalls)
			tool := model.Tool{
				Id:   call.Id,
				Type: strings.ToLower(strings.TrimSpace(call.Type)),
				Function: sanitizeDecodedFunction(&model.Function{
					Name:      call.Function.Name,
					Arguments: call.Function.Arguments,
				}),
				Index: &idx,
			}
			if tool.Type == "" {
				tool.Type = "function"
			}
			toolCalls = append(toolCalls, tool)
			if identifier != "" {
				toolCallSeen[identifier] = true
			}
		}
	}

	// Convert status to finish reason for final chunks
	switch responseAPIChunk.Status {
	case "completed":
		reason := "stop"
		finishReason = &reason
	case "failed":
		reason := "stop"
		finishReason = &reason
	case "incomplete":
		reason := "length"
		finishReason = &reason
	}

	// Create the streaming choice
	choice := ChatCompletionsStreamResponseChoice{
		Index: 0,
		Delta: model.Message{
			Role:    "assistant",
			Content: deltaContent,
		},
		FinishReason: finishReason,
	}

	// Set tool calls if present
	if len(toolCalls) > 0 {
		choice.Delta.ToolCalls = toolCalls
	}

	// Set reasoning content if present
	if reasoningText != "" {
		choice.Delta.Reasoning = &reasoningText
	}

	// Create the streaming response
	streamResponse := ChatCompletionsStreamResponse{
		Id:      responseAPIChunk.Id,
		Object:  "chat.completion.chunk",
		Created: responseAPIChunk.CreatedAt,
		Model:   responseAPIChunk.Model,
		Choices: []ChatCompletionsStreamResponseChoice{choice},
	}

	// Add usage if available (typically only in the final chunk)
	if responseAPIChunk.Usage != nil {
		streamResponse.Usage = responseAPIChunk.Usage.ToModelUsage()
	}

	return &streamResponse
}

// ResponseAPIStreamEvent represents a flexible structure for Response API streaming events
// This handles different event types that have varying schemas
type ResponseAPIStreamEvent struct {
	// Common fields for all events
	Type           string `json:"type,omitempty"`            // Event type (e.g., "response.output_text.done")
	SequenceNumber int    `json:"sequence_number,omitempty"` // Sequence number for ordering

	// Response-level events (type starts with "response.")
	Response *ResponseAPIResponse `json:"response,omitempty"` // Full response object for response-level events
	// Required action events (response.required_action.*)
	RequiredAction *ResponseAPIRequiredAction `json:"required_action,omitempty"`

	// Output item events (type contains "output_item")
	OutputIndex int         `json:"output_index,omitempty"` // Index of the output item
	Item        *OutputItem `json:"item,omitempty"`         // Output item for item-level events

	// Content events (type contains "content" or "output_text")
	ItemId       string          `json:"item_id,omitempty"`       // ID of the item containing the content
	ContentIndex int             `json:"content_index,omitempty"` // Index of the content within the item
	Part         *OutputContent  `json:"part,omitempty"`          // Content part for part-level events
	Delta        json.RawMessage `json:"delta,omitempty"`         // Delta payload (string or object)
	Text         string          `json:"text,omitempty"`          // Full text content (for done events)

	// Function call events (type contains "function_call")
	Arguments string          `json:"arguments,omitempty"` // Complete function arguments (for done events)
	Output    json.RawMessage `json:"output,omitempty"`    // Output payload for structured data events
	JSON      json.RawMessage `json:"json,omitempty"`      // Direct JSON payload field when provided

	// General fields that might be in any event
	Id     string       `json:"id,omitempty"`     // Event ID
	Status string       `json:"status,omitempty"` // Event status
	Usage  *model.Usage `json:"usage,omitempty"`  // Usage information
}

// ParseResponseAPIStreamEvent attempts to parse a streaming event as either a full response
// or a streaming event, returning the appropriate data structure
func ParseResponseAPIStreamEvent(data []byte) (*ResponseAPIResponse, *ResponseAPIStreamEvent, error) {
	// First try to parse as a full ResponseAPIResponse (for response-level events)
	var fullResponse ResponseAPIResponse
	if err := json.Unmarshal(data, &fullResponse); err == nil && fullResponse.Id != "" {
		return &fullResponse, nil, nil
	}

	// If that fails, try to parse as a streaming event
	var streamEvent ResponseAPIStreamEvent
	if err := json.Unmarshal(data, &streamEvent); err != nil {
		return nil, nil, errors.Wrap(err, "ParseResponseAPIStreamEvent: failed to unmarshal as stream event")
	}

	return nil, &streamEvent, nil
}

// ConvertStreamEventToResponse converts a streaming event to a ResponseAPIResponse structure
// This allows us to use the existing conversion logic for different event types
func ConvertStreamEventToResponse(event *ResponseAPIStreamEvent) ResponseAPIResponse {
	// Convert model.Usage to ResponseAPIUsage if present
	var responseUsage *ResponseAPIUsage
	if event.Usage != nil {
		responseUsage = (&ResponseAPIUsage{}).FromModelUsage(event.Usage)
	}

	response := ResponseAPIResponse{
		Id:        event.Id,
		Object:    "response",
		Status:    "in_progress", // Default status for streaming events
		Usage:     responseUsage,
		CreatedAt: 0, // Will be filled by the conversion logic if needed
	}

	// If the event already has a specific status, use it
	if event.Status != "" {
		response.Status = event.Status
	}

	// Handle different event types
	switch {
	case event.Response != nil:
		// Handle events that contain a full response object (response.created, response.completed, etc.)
		return *event.Response

	case strings.HasPrefix(event.Type, "response.reasoning_summary_text.delta"):
		// Handle reasoning summary text delta events
		if delta := extractStringFromRaw(event.Delta, "text", "delta"); delta != "" {
			outputItem := OutputItem{
				Type: "reasoning",
				Summary: []OutputContent{
					{
						Type: "summary_text",
						Text: delta,
					},
				},
			}
			response.Output = []OutputItem{outputItem}
		}

	case strings.HasPrefix(event.Type, "response.reasoning_summary_text.done"):
		// Handle reasoning summary text completion events
		if event.Text != "" {
			outputItem := OutputItem{
				Type: "reasoning",
				Summary: []OutputContent{
					{
						Type: "summary_text",
						Text: event.Text,
					},
				},
			}
			response.Output = []OutputItem{outputItem}
		}

	case strings.HasPrefix(event.Type, "response.output_text.delta"):
		// Handle text delta events
		if delta := extractStringFromRaw(event.Delta, "text", "delta"); delta != "" {
			outputItem := OutputItem{
				Type: "message",
				Role: "assistant",
				Content: []OutputContent{
					{
						Type: "output_text",
						Text: delta,
					},
				},
			}
			response.Output = []OutputItem{outputItem}
		}

	case strings.HasPrefix(event.Type, "response.output_json.delta"):
		// Handle structured JSON delta events
		if partial := extractStringFromRaw(event.Delta, "partial_json", "json", "text"); partial != "" {
			outputItem := OutputItem{
				Type: "message",
				Role: "assistant",
				Content: []OutputContent{
					{
						Type: "output_json",
						Text: partial,
					},
				},
			}
			response.Output = []OutputItem{outputItem}
		}

	case strings.HasPrefix(event.Type, "response.output_text.done"):
		// Handle text completion events
		if event.Text != "" {
			outputItem := OutputItem{
				Type: "message",
				Role: "assistant",
				Content: []OutputContent{
					{
						Type: "output_text",
						Text: event.Text,
					},
				},
			}
			response.Output = []OutputItem{outputItem}
		}

	case strings.HasPrefix(event.Type, "response.output_json.done"):
		// Handle structured JSON completion events
		if jsonPayload := extractJSONPayloadFromEvent(event); len(jsonPayload) > 0 {
			outputItem := OutputItem{
				Type: "message",
				Role: "assistant",
				Content: []OutputContent{
					{
						Type: "output_json",
						JSON: jsonPayload,
					},
				},
			}
			response.Output = []OutputItem{outputItem}
			if response.Status == "in_progress" {
				response.Status = "completed"
			}
		}

	case strings.HasPrefix(event.Type, "response.output_item"):
		// Handle output item events (added, done)
		if event.Item != nil {
			response.Output = []OutputItem{*event.Item}
		}

	case strings.HasPrefix(event.Type, "response.function_call_arguments.delta"):
		// Handle function call arguments delta events
		if delta := extractStringFromRaw(event.Delta, "partial_json", "text", "arguments", "delta"); delta != "" {
			outputItem := OutputItem{
				Type:      "function_call",
				Arguments: delta, // This is a delta, not complete arguments
			}
			response.Output = []OutputItem{outputItem}
		}

	case strings.HasPrefix(event.Type, "response.function_call_arguments.done"):
		// Handle function call arguments completion events
		if event.Arguments != "" {
			outputItem := OutputItem{
				Type:      "function_call",
				Arguments: event.Arguments, // Complete arguments
			}
			response.Output = []OutputItem{outputItem}
		}

	case strings.HasPrefix(event.Type, "response.content_part"):
		// Handle content part events (added, done)
		if event.Part != nil {
			outputItem := OutputItem{
				Type:    "message",
				Role:    "assistant",
				Content: []OutputContent{*event.Part},
			}
			response.Output = []OutputItem{outputItem}
		}

	case strings.HasPrefix(event.Type, "response.reasoning_summary_part"):
		// Handle reasoning summary part events (added, done)
		if event.Part != nil {
			outputItem := OutputItem{
				Type:    "reasoning",
				Summary: []OutputContent{*event.Part},
			}
			response.Output = []OutputItem{outputItem}
		}

	case strings.HasPrefix(event.Type, "response."):
		// Handle other response-level events (in_progress, etc.)
		// These typically don't have content but may have metadata
		// The response structure is already set up above with basic fields

	default:
		// Unknown event type - log but don't fail
		// The response structure is already set up above with basic fields
	}

	return response
}

func extractStringFromRaw(raw json.RawMessage, keys ...string) string {
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

func extractJSONPayloadFromEvent(event *ResponseAPIStreamEvent) json.RawMessage {
	if event == nil {
		return nil
	}
	if len(event.JSON) > 0 {
		return cloneRawMessage(event.JSON)
	}
	if event.Part != nil {
		if len(event.Part.JSON) > 0 {
			return cloneRawMessage(event.Part.JSON)
		}
		if event.Part.Text != "" {
			return normalizeJSONRaw(event.Part.Text)
		}
	}
	if len(event.Output) > 0 {
		if payload := decodeJSONBlob(event.Output); len(payload) > 0 {
			return payload
		}
	}
	if event.Text != "" {
		return normalizeJSONRaw(event.Text)
	}
	if len(event.Delta) > 0 {
		if partial := extractStringFromRaw(event.Delta, "json", "partial_json", "text"); partial != "" {
			return normalizeJSONRaw(partial)
		}
	}
	return nil
}

func decodeJSONBlob(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var node any
	if err := json.Unmarshal(raw, &node); err != nil {
		return cloneRawMessage(raw)
	}
	if payload := extractJSONValue(node); len(payload) > 0 {
		return payload
	}
	return cloneRawMessage(raw)
}

func extractJSONValue(node any) json.RawMessage {
	switch v := node.(type) {
	case map[string]any:
		if val, ok := v["json"]; ok {
			if payload := extractJSONValue(val); len(payload) > 0 {
				return payload
			}
		}
		if val, ok := v["text"]; ok {
			if payload := extractJSONValue(val); len(payload) > 0 {
				return payload
			}
		}
		if val, ok := v["content"]; ok {
			if payload := extractJSONValue(val); len(payload) > 0 {
				return payload
			}
		}
		if b, err := json.Marshal(v); err == nil {
			return b
		}
	case []any:
		for _, child := range v {
			if payload := extractJSONValue(child); len(payload) > 0 {
				return payload
			}
		}
		if b, err := json.Marshal(v); err == nil {
			return b
		}
	case string:
		return normalizeJSONRaw(v)
	case json.RawMessage:
		return cloneRawMessage(v)
	}
	return nil
}

func normalizeJSONRaw(text string) json.RawMessage {
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
	cloned := make([]byte, len(bytes))
	copy(cloned, bytes)
	return json.RawMessage(cloned)
}

func cloneRawMessage(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	cloned := make([]byte, len(raw))
	copy(cloned, raw)
	return json.RawMessage(cloned)
}
