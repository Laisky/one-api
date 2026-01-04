package openai

import (
	"fmt"
	"strings"

	"github.com/songquanpeng/one-api/relay/model"
)

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
