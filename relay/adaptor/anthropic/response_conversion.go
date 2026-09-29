package anthropic

import (
	"encoding/json"
	"fmt"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/tracing"
	"github.com/Laisky/one-api/relay/adaptor/common/toolnamesafe"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"time"
)

// stopReasonClaude2OpenAI translates an Anthropic termination reason without losing unknown reasons.
func stopReasonClaude2OpenAI(reason *string) string {
	if reason == nil {
		return ""
	}
	switch *reason {
	case "end_turn":
		return "stop"
	case "stop_sequence":
		return "stop"
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	default:
		return *reason
	}
}

// StreamResponseClaude2OpenAI converts one Claude event and any receipt to Chat metadata.
// Incremental events have no finish reason; real stop reasons retain their mapping.
func StreamResponseClaude2OpenAI(c *gin.Context, claudeResponse *StreamResponse) (*openai.ChatCompletionsStreamResponse, *Response) {
	logger := gmw.GetLogger(c)

	var response *Response
	var responseText string
	var reasoningText string
	var signatureText string
	var stopReason string
	tools := make([]model.Tool, 0)

	switch claudeResponse.Type {
	case "message_start":
		return nil, claudeResponse.Message
	case "content_block_start":
		if claudeResponse.ContentBlock != nil {
			responseText = claudeResponse.ContentBlock.Text
			if claudeResponse.ContentBlock.Thinking != nil {
				reasoningText = *claudeResponse.ContentBlock.Thinking
			}
			if claudeResponse.ContentBlock.Signature != nil {
				signatureText = *claudeResponse.ContentBlock.Signature
			}

			if claudeResponse.ContentBlock.Type == "tool_use" {
				// Set index for streaming tool calls - use the current index in the tools slice
				index := len(tools)
				// Restore any tool name that was sanitized in the request path so the
				// client sees the original identifier it submitted.
				toolName := toolnamesafe.RestoreToolName(c, claudeResponse.ContentBlock.Name)
				tools = append(tools, model.Tool{
					Id:   claudeResponse.ContentBlock.Id,
					Type: "function",
					Function: &model.Function{
						Name:      toolName,
						Arguments: "",
					},
					Index: &index, // Set index for streaming delta accumulation
				})
			}
		}
	case "content_block_delta":
		if claudeResponse.Delta != nil {
			responseText = claudeResponse.Delta.Text
			if claudeResponse.Delta.Thinking != nil {
				reasoningText = *claudeResponse.Delta.Thinking
			}
			if claudeResponse.Delta.Type == "signature_delta" && claudeResponse.Delta.Signature != nil {
				signatureText = *claudeResponse.Delta.Signature
			}

			if claudeResponse.Delta.Type == "input_json_delta" {
				// For input_json_delta, we should update the last tool call's arguments, not create a new one
				// The index should match the last tool call that was started in content_block_start
				if len(tools) > 0 {
					// Update the last tool call's arguments (this is a delta for the existing tool call)
					lastIndex := len(tools) - 1
					lastTool := tools[lastIndex]
					if existingArgs, ok := lastTool.Function.Arguments.(string); ok {
						lastTool.Function.Arguments = existingArgs + claudeResponse.Delta.PartialJson
					} else {
						lastTool.Function.Arguments = claudeResponse.Delta.PartialJson
					}
					// Keep the same index as the original tool call
					tools[lastIndex] = lastTool
				} else {
					// Fallback: create new tool call if no existing tool call found
					index := 0
					tools = append(tools, model.Tool{
						Function: &model.Function{
							Arguments: claudeResponse.Delta.PartialJson,
						},
						Index: &index, // Set index for streaming delta accumulation
					})
				}
			}
		}
	case "message_delta":
		if claudeResponse.Usage != nil {
			response = &Response{
				Usage: *claudeResponse.Usage,
			}
		}
		if claudeResponse.Delta != nil && claudeResponse.Delta.StopReason != nil {
			stopReason = *claudeResponse.Delta.StopReason
		}
	case "thinking_delta":
		if claudeResponse.Delta != nil && claudeResponse.Delta.Thinking != nil {
			reasoningText = *claudeResponse.Delta.Thinking
		}
	case "signature_delta":
		if claudeResponse.Delta != nil && claudeResponse.Delta.Signature != nil {
			signatureText = *claudeResponse.Delta.Signature
		}
	case "ping",
		"message_stop",
		"content_block_stop":
	case "error":
		// handled by caller (StreamHandler)
	default:
		logger.Error("unknown stream response type", zap.String("type", claudeResponse.Type))
	}

	// Cache signature if present (for thinking blocks)
	if signatureText != "" && (reasoningText != "" || claudeResponse.Type == "signature_delta") {
		// Get token ID from context
		if tokenID, exists := c.Get(ctxkey.TokenId); exists {
			if tokenIDInt, ok := tokenID.(int); ok {
				// We need the original request to generate conversation ID
				// For now, we'll cache with a temporary key and update it later
				// This will be properly handled in the request conversion phase
				tokenIDStr := getTokenIDFromRequest(tokenIDInt)
				tempKey := fmt.Sprintf("temp_sig:%s:%d", tokenIDStr, time.Now().UnixNano())
				GetSignatureCache().Store(tempKey, signatureText)

				// Store the temp key in context for later use
				c.Set(ctxkey.TempSignatureKey, tempKey)
			}
		}
	}

	var choice openai.ChatCompletionsStreamResponseChoice
	choice.Delta.Content = responseText
	if reasoningText != "" {
		choice.Delta.SetReasoningContent(c.Query("reasoning_format"), reasoningText)
	}
	if len(tools) > 0 {
		choice.Delta.Content = nil // compatible with other OpenAI derivative applications, like LobeOpenAICompatibleFactory ...
		choice.Delta.ToolCalls = tools
	}
	choice.Delta.Role = "assistant"
	finishReason := stopReasonClaude2OpenAI(&stopReason)
	if finishReason != "" && finishReason != "null" {
		choice.FinishReason = &finishReason
	}
	var openaiResponse openai.ChatCompletionsStreamResponse
	openaiResponse.Object = "chat.completion.chunk"
	openaiResponse.Choices = []openai.ChatCompletionsStreamResponseChoice{choice}
	return &openaiResponse, response
}

