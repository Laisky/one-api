package controller

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/adaptor/openai_compatible"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

// renderChatResponseAsResponseAPI renders a Chat Completion response as a Response API response
func renderChatResponseAsResponseAPI(c *gin.Context, status int, textResp *openai_compatible.SlimTextResponse, originalReq *openai.ResponseAPIRequest, meta *metalib.Meta) error {
	c.Set(ctxkey.ResponseRewriteApplied, true)
	responseID := generateResponseAPIID(c, textResp)
	statusText, incomplete := deriveResponseStatus(textResp.Choices)
	usage := (&openai.ResponseAPIUsage{}).FromModelUsage(&textResp.Usage)
	output := buildResponseOutput(textResp.Choices)
	toolCalls := buildRequiredActionToolCalls(textResp.Choices)

	response := openai.ResponseAPIResponse{
		Id:                 responseID,
		Object:             "response",
		CreatedAt:          time.Now().Unix(),
		Status:             statusText,
		Model:              meta.ActualModelName,
		Output:             output,
		Usage:              usage,
		Instructions:       originalReq.Instructions,
		MaxOutputTokens:    originalReq.MaxOutputTokens,
		Metadata:           originalReq.Metadata,
		ParallelToolCalls:  originalReq.ParallelToolCalls != nil && *originalReq.ParallelToolCalls,
		PreviousResponseId: originalReq.PreviousResponseId,
		Reasoning:          originalReq.Reasoning,
		ServiceTier:        originalReq.ServiceTier,
		Temperature:        originalReq.Temperature,
		Text:               originalReq.Text,
		ToolChoice:         originalReq.ToolChoice,
		Tools:              convertResponseAPITools(originalReq.Tools),
		TopP:               originalReq.TopP,
		Truncation:         originalReq.Truncation,
		User:               originalReq.User,
	}

	if len(toolCalls) > 0 {
		response.RequiredAction = &openai.ResponseAPIRequiredAction{
			Type: "submit_tool_outputs",
			SubmitToolOutputs: &openai.ResponseAPISubmitToolOutputs{
				ToolCalls: toolCalls,
			},
		}
	}

	if incomplete != nil {
		response.IncompleteDetails = incomplete
	}

	data, err := json.Marshal(response)
	if err != nil {
		return err
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(status)
	_, err = c.Writer.Write(data)
	return err
}

// generateResponseAPIID generates a unique ID for a Response API response
func generateResponseAPIID(c *gin.Context, _ *openai_compatible.SlimTextResponse) string {
	if reqID := c.GetString(ctxkey.RequestId); reqID != "" {
		return fmt.Sprintf("resp-%s", reqID)
	}
	return fmt.Sprintf("resp-%d", time.Now().UnixNano())
}

// deriveResponseStatus derives the status of a Response API response from Chat Completion choices
func deriveResponseStatus(choices []openai_compatible.TextResponseChoice) (string, *openai.IncompleteDetails) {
	status := "completed"
	for _, choice := range choices {
		switch choice.FinishReason {
		case "length":
			return "incomplete", &openai.IncompleteDetails{Reason: "max_output_tokens"}
		case "content_filter":
			return "incomplete", &openai.IncompleteDetails{Reason: "content_filter"}
		case "cancelled":
			status = "cancelled"
		}
	}
	return status, nil
}

// buildResponseOutput builds the output items for a Response API response from Chat Completion choices
func buildResponseOutput(choices []openai_compatible.TextResponseChoice) []openai.OutputItem {
	var output []openai.OutputItem
	for _, choice := range choices {
		msg := choice.Message
		contents := convertMessageContent(msg)
		if len(contents) > 0 {
			output = append(output, openai.OutputItem{
				Type:    "message",
				Role:    "assistant",
				Status:  "completed",
				Content: contents,
			})
		}

		if reasoning := extractReasoning(msg); reasoning != "" {
			output = append(output, openai.OutputItem{
				Type:   "reasoning",
				Status: "completed",
				Summary: []openai.OutputContent{
					{Type: "summary_text", Text: reasoning},
				},
			})
		}

		for _, tool := range msg.ToolCalls {
			arguments := ""
			if tool.Function != nil && tool.Function.Arguments != nil {
				switch v := tool.Function.Arguments.(type) {
				case string:
					arguments = v
				default:
					if b, err := json.Marshal(v); err == nil {
						arguments = string(b)
					}
				}
			}
			output = append(output, openai.OutputItem{
				Type:   "function_call",
				Status: "completed",
				CallId: tool.Id,
				Name: func() string {
					if tool.Function != nil {
						return tool.Function.Name
					}
					return ""
				}(),
				Arguments: arguments,
			})
		}
	}
	return output
}

// buildRequiredActionToolCalls builds the required action tool calls for a Response API response from Chat Completion choices
func buildRequiredActionToolCalls(choices []openai_compatible.TextResponseChoice) []openai.ResponseAPIToolCall {
	toolCalls := make([]openai.ResponseAPIToolCall, 0)
	for _, choice := range choices {
		for _, tool := range choice.Message.ToolCalls {
			if tool.Function == nil {
				continue
			}
			callID := ensureResponseAPICallID(tool.Id)
			if callID == "" {
				continue
			}
			arguments := ""
			if tool.Function.Arguments != nil {
				switch v := tool.Function.Arguments.(type) {
				case string:
					arguments = v
				default:
					if b, err := json.Marshal(v); err == nil {
						arguments = string(b)
					}
				}
			}
			toolCalls = append(toolCalls, openai.ResponseAPIToolCall{
				Id:   callID,
				Type: "function",
				Function: &openai.ResponseAPIFunctionCall{
					Name:      tool.Function.Name,
					Arguments: arguments,
				},
			})
		}
	}
	return toolCalls
}

// ensureResponseAPICallID ensures that a tool call ID is in the format expected by the Response API
func ensureResponseAPICallID(originalID string) string {
	trimmed := strings.TrimSpace(originalID)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "call_") {
		return trimmed
	}
	if strings.HasPrefix(trimmed, "fc_") {
		return strings.Replace(trimmed, "fc_", "call_", 1)
	}
	return "call_" + trimmed
}

