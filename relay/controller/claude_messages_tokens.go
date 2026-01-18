package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"

	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

// getClaudeMessagesPromptTokens estimates the number of prompt tokens for Claude Messages API.
func getClaudeMessagesPromptTokens(ctx context.Context, request *ClaudeMessagesRequest) int {
	logger := gmw.GetLogger(ctx)

	// Convert Claude Messages to OpenAI format for accurate token counting
	openaiRequest := convertClaudeToOpenAIForTokenCounting(request)

	// Use simple character-based estimation for now to avoid tiktoken issues
	// This can be improved later with proper tokenization
	promptTokens := estimateTokensFromMessages(openaiRequest.Messages)

	// Add tokens for tools if present
	toolsTokens := 0
	if len(request.Tools) > 0 {
		toolsTokens = countClaudeToolsTokens(ctx, request.Tools, "gpt-3.5-turbo")
		promptTokens += toolsTokens
	}

	// Add tokens for images using Claude-specific calculation
	imageTokens := calculateClaudeImageTokens(ctx, request)
	promptTokens += imageTokens

	textTokens := promptTokens - imageTokens - toolsTokens

	logger.Debug("estimated prompt tokens for Claude Messages",
		zap.Int("total", promptTokens),
		zap.Int("text", textTokens),
		zap.Int("tools", toolsTokens),
		zap.Int("images", imageTokens),
	)
	return promptTokens
}

// countClaudeToolsTokens estimates tokens for Claude tools.
func countClaudeToolsTokens(ctx context.Context, tools []relaymodel.ClaudeTool, model string) int {
	totalTokens := 0

	for _, tool := range tools {
		// Count tokens for tool name and description
		totalTokens += openai.CountTokenText(tool.Name, model)
		totalTokens += openai.CountTokenText(tool.Description, model)

		// Count tokens for input schema (convert to JSON string for counting)
		if tool.InputSchema != nil {
			if schemaBytes, err := json.Marshal(tool.InputSchema); err == nil {
				totalTokens += openai.CountTokenText(string(schemaBytes), model)
			}
		}
	}

	return totalTokens
}

// convertClaudeToOpenAIForTokenCounting converts Claude Messages format to OpenAI format for token counting.
func convertClaudeToOpenAIForTokenCounting(request *ClaudeMessagesRequest) *relaymodel.GeneralOpenAIRequest {
	openaiRequest := &relaymodel.GeneralOpenAIRequest{
		Model:    request.Model,
		Messages: []relaymodel.Message{},
	}

	// Convert system prompt
	if request.System != nil {
		switch system := request.System.(type) {
		case string:
			if system != "" {
				openaiRequest.Messages = append(openaiRequest.Messages, relaymodel.Message{
					Role:    "system",
					Content: system,
				})
			}
		case []any:
			// For structured system content, extract text parts
			var systemParts []string
			for _, block := range system {
				if blockMap, ok := block.(map[string]any); ok {
					if text, exists := blockMap["text"]; exists {
						if textStr, ok := text.(string); ok {
							systemParts = append(systemParts, textStr)
						}
					}
				}
			}
			if len(systemParts) > 0 {
				systemText := strings.Join(systemParts, "\n")
				openaiRequest.Messages = append(openaiRequest.Messages, relaymodel.Message{
					Role:    "system",
					Content: systemText,
				})
			}
		}
	}

	// Convert messages
	for _, msg := range request.Messages {
		openaiMessage := relaymodel.Message{
			Role: msg.Role,
		}

		// Convert content based on type
		switch content := msg.Content.(type) {
		case string:
			// Simple string content
			openaiMessage.Content = content
		case []any:
			// Structured content blocks - convert to OpenAI format
			var contentParts []relaymodel.MessageContent
			for _, block := range content {
				if blockMap, ok := block.(map[string]any); ok {
					if blockType, exists := blockMap["type"]; exists {
						switch blockType {
						case "text":
							if text, exists := blockMap["text"]; exists {
								if textStr, ok := text.(string); ok {
									contentParts = append(contentParts, relaymodel.MessageContent{
										Type: "text",
										Text: &textStr,
									})
								}
							}
						case "image":
							if source, exists := blockMap["source"]; exists {
								if sourceMap, ok := source.(map[string]any); ok {
									imageURL := relaymodel.ImageURL{}
									if mediaType, exists := sourceMap["media_type"]; exists {
										if data, exists := sourceMap["data"]; exists {
											if dataStr, ok := data.(string); ok {
												// Convert to data URL format for token counting
												imageURL.Url = fmt.Sprintf("data:%s;base64,%s", mediaType, dataStr)
											}
										}
									} else if url, exists := sourceMap["url"]; exists {
										if urlStr, ok := url.(string); ok {
											imageURL.Url = urlStr
										}
									}
									contentParts = append(contentParts, relaymodel.MessageContent{
										Type:     "image_url",
										ImageURL: &imageURL,
									})
								}
							}
						}
					}
				}
			}
			if len(contentParts) > 0 {
				openaiMessage.Content = contentParts
			}
		default:
			// Fallback: convert to string
			if contentBytes, err := json.Marshal(content); err == nil {
				openaiMessage.Content = string(contentBytes)
			}
		}

		openaiRequest.Messages = append(openaiRequest.Messages, openaiMessage)
	}

	return openaiRequest
}