func ResponseClaude2OpenAI(c *gin.Context, claudeResponse *Response) *openai.TextResponse {
	logger := gmw.GetLogger(c)
	var responseText string
	var reasoningText string

	tools := make([]model.Tool, 0)
	for i, v := range claudeResponse.Content {
		switch v.Type {
		case "thinking", "redacted_thinking":
			if v.Thinking != nil {
				reasoningText += *v.Thinking
			} else {
				logger.Error("thinking is nil in response")
			}
			// Cache signature if present
			if v.Signature != nil {
				// Cache the signature for future use
				if tokenID, exists := c.Get(ctxkey.TokenId); exists {
					if tokenIDInt, ok := tokenID.(int); ok {
						// Get conversation ID from request context or generate it
						var conversationID string
						if convID, exists := c.Get(ctxkey.ConversationId); exists {
							conversationID = convID.(string)
						} else {
							// We'll need the original request messages to generate conversation ID
							// For now, use a temporary approach
							conversationID = fmt.Sprintf("temp_conv_%d", time.Now().UnixNano())
						}

						tokenIDStr := getTokenIDFromRequest(tokenIDInt)
						cacheKey := generateSignatureKey(tokenIDStr, conversationID, 0, i) // messageIndex=0 for response
						GetSignatureCache().Store(cacheKey, *v.Signature)
					}
				}
			}
		case "text":
			responseText += v.Text
		case "tool_use":
			// handled below when building tool call payloads
		default:
			logger.Warn("unknown response type", zap.String("type", v.Type))
		}

		if v.Type == "tool_use" {
			args, _ := json.Marshal(v.Input)
			// Restore any tool name sanitized at the request boundary so the
			// converted OpenAI response carries the original identifier.
			toolName := toolnamesafe.RestoreToolName(c, v.Name)
			tools = append(tools, model.Tool{
				Id:   v.Id,
				Type: "function", // compatible with other OpenAI derivative applications
				Function: &model.Function{
					Name:      toolName,
					Arguments: string(args),
				},
			})
		}
	}

	choice := openai.TextResponseChoice{
		Index: 0,
		Message: model.Message{
			Role:      "assistant",
			Content:   responseText,
			Reasoning: &reasoningText,
			Name:      nil,
			ToolCalls: tools,
		},
		FinishReason: stopReasonClaude2OpenAI(claudeResponse.StopReason),
	}
	if reasoningText != "" {
		choice.Message.SetReasoningContent(c.Query("reasoning_format"), reasoningText)
	}
	fullTextResponse := openai.TextResponse{
		Id:      tracing.GenerateChatCompletionID(c),
		Model:   claudeResponse.Model,
		Object:  "chat.completion",
		Created: helper.GetTimestamp(),
		Choices: []openai.TextResponseChoice{choice},
	}
	return &fullTextResponse
}
