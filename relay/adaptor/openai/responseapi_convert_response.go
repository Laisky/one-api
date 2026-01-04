package openai

import (
	"fmt"
	"strings"

	"github.com/songquanpeng/one-api/relay/model"
)

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