// convertClaudeToolsToOpenAI converts Claude tools to OpenAI format for token counting.
func convertClaudeToolsToOpenAI(claudeTools []relaymodel.ClaudeTool) []relaymodel.Tool {
	var openaiTools []relaymodel.Tool

	for _, tool := range claudeTools {
		openaiTool := relaymodel.Tool{
			Type: "function",
			Function: &relaymodel.Function{
				Name:        tool.Name,
				Description: tool.Description,
			},
		}

		// Convert input schema
		if tool.InputSchema != nil {
			if schemaMap, ok := tool.InputSchema.(map[string]any); ok {
				openaiTool.Function.Parameters = schemaMap
			}
		}

		openaiTools = append(openaiTools, openaiTool)
	}

	return openaiTools
}

// calculateClaudeStructuredOutputCost calculates additional cost for structured output in Claude Messages API.
func calculateClaudeStructuredOutputCost(_ *ClaudeMessagesRequest, _ int, _ float64, _ float64) int64 {
	// No surcharge for structured outputs
	return 0
}

// calculateClaudeImageTokens calculates tokens for images in Claude Messages API.
// According to Claude documentation: tokens = (width px * height px) / 750
func calculateClaudeImageTokens(ctx context.Context, request *ClaudeMessagesRequest) int {
	logger := gmw.GetLogger(ctx)
	totalImageTokens := 0

	// Process messages for images
	for _, message := range request.Messages {
		switch content := message.Content.(type) {
		case []any:
			// Handle content blocks (text, image, etc.)
			for _, block := range content {
				if blockMap, ok := block.(map[string]any); ok {
					if blockType, exists := blockMap["type"]; exists && blockType == "image" {
						imageTokens := calculateSingleImageTokens(ctx, blockMap)
						totalImageTokens += imageTokens
					}
				}
			}
		}
	}

	// Process system prompt for images (if it contains structured content)
	if request.System != nil {
		if systemBlocks, ok := request.System.([]any); ok {
			for _, block := range systemBlocks {
				if blockMap, ok := block.(map[string]any); ok {
					if blockType, exists := blockMap["type"]; exists && blockType == "image" {
						imageTokens := calculateSingleImageTokens(ctx, blockMap)
						totalImageTokens += imageTokens
					}
				}
			}
		}
	}

	logger.Debug("calculated image tokens for Claude Messages", zap.Int("image_tokens", totalImageTokens))
	return totalImageTokens
}

// calculateSingleImageTokens calculates tokens for a single image block.
func calculateSingleImageTokens(ctx context.Context, imageBlock map[string]any) int {
	logger := gmw.GetLogger(ctx)

	source, exists := imageBlock["source"]
	if !exists {
		return 0
	}

	sourceMap, ok := source.(map[string]any)
	if !ok {
		return 0
	}

	sourceType, exists := sourceMap["type"]
	if !exists {
		return 0
	}

	switch sourceType {
	case "base64":
		if data, exists := sourceMap["data"]; exists {
			if dataStr, ok := data.(string); ok {
				estimatedTokens := min(max(len(dataStr)/1000, 50), 2000)
				logger.Debug("estimated tokens for base64 image",
					zap.Int("tokens", estimatedTokens),
					zap.Int("data_length", len(dataStr)),
				)
				return estimatedTokens
			}
		}

	case "url":
		estimatedTokens := 853
		logger.Debug("estimated tokens for URL image", zap.Int("tokens", estimatedTokens))
		return estimatedTokens

	case "file":
		estimatedTokens := 853
		logger.Debug("estimated tokens for file image", zap.Int("tokens", estimatedTokens))
		return estimatedTokens
	}

	return 0
}

// estimateTokensFromMessages provides a simple character-based token estimation.
// This is a fallback when proper tokenization is not available.
func estimateTokensFromMessages(messages []relaymodel.Message) int {
	totalChars := 0

	for _, message := range messages {
		// Count role characters
		totalChars += len(message.Role)

		// Count content characters
		switch content := message.Content.(type) {
		case string:
			totalChars += len(content)
		case []relaymodel.MessageContent:
			for _, part := range content {
				if part.Type == "text" && part.Text != nil {
					totalChars += len(*part.Text)
				}
				// Images are counted separately in calculateClaudeImageTokens
			}
		default:
			// Fallback: convert to string and count
			if contentBytes, err := json.Marshal(content); err == nil {
				totalChars += len(contentBytes)
			}
		}
	}

	// Rough estimation: 4 characters per token (this is a simplification)
	estimatedTokens := max(totalChars/4, 1)
	return estimatedTokens
}
