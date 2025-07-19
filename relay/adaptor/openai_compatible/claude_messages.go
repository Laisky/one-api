package openai_compatible

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
)

// ConvertClaudeRequest converts Claude Messages API request to OpenAI format for OpenAI-compatible adapters
func ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}

	// Convert Claude Messages API request to OpenAI format first
	openaiRequest := &model.GeneralOpenAIRequest{
		Model:       request.Model,
		MaxTokens:   request.MaxTokens,
		Temperature: request.Temperature,
		TopP:        request.TopP,
		Stream:      request.Stream != nil && *request.Stream,
		Stop:        request.StopSequences,
	}

	// Convert system message if present
	if request.System != nil {
		switch system := request.System.(type) {
		case string:
			if system != "" {
				openaiRequest.Messages = append(openaiRequest.Messages, model.Message{
					Role:    "system",
					Content: system,
				})
			}
		case []any:
			// Handle structured system content
			var systemContent []model.MessageContent
			for _, block := range system {
				if blockMap, ok := block.(map[string]any); ok {
					if blockType, exists := blockMap["type"]; exists && blockType == "text" {
						if text, exists := blockMap["text"]; exists {
							if textStr, ok := text.(string); ok {
								systemContent = append(systemContent, model.MessageContent{
									Type: "text",
									Text: &textStr,
								})
							}
						}
					}
				}
			}
			if len(systemContent) > 0 {
				openaiRequest.Messages = append(openaiRequest.Messages, model.Message{
					Role:    "system",
					Content: systemContent,
				})
			}
		}
	}

	// Convert messages
	for _, msg := range request.Messages {
		openaiMessage := model.Message{
			Role: msg.Role,
		}

		// Convert content based on type
		switch content := msg.Content.(type) {
		case string:
			// Simple string content
			openaiMessage.Content = content
		case []any:
			// Structured content blocks - convert to OpenAI format
			var contentParts []model.MessageContent
			for _, block := range content {
				if blockMap, ok := block.(map[string]any); ok {
					if blockType, exists := blockMap["type"]; exists {
						switch blockType {
						case "text":
							if text, exists := blockMap["text"]; exists {
								if textStr, ok := text.(string); ok {
									contentParts = append(contentParts, model.MessageContent{
										Type: "text",
										Text: &textStr,
									})
								}
							}
						case "image":
							// Handle image content
							if source, exists := blockMap["source"]; exists {
								if sourceMap, ok := source.(map[string]any); ok {
									if sourceType, exists := sourceMap["type"]; exists {
										switch sourceType {
										case "base64":
											if mediaType, exists := sourceMap["media_type"]; exists {
												if data, exists := sourceMap["data"]; exists {
													if mediaTypeStr, ok := mediaType.(string); ok {
														if dataStr, ok := data.(string); ok {
															contentParts = append(contentParts, model.MessageContent{
																Type: "image_url",
																ImageURL: &model.ImageURL{
																	Url: fmt.Sprintf("data:%s;base64,%s", mediaTypeStr, dataStr),
																},
															})
														}
													}
												}
											}
										case "url":
											if url, exists := sourceMap["url"]; exists {
												if urlStr, ok := url.(string); ok {
													contentParts = append(contentParts, model.MessageContent{
														Type: "image_url",
														ImageURL: &model.ImageURL{
															Url: urlStr,
														},
													})
												}
											}
										}
									}
								}
							}
						case "tool_use":
							// Handle tool use - convert to OpenAI tool call format
							if id, exists := blockMap["id"]; exists {
								if name, exists := blockMap["name"]; exists {
									if input, exists := blockMap["input"]; exists {
										if idStr, ok := id.(string); ok {
											if nameStr, ok := name.(string); ok {
												var argsStr string
												if inputBytes, err := json.Marshal(input); err == nil {
													argsStr = string(inputBytes)
												}
												openaiMessage.ToolCalls = append(openaiMessage.ToolCalls, model.Tool{
													Id:   idStr,
													Type: "function",
													Function: model.Function{
														Name:      nameStr,
														Arguments: argsStr,
													},
												})
											}
										}
									}
								}
							}
						case "tool_result":
							// Handle tool result - this should be in a separate message
							if toolCallId, exists := blockMap["tool_call_id"]; exists {
								if content, exists := blockMap["content"]; exists {
									if toolCallIdStr, ok := toolCallId.(string); ok {
										var contentStr string
										switch c := content.(type) {
										case string:
											contentStr = c
										case []any:
											// Handle structured tool result content
											for _, item := range c {
												if itemMap, ok := item.(map[string]any); ok {
													if itemType, exists := itemMap["type"]; exists && itemType == "text" {
														if text, exists := itemMap["text"]; exists {
															if textStr, ok := text.(string); ok {
																contentStr += textStr
															}
														}
													}
												}
											}
										}
										openaiMessage.ToolCallId = toolCallIdStr
										openaiMessage.Content = contentStr
									}
								}
							}
						}
					}
				}
			}
			if len(contentParts) > 0 {
				openaiMessage.Content = contentParts
			}
		}

		openaiRequest.Messages = append(openaiRequest.Messages, openaiMessage)
	}

	// Convert tools if present
	if len(request.Tools) > 0 {
		var tools []model.Tool
		for _, claudeTool := range request.Tools {
			tool := model.Tool{
				Type: "function",
				Function: model.Function{
					Name:        claudeTool.Name,
					Description: claudeTool.Description,
					Parameters:  claudeTool.InputSchema.(map[string]any),
				},
			}
			tools = append(tools, tool)
		}
		openaiRequest.Tools = tools
	}

	// Convert tool choice if present
	if request.ToolChoice != nil {
		openaiRequest.ToolChoice = request.ToolChoice
	}

	// Mark this as a Claude Messages conversion for response handling
	c.Set(ctxkey.ClaudeMessagesConversion, true)
	c.Set(ctxkey.OriginalClaudeRequest, request)

	return openaiRequest, nil
}

// HandleClaudeMessagesResponse handles Claude Messages response conversion for OpenAI-compatible adapters
// This should be called in the adapter's DoResponse method when ClaudeMessagesConversion flag is set
func HandleClaudeMessagesResponse(c *gin.Context, resp *http.Response, meta *meta.Meta, handler func(*gin.Context, *http.Response, int, string) (*model.ErrorWithStatusCode, *model.Usage)) (*model.Usage, *model.ErrorWithStatusCode) {
	// Check if this is a Claude Messages conversion
	if isClaudeConversion, exists := c.Get(ctxkey.ClaudeMessagesConversion); !exists || !isClaudeConversion.(bool) {
		// Not a Claude Messages conversion, proceed normally
		err, usage := handler(c, resp, meta.PromptTokens, meta.ActualModelName)
		return usage, err
	}

	// This is a Claude Messages conversion, we need to convert the response
	// For now, we'll use a simplified approach - let the normal handler process the response
	// and then indicate that Claude Messages conversion was handled
	err, _ := handler(c, resp, meta.PromptTokens, meta.ActualModelName)
	if err != nil {
		return nil, err
	}

	// For Claude Messages conversion, we return nil usage to indicate
	// that the conversion was handled by the adapter
	return nil, nil
}
