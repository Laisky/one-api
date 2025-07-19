package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay"
	"github.com/songquanpeng/one-api/relay/adaptor/anthropic"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/billing"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/pricing"
)

// ClaudeMessagesRequest is an alias for the model.ClaudeRequest to follow DRY principle
type ClaudeMessagesRequest = relaymodel.ClaudeRequest

// RelayClaudeMessagesHelper handles Claude Messages API requests with direct pass-through
func RelayClaudeMessagesHelper(c *gin.Context) *relaymodel.ErrorWithStatusCode {
	ctx := c.Request.Context()
	meta := metalib.GetByContext(c)

	// get & validate Claude Messages API request
	claudeRequest, err := getAndValidateClaudeMessagesRequest(c)
	if err != nil {
		logger.Errorf(ctx, "getAndValidateClaudeMessagesRequest failed: %s", err.Error())
		return openai.ErrorWrapper(err, "invalid_claude_messages_request", http.StatusBadRequest)
	}
	meta.IsStream = claudeRequest.Stream != nil && *claudeRequest.Stream

	if reqBody, ok := c.Get(ctxkey.KeyRequestBody); ok {
		logger.Debugf(c.Request.Context(), "get claude messages request: %s\n", string(reqBody.([]byte)))
	}

	// map model name
	meta.OriginModelName = claudeRequest.Model
	claudeRequest.Model = meta.ActualModelName
	meta.ActualModelName = claudeRequest.Model
	metalib.Set2Context(c, meta)

	// get channel model ratio
	channelModelRatio, channelCompletionRatio := getChannelRatios(c, meta.ChannelId)

	// get model ratio using three-layer pricing system
	pricingAdaptor := relay.GetAdaptor(meta.ChannelType)
	modelRatio := pricing.GetModelRatioWithThreeLayers(claudeRequest.Model, channelModelRatio, pricingAdaptor)
	groupRatio := c.GetFloat64(ctxkey.ChannelRatio)

	ratio := modelRatio * groupRatio

	// pre-consume quota based on estimated input tokens
	promptTokens := getClaudeMessagesPromptTokens(c.Request.Context(), claudeRequest)
	meta.PromptTokens = promptTokens
	preConsumedQuota, bizErr := preConsumeClaudeMessagesQuota(c, claudeRequest, promptTokens, ratio, meta)
	if bizErr != nil {
		logger.Warnf(ctx, "preConsumeClaudeMessagesQuota failed: %+v", *bizErr)
		return bizErr
	}

	adaptorInstance := relay.GetAdaptor(meta.APIType)
	if adaptorInstance == nil {
		return openai.ErrorWrapper(errors.New("invalid api type"), "invalid_api_type", http.StatusBadRequest)
	}
	adaptorInstance.Init(meta)

	// convert request using adaptor's ConvertClaudeRequest method
	convertedRequest, err := adaptorInstance.ConvertClaudeRequest(c, claudeRequest)
	if err != nil {
		return openai.ErrorWrapper(err, "convert_request_failed", http.StatusInternalServerError)
	}

	// Use converted request to preserve model mapping
	requestBytes, err := json.Marshal(convertedRequest)
	if err != nil {
		return openai.ErrorWrapper(err, "marshal_request_failed", http.StatusInternalServerError)
	}
	requestBody := bytes.NewReader(requestBytes)

	// for debug
	requestBodyBytes, _ := io.ReadAll(requestBody)
	requestBody = bytes.NewReader(requestBodyBytes)

	// do request
	resp, err := adaptorInstance.DoRequest(c, meta, requestBody)
	if err != nil {
		logger.Errorf(ctx, "DoRequest failed: %s", err.Error())
		return openai.ErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
	}

	// Check for HTTP errors
	if resp.StatusCode != http.StatusOK {
		billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, c.GetInt(ctxkey.TokenId))
		return RelayErrorHandler(resp)
	}

	// Set context flag to indicate Claude Messages native mode
	c.Set(ctxkey.ClaudeMessagesNative, true)

	// do response - let the adapter handle the response conversion
	var usage *relaymodel.Usage
	var respErr *relaymodel.ErrorWithStatusCode

	// Call the adapter's DoResponse method to handle response conversion
	usage, respErr = adaptorInstance.DoResponse(c, resp, meta)

	// If the adapter didn't handle the conversion (e.g., for native Anthropic),
	// fall back to Claude native handlers
	if respErr == nil && usage == nil {
		// Check if there's a converted response from the adapter
		if convertedResp, exists := c.Get(ctxkey.ConvertedResponse); exists {
			// The adapter has already converted the response to Claude format
			// We can use it directly without calling Claude native handlers
			resp = convertedResp.(*http.Response)

			// Copy the response directly to the client
			for k, v := range resp.Header {
				c.Header(k, v[0])
			}
			c.Status(resp.StatusCode)

			// Copy the response body and extract usage information
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				respErr = openai.ErrorWrapper(err, "read_converted_response_failed", http.StatusInternalServerError)
			} else {
				c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), body)

				// Extract usage information from the Claude response body for billing
				var claudeResp relaymodel.ClaudeResponse
				if parseErr := json.Unmarshal(body, &claudeResp); parseErr == nil && claudeResp.Usage.InputTokens > 0 {
					usage = &relaymodel.Usage{
						PromptTokens:     claudeResp.Usage.InputTokens,
						CompletionTokens: claudeResp.Usage.OutputTokens,
						TotalTokens:      claudeResp.Usage.InputTokens + claudeResp.Usage.OutputTokens,
					}
				} else {
					// Fallback: use estimated prompt tokens if parsing fails
					promptTokens := getClaudeMessagesPromptTokens(ctx, claudeRequest)
					usage = &relaymodel.Usage{
						PromptTokens:     promptTokens,
						CompletionTokens: 0, // Unknown completion tokens
						TotalTokens:      promptTokens,
					}
				}
			}
		} else {
			// No converted response, use Claude native handlers for proper format
			if meta.IsStream {
				respErr, usage = anthropic.ClaudeNativeStreamHandler(c, resp)
			} else {
				// For non-streaming, we need the prompt tokens count for usage calculation
				promptTokens := getClaudeMessagesPromptTokens(ctx, claudeRequest)
				respErr, usage = anthropic.ClaudeNativeHandler(c, resp, promptTokens, meta.ActualModelName)
			}
		}
	}

	if respErr != nil {
		logger.Errorf(ctx, "Claude native response handler failed: %+v", *respErr)
		billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, c.GetInt(ctxkey.TokenId))
		return respErr
	}

	// post-consume quota
	quotaId := c.GetInt(ctxkey.Id)
	requestId := c.GetString(ctxkey.RequestId)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		quota := postConsumeClaudeMessagesQuota(ctx, usage, meta, claudeRequest, ratio, preConsumedQuota, modelRatio, groupRatio, channelCompletionRatio)

		// also update user request cost
		if quota != 0 {
			docu := model.NewUserRequestCost(
				quotaId,
				requestId,
				quota,
			)
			if err = docu.Insert(); err != nil {
				logger.Errorf(ctx, "insert user request cost failed: %+v", err)
			}
		}
	}()

	return nil
}

