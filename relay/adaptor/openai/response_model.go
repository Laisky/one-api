package openai

import (
	"fmt"
	"maps"
	"strings"

	"github.com/songquanpeng/one-api/relay/model"
)

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
			reasoningSummary := DefaultResponseReasoningSummaryForModel(request.Model)
			responseReq.Reasoning.Summary = &reasoningSummary
		}
		// Ensure the default (or user-provided) summary is compatible with the target model.
		_ = NormalizeResponseReasoningSummaryForModel(request.Model, responseReq.Reasoning)
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
			paramsMap := map[string]any{}
			if fn.Parameters != nil {
				sanitized := sanitizeResponseAPIFunctionParameters(fn.Parameters)
				if parsed, ok := sanitized.(map[string]any); ok && parsed != nil {
					paramsMap = parsed
				}
			}

			if _, ok := paramsMap["type"]; !ok {
				paramsMap["type"] = "object"
			} else if typeStr, ok := paramsMap["type"].(string); !ok || strings.TrimSpace(typeStr) == "" {
				paramsMap["type"] = "object"
			}
			fn.Parameters = paramsMap
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
