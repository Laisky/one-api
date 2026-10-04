package controller

import (
	"context"
	"encoding/json"
	"math"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/relay/adaptor/common/claudevision"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

const claudeFileImageFallbackTokens = 853

// fastTokenEstimateThreshold names the former shortcut boundary for compatibility regressions.
const fastTokenEstimateThreshold = 1 * 1024 * 1024

// estimateClaudeMessagesPromptTokens counts the same semantic content for every serialized body size.
// The body size does not justify a lower quote; transport limits bound accepted input separately.
func estimateClaudeMessagesPromptTokens(ctx context.Context, request *ClaudeMessagesRequest, _ int) (int, error) {
	return getClaudeMessagesPromptTokens(ctx, request)
}

// getClaudeMessagesPromptTokens returns a native prompt estimate or an error
// for malformed PDF sources or unrepresentable document and prompt token sums.
func getClaudeMessagesPromptTokens(ctx context.Context, request *ClaudeMessagesRequest) (int, error) {
	if request == nil {
		return 0, errors.New("nil native Claude prompt quote")
	}
	logger := gmw.GetLogger(ctx)

	// Convert Claude Messages to OpenAI format for accurate token counting
	openaiRequest := convertClaudeToOpenAIForTokenCounting(request)

	// Use OpenAI token counter for accurate tokenization
	modelName := claudeReservationModel(request)
	promptTokens := openai.CountTokenMessages(ctx, openaiRequest.Messages, modelName)

	// Add tokens for tools if present
	if len(request.Tools) > 0 {
		promptTokens += countClaudeToolsTokens(ctx, request.Tools, request.Model)
	}

	fileImageTokens := countClaudeFileImageTokens(request)
	if claudevision.IsSonnet55(modelName) {
		// Converted URL images were already counted above; add only missing native blocks.
		convertedImages := 0
		for _, message := range openaiRequest.Messages {
			for _, part := range message.ParseContent() {
				if part.Type == relaymodel.ContentTypeImageURL {
					convertedImages++
				}
			}
		}
		fileImageTokens = max(0, countClaudeNativeImageAllowance(request)-convertedImages*claudevision.Sonnet55MaxImageTokens)
	}
	if fileImageTokens > 0 {
		promptTokens += fileImageTokens
	}

	documentTokens, err := countClaudeNativeDocumentAllowance(request)
	if err != nil {
		return 0, err
	}
	if promptTokens < 0 || documentTokens > math.MaxInt-promptTokens {
		return 0, errors.New("native Claude prompt token sum exceeds integer range")
	}
	promptTokens += documentTokens

	logger.Debug("estimated prompt tokens for Claude Messages",
		zap.Int("total", promptTokens),
		zap.String("model", request.Model),
		zap.Int("image_fallback", fileImageTokens),
		zap.Int("document_allowance", documentTokens),
	)
	return promptTokens, nil
}

// countClaudeFileImageTokens estimates tokens for image blocks that reference file-based sources.
// Parameters: request is the Claude Messages API request.
// Returns: the estimated token count for file-based images.
func countClaudeFileImageTokens(request *ClaudeMessagesRequest) int {
	if request == nil {
		return 0
	}

	total := 0
	for _, message := range request.Messages {
		total += countClaudeFileImageTokensFromContent(message.Content)
	}
	if request.System != nil {
		if systemBlocks, ok := request.System.([]any); ok {
			total += countClaudeFileImageTokensFromBlocks(systemBlocks)
		}
	}
	return total
}

// countClaudeFileImageTokensFromContent walks a Claude message content and counts file-based images.
// Parameters: content is the Claude message content (string or blocks).
// Returns: the estimated token count for file-based images in the content.
func countClaudeFileImageTokensFromContent(content any) int {
	switch typed := content.(type) {
	case []any:
		return countClaudeFileImageTokensFromBlocks(typed)
	default:
		return 0
	}
}

// countClaudeFileImageTokensFromBlocks counts file-based image blocks in structured content blocks.
// Parameters: blocks is the list of structured content blocks.
// Returns: the estimated token count for file-based images in the blocks.
func countClaudeFileImageTokensFromBlocks(blocks []any) int {
	total := 0
	for _, block := range blocks {
		blockMap, ok := block.(map[string]any)
		if !ok {
			continue
		}
		blockType, _ := blockMap["type"].(string)
		if blockType == "tool_result" {
			total += countClaudeFileImageTokensFromContent(blockMap["content"])
			continue
		}
		if blockType != "image" {
			continue
		}
		source, ok := blockMap["source"].(map[string]any)
		if !ok {
			continue
		}
		sourceType, _ := source["type"].(string)
		if sourceType == "file" {
			total += claudeFileImageFallbackTokens
			continue
		}
		if sourceType == "" {
			if _, exists := source["file_id"]; exists {
				total += claudeFileImageFallbackTokens
				continue
			}
			if _, exists := source["file"]; exists {
				total += claudeFileImageFallbackTokens
			}
		}
	}
	return total
}

// countClaudeToolsTokens estimates tokens for Claude tools.
// Tools with DeferLoading=true are skipped as they are not loaded into context.
func countClaudeToolsTokens(ctx context.Context, tools []relaymodel.ClaudeTool, model string) int {
	totalTokens := 0

	for _, tool := range tools {
		// Skip deferred tools - they are not loaded into context
		if tool.DeferLoading != nil && *tool.DeferLoading {
			continue
		}

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
			if parts := claudeContentTokenParts(content); len(parts) > 0 {
				openaiMessage.Content = parts
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
