package anthropic

import (
	"encoding/json"
	"fmt"
	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/image"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"math"
	"strings"
)

// isModelSupportThinking is used to check if the model supports extended thinking
func isModelSupportThinking(model string) bool {
	if strings.Contains(model, "claude-3-5") ||
		strings.Contains(model, "claude-2") ||
		strings.Contains(model, "claude-instant-1") {
		return false
	}

	return true
}

// ConvertClaudeRequest converts a Claude Messages API request to anthropic.Request format
func ConvertClaudeRequest(c *gin.Context, claudeRequest model.ClaudeRequest) (*Request, error) {
	// Convert tools
	claudeTools := make([]Tool, 0, len(claudeRequest.Tools))
	for _, tool := range claudeRequest.Tools {
		inputSchema, err := claudeInputSchema(tool.InputSchema)
		if err != nil {
			return nil, err
		}
		claudeTools = append(claudeTools, Tool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: inputSchema,
		})
	}

	// Convert messages
	claudeMessages := make([]Message, 0, len(claudeRequest.Messages))
	for _, msg := range claudeRequest.Messages {
		claudeMessage := Message{
			Role: msg.Role,
		}

		// Convert content based on type
		switch content := msg.Content.(type) {
		case string:
			// Simple string content
			claudeMessage.Content = []Content{
				{
					Type: "text",
					Text: content,
				},
			}
		case []any:
			// Structured content blocks
			for _, block := range content {
				if blockMap, ok := block.(map[string]any); ok {
					contentBlock := Content{}
					if blockType, exists := blockMap["type"]; exists {
						if typeStr, ok := blockType.(string); ok {
							contentBlock.Type = typeStr
						}
					}
					if text, exists := blockMap["text"]; exists {
						if textStr, ok := text.(string); ok {
							contentBlock.Text = textStr
						}
					}
					// Handle thinking blocks (extended thinking feature)
					if thinking, exists := blockMap["thinking"]; exists {
						if thinkingStr, ok := thinking.(string); ok {
							contentBlock.Thinking = &thinkingStr
						}
					}
					if signature, exists := blockMap["signature"]; exists {
						if sigStr, ok := signature.(string); ok {
							contentBlock.Signature = &sigStr
						}
					}
					// Handle tool_use blocks
					if id, exists := blockMap["id"]; exists {
						if idStr, ok := id.(string); ok {
							contentBlock.Id = idStr
						}
					}
					if name, exists := blockMap["name"]; exists {
						if nameStr, ok := name.(string); ok {
							contentBlock.Name = nameStr
						}
					}
					if input, exists := blockMap["input"]; exists {
						contentBlock.Input = input
					}
					// Handle tool_result blocks
					if contentVal, exists := blockMap["content"]; exists {
						if contentStr, ok := contentVal.(string); ok {
							contentBlock.Content = contentStr
						}
					}
					if toolUseId, exists := blockMap["tool_use_id"]; exists {
						if toolUseIdStr, ok := toolUseId.(string); ok {
							contentBlock.ToolUseId = toolUseIdStr
						}
					}
					// Handle image content
					if source, exists := blockMap["source"]; exists {
						if sourceMap, ok := source.(map[string]any); ok {
							contentBlock.Source = &ImageSource{}
							if sourceType, exists := sourceMap["type"]; exists {
								if typeStr, ok := sourceType.(string); ok {
									contentBlock.Source.Type = typeStr
								}
							}
							if mediaType, exists := sourceMap["media_type"]; exists {
								if mediaTypeStr, ok := mediaType.(string); ok {
									contentBlock.Source.MediaType = mediaTypeStr
								}
							}
							if data, exists := sourceMap["data"]; exists {
								if dataStr, ok := data.(string); ok {
									contentBlock.Source.Data = dataStr
								}
							}
						}
					}
					claudeMessage.Content = append(claudeMessage.Content, contentBlock)
				}
			}
		default:
			// Try to marshal and unmarshal as Content slice
			contentBytes, err := json.Marshal(content)
			if err != nil {
				return nil, errors.Wrap(err, "failed to marshal message content")
			}
			var contentBlocks []Content
			if err := json.Unmarshal(contentBytes, &contentBlocks); err != nil {
				return nil, errors.Wrap(err, "failed to unmarshal message content")
			}
			// Message.Content has no omitempty (Anthropic requires the field), so a
			// nil slice would serialize as `"content": null` and be rejected. This
			// arm is reached whenever content is neither a string nor an array —
			// a literal null, or an object — where Unmarshal leaves the slice nil.
			// Every other adaptor guards this with len(); Anthropic was the outlier.
			if len(contentBlocks) == 0 {
				contentBlocks = []Content{}
			}
			claudeMessage.Content = contentBlocks
		}

		claudeMessages = append(claudeMessages, claudeMessage)
	}

	// Convert system prompt
	var systemPrompt string
	if claudeRequest.System != nil {
		switch system := claudeRequest.System.(type) {
		case string:
			systemPrompt = system
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
			systemPrompt = strings.Join(systemParts, "\n")
		}
	}

	// Build the request
	request := &Request{
		Model:         claudeRequest.Model,
		MaxTokens:     claudeRequest.MaxTokens,
		Messages:      claudeMessages,
		System:        systemPrompt,
		Temperature:   claudeRequest.Temperature,
		TopP:          claudeRequest.TopP,
		Stream:        claudeRequest.Stream != nil && *claudeRequest.Stream,
		StopSequences: claudeRequest.StopSequences,
		Tools:         claudeTools,
		ToolChoice:    claudeRequest.ToolChoice,
		Thinking:      claudeRequest.Thinking,
		OutputConfig:  claudeRequest.OutputConfig,
	}

	// Handle TopK (convert from *int to int)
	if claudeRequest.TopK != nil {
		request.TopK = claudeRequest.TopK
	}

	NormalizeModelCompatibility(CompatibilityModel(c, request.Model), &request.Temperature, &request.TopP, &request.TopK, &request.Thinking)

	return request, nil
}