// getAndValidateClaudeMessagesRequest gets and validates Claude Messages API request
func getAndValidateClaudeMessagesRequest(c *gin.Context) (*ClaudeMessagesRequest, error) {
	claudeRequest := &ClaudeMessagesRequest{}
	err := common.UnmarshalBodyReusable(c, claudeRequest)
	if err != nil {
		return nil, err
	}

	// Basic validation
	if claudeRequest.Model == "" {
		return nil, errors.New("model is required")
	}
	if claudeRequest.MaxTokens <= 0 {
		return nil, errors.New("max_tokens must be greater than 0")
	}
	if len(claudeRequest.Messages) == 0 {
		return nil, errors.New("messages array cannot be empty")
	}

	// Validate messages
	for i, message := range claudeRequest.Messages {
		if message.Role == "" {
			return nil, errors.Errorf("message[%d].role is required", i)
		}
		if message.Role != "user" && message.Role != "assistant" {
			return nil, errors.Errorf("message[%d].role must be 'user' or 'assistant'", i)
		}
		if message.Content == nil {
			return nil, errors.Errorf("message[%d].content is required", i)
		}
		// Additional validation for content based on type
		switch content := message.Content.(type) {
		case string:
			if content == "" {
				return nil, errors.Errorf("message[%d].content cannot be empty string", i)
			}
		case []any:
			if len(content) == 0 {
				return nil, errors.Errorf("message[%d].content array cannot be empty", i)
			}
		default:
			// Allow other content types (like structured content blocks)
		}
	}

	return claudeRequest, nil
}