// convertMessageContent converts a Chat Completion message content to Response API output content
func convertMessageContent(msg relaymodel.Message) []openai.OutputContent {
	var contents []openai.OutputContent
	if msg.IsStringContent() {
		if text := strings.TrimSpace(msg.StringContent()); text != "" {
			contents = append(contents, openai.OutputContent{Type: "output_text", Text: text})
		}
		return contents
	}

	for _, part := range msg.ParseContent() {
		switch part.Type {
		case relaymodel.ContentTypeText:
			if part.Text != nil && *part.Text != "" {
				contents = append(contents, openai.OutputContent{Type: "output_text", Text: *part.Text})
			}
		case relaymodel.ContentTypeImageURL:
			if part.ImageURL != nil && part.ImageURL.Url != "" {
				contents = append(contents, openai.OutputContent{Type: "output_text", Text: part.ImageURL.Url})
			}
		case relaymodel.ContentTypeInputAudio:
			if part.InputAudio != nil && part.InputAudio.Data != "" {
				contents = append(contents, openai.OutputContent{Type: "output_text", Text: part.InputAudio.Data})
			}
		}
	}
	return contents
}

// extractReasoning extracts reasoning content from a Chat Completion message
func extractReasoning(msg relaymodel.Message) string {
	if msg.Reasoning != nil {
		return *msg.Reasoning
	}
	if msg.ReasoningContent != nil {
		return *msg.ReasoningContent
	}
	if msg.Thinking != nil {
		return *msg.Thinking
	}
	return ""
}

// convertResponseAPITools converts Response API tools to Chat Completion tools
func convertResponseAPITools(tools []openai.ResponseAPITool) []relaymodel.Tool {
	if len(tools) == 0 {
		return nil
	}
	converted := make([]relaymodel.Tool, 0, len(tools))
	for _, tool := range tools {
		switch strings.ToLower(tool.Type) {
		case "function":
			fn := tool.Function
			if fn == nil {
				fn = &relaymodel.Function{
					Name:        tool.Name,
					Description: tool.Description,
					Parameters:  tool.Parameters,
				}
			}
			converted = append(converted, relaymodel.Tool{
				Type: "function",
				Function: &relaymodel.Function{
					Name:        fn.Name,
					Description: fn.Description,
					Parameters:  fn.Parameters,
					Required:    fn.Required,
					Strict:      fn.Strict,
				},
			})
		case "web_search":
			converted = append(converted, relaymodel.Tool{
				Type:              "web_search",
				SearchContextSize: tool.SearchContextSize,
				Filters:           tool.Filters,
				UserLocation:      tool.UserLocation,
			})
		case "mcp":
			converted = append(converted, relaymodel.Tool{
				Type:            "mcp",
				ServerLabel:     tool.ServerLabel,
				ServerUrl:       tool.ServerUrl,
				RequireApproval: tool.RequireApproval,
				AllowedTools:    tool.AllowedTools,
				Headers:         tool.Headers,
			})
		default:
			converted = append(converted, relaymodel.Tool{Type: tool.Type})
		}
	}
	return converted
}