// ConvertRequest converts portable Chat controls and messages into an independently owned Claude request.
func ConvertRequest(c *gin.Context, textRequest model.GeneralOpenAIRequest) (*Request, error) {
	logger := gmw.GetLogger(c)
	var controlsErr error
	textRequest, controlsErr = prepareClaudeChatControls(c, textRequest)
	if controlsErr != nil {
		return nil, controlsErr
	}
	compatibilityModel := CompatibilityModel(c, textRequest.Model)

	claudeTools := make([]Tool, 0, len(textRequest.Tools))

	for _, tool := range textRequest.Tools {
		// Add nil check for Function pointer
		if tool.Function == nil {
			return nil, errors.New("tool function is nil")
		}

		schema, err := claudeInputSchema(tool.Function.Parameters)
		if err != nil {
			return nil, err
		}
		claudeTools = append(claudeTools, Tool{
			Strict:      tool.Function.Strict,
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			InputSchema: schema,
		})
	}

	claudeRequest := Request{
		Model:        textRequest.Model,
		MaxTokens:    textRequest.MaxTokens,
		Temperature:  textRequest.Temperature,
		TopP:         textRequest.TopP,
		TopK:         textRequest.TopK,
		Stream:       textRequest.Stream,
		Tools:        claudeTools,
		Thinking:     textRequest.Thinking,
		OutputConfig: textRequest.OutputConfig,
	}

	if claudeRequest.MaxTokens == 0 {
		claudeRequest.MaxTokens = config.DefaultMaxToken
	}

	// Track if we need to use fallback mode (will be set if any signature restoration fails)
	var useFallbackMode bool

	if isModelSupportThinking(textRequest.Model) &&
		c != nil && c.Request != nil && c.Request.URL.Query().Has("thinking") && claudeRequest.Thinking == nil {
		budgetTokens := int(math.Min(1024, float64(claudeRequest.MaxTokens/2)))
		claudeRequest.Thinking = &model.Thinking{
			Type:         "enabled",
			BudgetTokens: &budgetTokens,
		}
	}

	NormalizeModelCompatibility(compatibilityModel, &claudeRequest.Temperature, &claudeRequest.TopP, &claudeRequest.TopK, &claudeRequest.Thinking)

	if isModelSupportThinking(textRequest.Model) &&
		claudeRequest.Thinking != nil && claudeRequest.Thinking.Type != "disabled" {
		// For adaptive thinking, budget_tokens must not be present
		if claudeRequest.Thinking.Type == "adaptive" {
			claudeRequest.Thinking.BudgetTokens = nil
			logger.Debug("using adaptive thinking mode, stripped budget_tokens")
		}

		if claudeRequest.Thinking.Type == "enabled" && claudeRequest.MaxTokens <= 1024 {
			return nil, errors.New("max_tokens must be greater than 1024 when using extended thinking")
		}

		// top_p must be nil when using extended thinking
		claudeRequest.TopP = nil
	}

	var err error
	claudeRequest.ToolChoice, err = convertClaudeToolChoice(textRequest.ToolChoice, len(claudeTools) > 0)
	if err != nil {
		return nil, err
	}
	claudeRequest.StopSequences, err = convertClaudeStop(textRequest.Stop)
	if err != nil {
		return nil, err
	}
	if claudeRequest.Thinking != nil && claudeRequest.Thinking.Type != "disabled" {
		claudeRequest.Temperature, claudeRequest.TopP, claudeRequest.TopK = nil, nil, nil
	}
	if claudeRequest.Temperature != nil {
		claudeRequest.TopP = nil
	}

	systemMessageCount := 0
	emptySystemMessageCount := 0
	systemPrompts := make([]string, 0)

	for _, message := range textRequest.Messages {
		if message.Role == "system" {
			systemMessageCount++

			systemContent := strings.TrimSpace(message.StringContent())
			if systemContent == "" {
				emptySystemMessageCount++
				logger.Debug("skip empty system message during Anthropic conversion",
					zap.Int("system_message_index", systemMessageCount-1),
				)
				continue
			}

			systemPrompts = append(systemPrompts, systemContent)
			continue
		}

		if message.Role == "tool" {
			toolResultContent := message.StringContent()
			if toolResultContent == "" {
				for _, part := range message.ParseContent() {
					if part.Type == model.ContentTypeText && part.Text != nil {
						toolResultContent += *part.Text
					}
				}
			}

			logger.Debug("convert OpenAI tool role to Anthropic tool_result",
				zap.Int("message_index", len(claudeRequest.Messages)),
				zap.Bool("has_tool_call_id", message.ToolCallId != ""),
				zap.Bool("string_content", message.IsStringContent()),
				zap.Int("tool_calls_count", len(message.ToolCalls)),
			)
			if message.ToolCallId == "" {
				logger.Debug("tool role message missing tool_call_id during Anthropic conversion",
					zap.Int("message_index", len(claudeRequest.Messages)),
				)
			}

			claudeRequest.Messages = append(claudeRequest.Messages, Message{
				Role: "user",
				Content: []Content{{
					Type:      "tool_result",
					Content:   toolResultContent,
					ToolUseId: message.ToolCallId,
				}},
			})
			continue
		}

		claudeMessage := Message{
			Role: message.Role,
		}
		var content Content
		if message.IsStringContent() {
			stringContent := message.StringContent()

			if stringContent != "" {
				// For assistant messages with thinking enabled, check if we need to add thinking block
				if message.Role == "assistant" && claudeRequest.Thinking != nil {
					// Check if this message has reasoning content that should be converted to thinking block
					var reasoningContent string
					if message.Reasoning != nil {
						reasoningContent = *message.Reasoning
					} else if message.ReasoningContent != nil {
						reasoningContent = *message.ReasoningContent
					} else if message.Thinking != nil {
						reasoningContent = *message.Thinking
					}

					// If we have reasoning content, handle it appropriately
					if reasoningContent != "" {
						var signatureRestored bool
						thinkingContent := Content{
							Type:     "thinking",
							Thinking: &reasoningContent,
						}

						// Try to restore signature from cache if available
						if tokenID, exists := c.Get(ctxkey.TokenId); exists {
							if tokenIDInt, ok := tokenID.(int); ok {
								tokenIDStr := getTokenIDFromRequest(tokenIDInt)
								conversationID := generateConversationID(textRequest.Messages)

								// Store conversation ID in context for later use
								c.Set(ctxkey.ConversationId, conversationID)

								// Try to restore signature for this thinking block
								messageIndex := len(claudeRequest.Messages) // Current message index
								cacheKey := generateSignatureKey(tokenIDStr, conversationID, messageIndex, 0)

								if signature := GetSignatureCache().Get(cacheKey); signature != nil {
									thinkingContent.Signature = signature
									signatureRestored = true
								}
							}
						}

						// If signature was not restored, use fallback approach
						if !signatureRestored {
							// Set fallback mode flag
							useFallbackMode = true

							// Convert thinking content to <think> format and prepend to text content
							thinkingPrefix := fmt.Sprintf("<think>%s</think>\n\n", reasoningContent)

							// Find the first text content and prepend thinking
							for i := range claudeMessage.Content {
								if claudeMessage.Content[i].Type == "text" {
									claudeMessage.Content[i].Text = thinkingPrefix + claudeMessage.Content[i].Text
									break
								}
							}

							// If no text content found, create one with thinking prefix and original text
							if len(claudeMessage.Content) == 0 {
								claudeMessage.Content = append(claudeMessage.Content, Content{
									Type: "text",
									Text: thinkingPrefix + stringContent,
								})
							}
						} else {
							// Signature was restored, use proper thinking block
							claudeMessage.Content = append([]Content{thinkingContent}, claudeMessage.Content...)
						}
					}
				}

				// Only add text content if it's not empty
				content.Type = "text"
				content.Text = stringContent
				claudeMessage.Content = append(claudeMessage.Content, content)
			}

			// Add tool calls
			for i := range message.ToolCalls {
				inputParam := make(map[string]any)
				rawArguments, err := message.ToolCalls[i].Function.ArgumentsJSON()
				if err != nil {
					return nil, errors.Wrapf(err, "read tool call arguments for tool %s", message.ToolCalls[i].FunctionName())
				}
				if err := decodeClaudeJSON([]byte(rawArguments), &inputParam); err != nil {
					return nil, errors.Wrapf(err, "unmarshal tool call arguments for tool %s", message.ToolCalls[i].FunctionName())
				}
				claudeMessage.Content = append(claudeMessage.Content, Content{
					Type:  "tool_use",
					Id:    message.ToolCalls[i].Id,
					Name:  message.ToolCalls[i].FunctionName(),
					Input: inputParam,
				})
			}

			// Claude requires at least one content block per message
			if len(claudeMessage.Content) == 0 {
				return nil, errors.Wrap(errors.New("message must have at least one content block"), "validate message content")
			}

			claudeRequest.Messages = append(claudeRequest.Messages, claudeMessage)
			continue
		}
		var contents []Content

		// Store reasoning content for later processing
		var reasoningContent string
		var needsThinkingProcessing bool
		if message.Role == "assistant" && claudeRequest.Thinking != nil {
			// Check if this message has reasoning content that should be converted to thinking block
			if message.Reasoning != nil {
				reasoningContent = *message.Reasoning
				needsThinkingProcessing = true
			} else if message.ReasoningContent != nil {
				reasoningContent = *message.ReasoningContent
				needsThinkingProcessing = true
			} else if message.Thinking != nil {
				reasoningContent = *message.Thinking
				needsThinkingProcessing = true
			}
		}

		openaiContent := message.ParseContent()
		for _, part := range openaiContent {
			var content Content
			switch part.Type {
			case model.ContentTypeText:
				content.Type = "text"
				if part.Text != nil && *part.Text != "" {
					// Only add text content if it's not empty
					content.Text = *part.Text
					contents = append(contents, content)
				}
			case model.ContentTypeImageURL:
				content.Type = "image"
				content.Source = &ImageSource{
					Type: "base64",
				}
				mimeType, data, _ := image.GetImageFromUrl(part.ImageURL.Url)
				content.Source.MediaType = mimeType
				content.Source.Data = data
				contents = append(contents, content)
			}
		}

		// Add tool calls for non-string content messages
		for i := range message.ToolCalls {
			inputParam := make(map[string]any)
			rawArguments, err := message.ToolCalls[i].Function.ArgumentsJSON()
			if err != nil {
				return nil, errors.Wrapf(err, "read tool call arguments for tool %s", message.ToolCalls[i].FunctionName())
			}
			if err := decodeClaudeJSON([]byte(rawArguments), &inputParam); err != nil {
				return nil, errors.Wrapf(err, "unmarshal tool call arguments for tool %s", message.ToolCalls[i].FunctionName())
			}
			contents = append(contents, Content{
				Type:  "tool_use",
				Id:    message.ToolCalls[i].Id,
				Name:  message.ToolCalls[i].FunctionName(),
				Input: inputParam,
			})
		}

		// Process thinking content after content parsing
		if needsThinkingProcessing && reasoningContent != "" {
			var signatureRestored bool
			thinkingContent := Content{
				Type:     "thinking",
				Thinking: &reasoningContent,
			}

			// Try to restore signature from cache if available
			if tokenID, exists := c.Get(ctxkey.TokenId); exists {
				if tokenIDInt, ok := tokenID.(int); ok {
					tokenIDStr := getTokenIDFromRequest(tokenIDInt)
					conversationID := generateConversationID(textRequest.Messages)

					// Store conversation ID in context for later use
					c.Set(ctxkey.ConversationId, conversationID)

					// Try to restore signature for this thinking block
					messageIndex := len(claudeRequest.Messages) // Current message index
					cacheKey := generateSignatureKey(tokenIDStr, conversationID, messageIndex, 0)

					if signature := GetSignatureCache().Get(cacheKey); signature != nil {
						thinkingContent.Signature = signature
						signatureRestored = true
					}
				}
			}

			// If signature was not restored, use fallback approach
			if !signatureRestored {
				// Set fallback mode flag
				useFallbackMode = true

				// Convert thinking content to <think> format and prepend to text content
				thinkingPrefix := fmt.Sprintf("<think>%s</think>\n\n", reasoningContent)

				// Get the original response text from the message
				originalText := ""
				if message.IsStringContent() {
					originalText = message.StringContent()
				} else {
					// Try to get text from Content directly
					if str, ok := message.Content.(string); ok {
						originalText = str
					}
				}

				// Find the first text content and prepend thinking
				foundTextContent := false
				for i := range contents {
					if contents[i].Type == "text" {
						contents[i].Text = thinkingPrefix + contents[i].Text
						foundTextContent = true
						break
					}
				}

				// If no text content found, create one with thinking prefix and original text
				if !foundTextContent {
					contents = append(contents, Content{
						Type: "text",
						Text: thinkingPrefix + originalText,
					})
				}
			} else {
				// Signature was restored, use proper thinking block
				contents = append([]Content{thinkingContent}, contents...)
			}
		}

		claudeMessage.Content = contents

		// Claude requires at least one content block per message
		if len(claudeMessage.Content) == 0 {
			return nil, errors.Wrap(errors.New("message must have at least one content block"), "validate message content")
		}

		claudeRequest.Messages = append(claudeRequest.Messages, claudeMessage)
	}

	if len(systemPrompts) > 0 {
		claudeRequest.System = strings.Join(systemPrompts, "\n\n")
	}

	if systemMessageCount > 0 {
		logger.Debug("processed system messages for Anthropic conversion",
			zap.Int("system_messages_total", systemMessageCount),
			zap.Int("system_messages_merged", len(systemPrompts)),
			zap.Int("system_messages_empty", emptySystemMessageCount),
		)
	}

	// If fallback mode was used, disable thinking to avoid Claude validation errors
	if useFallbackMode && claudeRequest.Thinking != nil {
		claudeRequest.Thinking = nil
	}

	return &claudeRequest, nil
}