// getClaudeMessagesPromptTokens estimates the number of prompt tokens for Claude Messages API
func getClaudeMessagesPromptTokens(ctx context.Context, request *ClaudeMessagesRequest) int {
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

	logger.Debugf(ctx, "estimated prompt tokens for Claude Messages: %d (text: %d, tools: %d, images: %d)",
		promptTokens, textTokens, toolsTokens, imageTokens)
	return promptTokens
}

// countClaudeToolsTokens estimates tokens for Claude tools
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

// convertClaudeToOpenAIForTokenCounting converts Claude Messages format to OpenAI format for token counting
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

// convertClaudeToolsToOpenAI converts Claude tools to OpenAI format for token counting
func convertClaudeToolsToOpenAI(claudeTools []relaymodel.ClaudeTool) []relaymodel.Tool {
	var openaiTools []relaymodel.Tool

	for _, tool := range claudeTools {
		openaiTool := relaymodel.Tool{
			Type: "function",
			Function: relaymodel.Function{
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

// calculateClaudeStructuredOutputCost calculates additional cost for structured output in Claude Messages API
func calculateClaudeStructuredOutputCost(request *ClaudeMessagesRequest, completionTokens int, modelRatio float64) int64 {
	// Check if this is a structured output request
	// In Claude Messages API, structured output is typically indicated by specific tool usage or response format
	// For now, we'll check if there are tools that might indicate structured output

	// This is a simplified implementation - in a real scenario, you might want to:
	// 1. Check for specific tool types that indicate structured output
	// 2. Check for response format specifications
	// 3. Analyze the actual response content for structured patterns

	hasStructuredOutput := false

	// Check if any tools are present (which might indicate structured output)
	if len(request.Tools) > 0 {
		// For now, assume any tool usage might involve structured output
		// This could be refined based on specific tool types or patterns
		hasStructuredOutput = true
	}

	// Apply 25% surcharge on completion tokens for structured output (same as OpenAI)
	if hasStructuredOutput {
		structuredOutputCost := int64(math.Ceil(float64(completionTokens) * 0.25 * modelRatio))
		return structuredOutputCost
	}

	return 0
}

// calculateClaudeImageTokens calculates tokens for images in Claude Messages API
// According to Claude documentation: tokens = (width px * height px) / 750
func calculateClaudeImageTokens(ctx context.Context, request *ClaudeMessagesRequest) int {
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

	logger.Debugf(ctx, "calculated image tokens for Claude Messages: %d", totalImageTokens)
	return totalImageTokens
}

// calculateSingleImageTokens calculates tokens for a single image block
func calculateSingleImageTokens(ctx context.Context, imageBlock map[string]any) int {
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
		// For base64 images, we need to decode and get dimensions
		// This is complex, so we'll use a reasonable estimate
		// Based on Claude's examples: ~1590 tokens for 1092x1092 px image
		// We'll estimate based on data size as a proxy
		if data, exists := sourceMap["data"]; exists {
			if dataStr, ok := data.(string); ok {
				// Rough estimation: base64 data length correlates with image size
				// A 1092x1092 image (~1.19 megapixels) with ~1590 tokens has base64 length ~1.5MB
				// Estimate: tokens ≈ base64_length / 1000 (very rough approximation)
				estimatedTokens := len(dataStr) / 1000
				if estimatedTokens < 50 {
					estimatedTokens = 50 // Minimum for small images
				}
				if estimatedTokens > 2000 {
					estimatedTokens = 2000 // Cap for very large images
				}
				logger.Debugf(ctx, "estimated tokens for base64 image: %d (based on data length %d)", estimatedTokens, len(dataStr))
				return estimatedTokens
			}
		}

	case "url":
		// For URL images, we can't easily determine size without fetching
		// Use a reasonable default based on typical web images
		// Most web images are in the 500x500 to 1000x1000 range
		// Using Claude's formula: (800 * 800) / 750 ≈ 853 tokens
		estimatedTokens := 853
		logger.Debugf(ctx, "estimated tokens for URL image: %d (default estimate)", estimatedTokens)
		return estimatedTokens

	case "file":
		// For file-based images, we also can't determine size easily
		// Use a similar default as URL images
		estimatedTokens := 853
		logger.Debugf(ctx, "estimated tokens for file image: %d (default estimate)", estimatedTokens)
		return estimatedTokens
	}

	return 0
}

// estimateTokensFromMessages provides a simple character-based token estimation
// This is a fallback when proper tokenization is not available
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

// preConsumeClaudeMessagesQuota pre-consumes quota for Claude Messages API requests
func preConsumeClaudeMessagesQuota(c *gin.Context, request *ClaudeMessagesRequest, promptTokens int, ratio float64, meta *metalib.Meta) (int64, *relaymodel.ErrorWithStatusCode) {
	// Use similar logic to ChatCompletion pre-consumption
	preConsumedTokens := int64(promptTokens)
	if request.MaxTokens > 0 {
		preConsumedTokens += int64(request.MaxTokens)
	}

	baseQuota := int64(float64(preConsumedTokens) * ratio)
	if ratio != 0 && baseQuota <= 0 {
		baseQuota = 1
	}

	// Check user quota first
	tokenQuota := c.GetInt64(ctxkey.TokenQuota)
	tokenQuotaUnlimited := c.GetBool(ctxkey.TokenQuotaUnlimited)
	userQuota, err := model.CacheGetUserQuota(c.Request.Context(), meta.UserId)
	if err != nil {
		return baseQuota, openai.ErrorWrapper(err, "get_user_quota_failed", http.StatusInternalServerError)
	}
	if userQuota-baseQuota < 0 {
		return baseQuota, openai.ErrorWrapper(errors.New("user quota is not enough"), "insufficient_user_quota", http.StatusForbidden)
	}
	err = model.CacheDecreaseUserQuota(meta.UserId, baseQuota)
	if err != nil {
		return baseQuota, openai.ErrorWrapper(err, "decrease_user_quota_failed", http.StatusInternalServerError)
	}
	if userQuota > 100*baseQuota &&
		(tokenQuotaUnlimited || tokenQuota > 100*baseQuota) {
		// in this case, we do not pre-consume quota
		// because the user and token have enough quota
		baseQuota = 0
		logger.Info(c.Request.Context(), fmt.Sprintf("user %d has enough quota %d, trusted and no need to pre-consume", meta.UserId, userQuota))
	}
	if baseQuota > 0 {
		err := model.PreConsumeTokenQuota(meta.TokenId, baseQuota)
		if err != nil {
			return baseQuota, openai.ErrorWrapper(err, "pre_consume_token_quota_failed", http.StatusForbidden)
		}
	}

	logger.Debugf(c.Request.Context(), "pre-consumed quota for Claude Messages: %d (tokens: %d, ratio: %f)", baseQuota, int(preConsumedTokens), ratio)
	return baseQuota, nil
}

// postConsumeClaudeMessagesQuota calculates and applies final quota consumption for Claude Messages API
func postConsumeClaudeMessagesQuota(ctx context.Context, usage *relaymodel.Usage, meta *metalib.Meta, request *ClaudeMessagesRequest, ratio float64, preConsumedQuota int64, modelRatio float64, groupRatio float64, channelCompletionRatio map[string]float64) int64 {
	if usage == nil {
		logger.Warnf(ctx, "usage is nil for Claude Messages API")
		return 0
	}

	// Use three-layer pricing system for completion ratio
	pricingAdaptor := relay.GetAdaptor(meta.ChannelType)
	completionRatio := pricing.GetCompletionRatioWithThreeLayers(request.Model, channelCompletionRatio, pricingAdaptor)
	promptTokens := usage.PromptTokens
	completionTokens := usage.CompletionTokens

	// Calculate base quota
	baseQuota := int64(math.Ceil((float64(promptTokens) + float64(completionTokens)*completionRatio) * ratio))

	// Add structured output cost if applicable
	structuredOutputCost := calculateClaudeStructuredOutputCost(request, completionTokens, modelRatio)

	// Total quota includes base cost, tools cost, and structured output cost
	quota := baseQuota + usage.ToolsCost + structuredOutputCost
	if ratio != 0 && quota <= 0 {
		quota = 1
	}

	totalTokens := promptTokens + completionTokens
	if totalTokens == 0 {
		// in this case, must be some error happened
		// we cannot just return, because we may have to return the pre-consumed quota
		quota = 0
	}
	// Use centralized detailed billing function to follow DRY principle
	quotaDelta := quota - preConsumedQuota
	billing.PostConsumeQuotaDetailed(ctx, meta.TokenId, quotaDelta, quota, meta.UserId, meta.ChannelId,
		promptTokens, completionTokens, modelRatio, groupRatio, request.Model, meta.TokenName,
		meta.IsStream, meta.StartTime, false, completionRatio, usage.ToolsCost)

	logger.Debugf(ctx, "Claude Messages quota: pre-consumed=%d, actual=%d, difference=%d", preConsumedQuota, quota, quotaDelta)
	return quota
}
