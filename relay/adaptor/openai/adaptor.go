package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/logger"
	dbmodel "github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/adaptor/alibailian"
	"github.com/songquanpeng/one-api/relay/adaptor/baiduv2"
	"github.com/songquanpeng/one-api/relay/adaptor/doubao"
	"github.com/songquanpeng/one-api/relay/adaptor/geminiOpenaiCompatible"
	"github.com/songquanpeng/one-api/relay/adaptor/minimax"
	"github.com/songquanpeng/one-api/relay/adaptor/novita"
	"github.com/songquanpeng/one-api/relay/billing/ratio"
	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

type Adaptor struct {
	ChannelType int
}

func (a *Adaptor) Init(meta *meta.Meta) {
	a.ChannelType = meta.ChannelType
}

func (a *Adaptor) GetRequestURL(meta *meta.Meta) (string, error) {
	switch meta.ChannelType {
	case channeltype.Azure:
		defaultVersion := meta.Config.APIVersion

		// https://learn.microsoft.com/en-us/azure/ai-services/openai/how-to/reasoning?tabs=python#api--feature-support
		if strings.HasPrefix(meta.ActualModelName, "o1") ||
			strings.HasPrefix(meta.ActualModelName, "o3") {
			defaultVersion = "2024-12-01-preview"
		}

		if meta.Mode == relaymode.ImagesGenerations {
			// https://learn.microsoft.com/en-us/azure/ai-services/openai/dall-e-quickstart?tabs=dalle3%2Ccommand-line&pivots=rest-api
			// https://{resource_name}.openai.azure.com/openai/deployments/dall-e-3/images/generations?api-version=2024-03-01-preview
			fullRequestURL := fmt.Sprintf("%s/openai/deployments/%s/images/generations?api-version=%s", meta.BaseURL, meta.ActualModelName, defaultVersion)
			return fullRequestURL, nil
		}

		// https://learn.microsoft.com/en-us/azure/cognitive-services/openai/chatgpt-quickstart?pivots=rest-api&tabs=command-line#rest-api
		requestURL := strings.Split(meta.RequestURLPath, "?")[0]
		requestURL = fmt.Sprintf("%s?api-version=%s", requestURL, defaultVersion)
		task := strings.TrimPrefix(requestURL, "/v1/")
		model_ := meta.ActualModelName
		// https://github.com/songquanpeng/one-api/issues/2235
		// model_ = strings.Replace(model_, ".", "", -1)
		//https://github.com/songquanpeng/one-api/issues/1191
		// {your endpoint}/openai/deployments/{your azure_model}/chat/completions?api-version={api_version}
		requestURL = fmt.Sprintf("/openai/deployments/%s/%s", model_, task)
		return GetFullRequestURL(meta.BaseURL, requestURL, meta.ChannelType), nil
	case channeltype.Minimax:
		return minimax.GetRequestURL(meta)
	case channeltype.Doubao:
		return doubao.GetRequestURL(meta)
	case channeltype.Novita:
		return novita.GetRequestURL(meta)
	case channeltype.BaiduV2:
		return baiduv2.GetRequestURL(meta)
	case channeltype.AliBailian:
		return alibailian.GetRequestURL(meta)
	case channeltype.GeminiOpenAICompatible:
		return geminiOpenaiCompatible.GetRequestURL(meta)
	default:
		// Handle Claude Messages requests - convert to OpenAI Chat Completions endpoint
		if meta.RequestURLPath == "/v1/messages" {
			// Claude Messages requests should use OpenAI's chat completions endpoint
			chatCompletionsPath := "/v1/chat/completions"
			return GetFullRequestURL(meta.BaseURL, chatCompletionsPath, meta.ChannelType), nil
		}

		// Convert chat completions to responses API for OpenAI only
		// Skip conversion for models that only support ChatCompletion API
		if meta.Mode == relaymode.ChatCompletions &&
			meta.ChannelType == channeltype.OpenAI &&
			!IsModelsOnlySupportedByChatCompletionAPI(meta.ActualModelName) {
			responseAPIPath := "/v1/responses"
			return GetFullRequestURL(meta.BaseURL, responseAPIPath, meta.ChannelType), nil
		}
		return GetFullRequestURL(meta.BaseURL, meta.RequestURLPath, meta.ChannelType), nil
	}
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) error {
	adaptor.SetupCommonRequestHeader(c, req, meta)
	if meta.ChannelType == channeltype.Azure {
		req.Header.Set("api-key", meta.APIKey)
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+meta.APIKey)
	if meta.ChannelType == channeltype.OpenRouter {
		req.Header.Set("HTTP-Referer", "https://github.com/Laisky/one-api")
		req.Header.Set("X-Title", "One API")
	}
	return nil
}

func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}

	meta := meta.GetByContext(c)

	// Handle direct Response API requests
	if relayMode == relaymode.ResponseAPI {
		// For direct Response API requests, the request should already be in the correct format
		// We don't need to convert it, just pass it through
		return request, nil
	}

	// Convert ChatCompletion requests to Response API format only for OpenAI
	// Skip conversion for models that only support ChatCompletion API
	if relayMode == relaymode.ChatCompletions &&
		meta.ChannelType == channeltype.OpenAI &&
		!IsModelsOnlySupportedByChatCompletionAPI(meta.ActualModelName) {
		// Apply existing transformations first
		if err := a.applyRequestTransformations(meta, request); err != nil {
			return nil, err
		}

		// Convert to Response API format
		responseAPIRequest := ConvertChatCompletionToResponseAPI(request)

		// Store the converted request in context to detect it later in DoResponse
		c.Set(ctxkey.ConvertedRequest, responseAPIRequest)

		return responseAPIRequest, nil
	}

	// Apply existing transformations for other modes
	if err := a.applyRequestTransformations(meta, request); err != nil {
		return nil, err
	}

	return request, nil
}

// applyRequestTransformations applies the existing request transformations
func (a *Adaptor) applyRequestTransformations(meta *meta.Meta, request *model.GeneralOpenAIRequest) error {
	switch meta.ChannelType {
	case channeltype.OpenRouter:
		includeReasoning := true
		request.IncludeReasoning = &includeReasoning
		if request.Provider == nil || request.Provider.Sort == "" &&
			config.OpenrouterProviderSort != "" {
			if request.Provider == nil {
				request.Provider = &model.RequestProvider{}
			}

			request.Provider.Sort = config.OpenrouterProviderSort
		}
	default:
	}

	if request.Stream && !config.EnforceIncludeUsage {
		logger.Warn(context.Background(),
			"please set ENFORCE_INCLUDE_USAGE=true to ensure accurate billing in stream mode")
	}

	if config.EnforceIncludeUsage && request.Stream {
		// always return usage in stream mode
		if request.StreamOptions == nil {
			request.StreamOptions = &model.StreamOptions{}
		}
		request.StreamOptions.IncludeUsage = true
	}

	// o1/o3/o4 do not support system prompt/max_tokens/temperature
	if strings.HasPrefix(meta.ActualModelName, "o") {
		temperature := float64(1)
		request.Temperature = &temperature // Only the default (1) value is supported
		request.MaxTokens = 0
		request.TopP = nil
		if request.ReasoningEffort == nil {
			effortHigh := "high"
			request.ReasoningEffort = &effortHigh
		}

		request.Messages = func(raw []model.Message) (filtered []model.Message) {
			for i := range raw {
				if raw[i].Role != "system" {
					filtered = append(filtered, raw[i])
				}
			}

			return
		}(request.Messages)
	} else {
		request.ReasoningEffort = nil
	}

	// web search do not support system prompt/max_tokens/temperature
	if strings.HasSuffix(meta.ActualModelName, "-search") {
		request.Temperature = nil
		request.TopP = nil
		request.PresencePenalty = nil
		request.N = nil
		request.FrequencyPenalty = nil
	}

	if request.Stream && !config.EnforceIncludeUsage &&
		strings.HasSuffix(request.Model, "-audio") {
		// TODO: Since it is not clear how to implement billing in stream mode,
		// it is temporarily not supported
		return errors.New("set ENFORCE_INCLUDE_USAGE=true to enable stream mode for gpt-4o-audio")
	}

	return nil
}

func (a *Adaptor) ConvertImageRequest(_ *gin.Context, request *model.ImageRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return request, nil
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}

	// Convert Claude Messages API request to OpenAI Chat Completions format
	openaiRequest := &model.GeneralOpenAIRequest{
		Model:       request.Model,
		MaxTokens:   request.MaxTokens,
		Temperature: request.Temperature,
		TopP:        request.TopP,
		Stream:      request.Stream != nil && *request.Stream,
		Stop:        request.StopSequences,
		Thinking:    request.Thinking,
	}

	// Convert system prompt
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
				openaiRequest.Messages = append(openaiRequest.Messages, model.Message{
					Role:    "system",
					Content: systemText,
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
							if source, exists := blockMap["source"]; exists {
								if sourceMap, ok := source.(map[string]any); ok {
									imageURL := model.ImageURL{}
									if mediaType, exists := sourceMap["media_type"]; exists {
										if data, exists := sourceMap["data"]; exists {
											if dataStr, ok := data.(string); ok {
												// Convert to data URL format
												imageURL.Url = fmt.Sprintf("data:%s;base64,%s", mediaType, dataStr)
											}
										}
									}
									contentParts = append(contentParts, model.MessageContent{
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

	// Convert tools
	for _, tool := range request.Tools {
		openaiTool := model.Tool{
			Type: "function",
			Function: model.Function{
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

		openaiRequest.Tools = append(openaiRequest.Tools, openaiTool)
	}

	// Convert tool choice
	if request.ToolChoice != nil {
		openaiRequest.ToolChoice = request.ToolChoice
	}

	// Mark this as a Claude Messages conversion for response handling
	c.Set(ctxkey.ClaudeMessagesConversion, true)
	c.Set(ctxkey.OriginalClaudeRequest, request)

	// For OpenAI adaptor, we can return the OpenAI request directly
	// since OpenAI format is the native format
	return openaiRequest, nil
}

func (a *Adaptor) DoRequest(c *gin.Context,
	meta *meta.Meta,
	requestBody io.Reader) (*http.Response, error) {
	return adaptor.DoRequestHelper(a, c, meta, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context,
	resp *http.Response,
	meta *meta.Meta) (usage *model.Usage,
	err *model.ErrorWithStatusCode) {
	if meta.IsStream {
		var responseText string
		// Handle different streaming modes
		switch meta.Mode {
		case relaymode.ResponseAPI:
			// Direct Response API streaming - pass through without conversion
			err, responseText, usage = ResponseAPIDirectStreamHandler(c, resp, meta.Mode)
		default:
			// Check if we need to handle Response API streaming response for ChatCompletion
			if vi, ok := c.Get(ctxkey.ConvertedRequest); ok {
				if _, ok := vi.(*ResponseAPIRequest); ok {
					// This is a Response API streaming response that needs conversion
					err, responseText, usage = ResponseAPIStreamHandler(c, resp, meta.Mode)
				} else {
					// Regular streaming response
					err, responseText, usage = StreamHandler(c, resp, meta.Mode)
				}
			} else {
				// Regular streaming response
				err, responseText, usage = StreamHandler(c, resp, meta.Mode)
			}
		}

		if usage == nil || usage.TotalTokens == 0 {
			usage = ResponseText2Usage(responseText, meta.ActualModelName, meta.PromptTokens)
		}
		if usage.TotalTokens != 0 && usage.PromptTokens == 0 { // some channels don't return prompt tokens & completion tokens
			usage.PromptTokens = meta.PromptTokens
			usage.CompletionTokens = usage.TotalTokens - meta.PromptTokens
		}
	} else {
		switch meta.Mode {
		case relaymode.ImagesGenerations,
			relaymode.ImagesEdits:
			err, usage = ImageHandler(c, resp)
		// case relaymode.ImagesEdits:
		// err, usage = ImagesEditsHandler(c, resp)
		case relaymode.ResponseAPI:
			// For direct Response API requests, pass through the response directly
			// without conversion back to ChatCompletion format
			err, usage = ResponseAPIDirectHandler(c, resp, meta.PromptTokens, meta.ActualModelName)
		case relaymode.ChatCompletions:
			// Check if we need to convert Response API response back to ChatCompletion format
			if vi, ok := c.Get(ctxkey.ConvertedRequest); ok {
				if _, ok := vi.(*ResponseAPIRequest); ok {
					// This is a Response API response that needs conversion
					err, usage = ResponseAPIHandler(c, resp, meta.PromptTokens, meta.ActualModelName)
				} else {
					// Regular ChatCompletion request
					err, usage = Handler(c, resp, meta.PromptTokens, meta.ActualModelName)
				}
			} else {
				// Regular ChatCompletion request
				err, usage = Handler(c, resp, meta.PromptTokens, meta.ActualModelName)
			}
		default:
			err, usage = Handler(c, resp, meta.PromptTokens, meta.ActualModelName)
		}
	}

	// -------------------------------------
	// calculate web-search tool cost
	// -------------------------------------
	if usage != nil {
		searchContextSize := "medium"
		var req *model.GeneralOpenAIRequest
		if vi, ok := c.Get(ctxkey.ConvertedRequest); ok {
			if req, ok = vi.(*model.GeneralOpenAIRequest); ok {
				if req != nil &&
					req.WebSearchOptions != nil &&
					req.WebSearchOptions.SearchContextSize != nil {
					searchContextSize = *req.WebSearchOptions.SearchContextSize
				}

				switch {
				case strings.HasPrefix(meta.ActualModelName, "gpt-4o-search"):
					switch searchContextSize {
					case "low":
						usage.ToolsCost += int64(math.Ceil(30 / 1000 * ratio.QuotaPerUsd))
					case "medium":
						usage.ToolsCost += int64(math.Ceil(35 / 1000 * ratio.QuotaPerUsd))
					case "high":
						usage.ToolsCost += int64(math.Ceil(40 / 1000 * ratio.QuotaPerUsd))
					default:
						return nil, ErrorWrapper(
							errors.Errorf("invalid search context size %q", searchContextSize),
							"invalid search context size: "+searchContextSize,
							http.StatusBadRequest)
					}
				case strings.HasPrefix(meta.ActualModelName, "gpt-4o-mini-search"):
					switch searchContextSize {
					case "low":
						usage.ToolsCost += int64(math.Ceil(25 / 1000 * ratio.QuotaPerUsd))
					case "medium":
						usage.ToolsCost += int64(math.Ceil(27.5 / 1000 * ratio.QuotaPerUsd))
					case "high":
						usage.ToolsCost += int64(math.Ceil(30 / 1000 * ratio.QuotaPerUsd))
					default:
						return nil, ErrorWrapper(
							errors.Errorf("invalid search context size %q", searchContextSize),
							"invalid search context size: "+searchContextSize,
							http.StatusBadRequest)
					}
				}

				// -------------------------------------
				// calculate structured output cost
				// -------------------------------------
				// Structured output with json_schema incurs additional costs
				// Based on OpenAI's pricing, structured output typically has a multiplier applied
				if req.ResponseFormat != nil &&
					req.ResponseFormat.Type == "json_schema" &&
					req.ResponseFormat.JsonSchema != nil {
					// Apply structured output cost multiplier
					// For structured output, there's typically an additional cost based on completion tokens
					// Using a conservative estimate of 25% additional cost for structured output

					// get channel-specific pricing if available
					var channelModelRatio map[string]float64
					if channelModel, ok := c.Get(ctxkey.ChannelModel); ok {
						if channel, ok := channelModel.(*dbmodel.Channel); ok {
							channelModelRatio = channel.GetModelRatio()
						}
					}

					modelRatio := ratio.GetModelRatioWithChannel(meta.ActualModelName, meta.ChannelType, channelModelRatio)
					structuredOutputCost := int64(math.Ceil(float64(usage.CompletionTokens) * 0.25 * modelRatio))
					usage.ToolsCost += structuredOutputCost

					// Log structured output cost application for debugging
					logger.Debugf(c.Request.Context(), "Applied structured output cost: %d (completion tokens: %d, model: %s)",
						structuredOutputCost, usage.CompletionTokens, meta.ActualModelName)
				}
			}
		}

		// Also check the original request in case it wasn't converted
		if req == nil {
			if vi, ok := c.Get(ctxkey.RequestModel); ok {
				if req, ok = vi.(*model.GeneralOpenAIRequest); ok && req != nil {
					if req.ResponseFormat != nil &&
						req.ResponseFormat.Type == "json_schema" &&
						req.ResponseFormat.JsonSchema != nil {
						// Apply structured output cost multiplier

						// get channel-specific pricing if available
						var channelModelRatio map[string]float64
						if channelModel, ok := c.Get(ctxkey.ChannelModel); ok {
							if channel, ok := channelModel.(*dbmodel.Channel); ok {
								// Get from unified ModelConfigs only (after migration)
								channelModelRatio = channel.GetModelRatioFromConfigs()
							}
						}

						modelRatio := ratio.GetModelRatioWithChannel(meta.ActualModelName, meta.ChannelType, channelModelRatio)
						structuredOutputCost := int64(math.Ceil(float64(usage.CompletionTokens) * 0.25 * modelRatio))
						usage.ToolsCost += structuredOutputCost

						// Log structured output cost application for debugging
						logger.Debugf(c.Request.Context(), "Applied structured output cost from original request: %d (completion tokens: %d, model: %s)",
							structuredOutputCost, usage.CompletionTokens, meta.ActualModelName)
					}
				}
			}
		}
	}

	// Handle Claude Messages response conversion
	if isClaudeConversion, exists := c.Get(ctxkey.ClaudeMessagesConversion); exists && isClaudeConversion.(bool) {
		claudeResp, convertErr := a.convertToClaudeResponse(c, resp)
		if convertErr != nil {
			return nil, convertErr
		}

		// Replace the original response with the converted Claude response
		// We need to update the response in the context so the controller can use it
		c.Set(ctxkey.ConvertedResponse, claudeResp)

		// For Claude Messages conversion, we don't return usage separately
		// The usage is included in the Claude response body, so return nil usage
		return nil, nil
	}

	return
}

// convertToClaudeResponse converts OpenAI response format to Claude Messages format
func (a *Adaptor) convertToClaudeResponse(c *gin.Context, resp *http.Response) (*http.Response, *model.ErrorWithStatusCode) {
	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	resp.Body.Close()

	// Check if it's a streaming response
	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		// Handle streaming response conversion
		return a.convertStreamingToClaudeResponse(c, resp, body)
	}

	// Handle non-streaming response conversion
	return a.convertNonStreamingToClaudeResponse(c, resp, body)
}

// convertNonStreamingToClaudeResponse converts a non-streaming OpenAI response to Claude format
func (a *Adaptor) convertNonStreamingToClaudeResponse(c *gin.Context, resp *http.Response, body []byte) (*http.Response, *model.ErrorWithStatusCode) {
	var openaiResp TextResponse
	if err := json.Unmarshal(body, &openaiResp); err != nil {
		// If it's an error response, pass it through
		newResp := &http.Response{
			StatusCode: resp.StatusCode,
			Header:     resp.Header,
			Body:       io.NopCloser(bytes.NewReader(body)),
		}
		return newResp, nil
	}

	// Convert to Claude Messages format
	claudeResp := model.ClaudeResponse{
		ID:      openaiResp.Id,
		Type:    "message",
		Role:    "assistant",
		Model:   openaiResp.Model,
		Content: []model.ClaudeContent{},
		Usage: model.ClaudeUsage{
			InputTokens:  openaiResp.Usage.PromptTokens,
			OutputTokens: openaiResp.Usage.CompletionTokens,
		},
		StopReason: "end_turn",
	}

	// Convert choices to content
	for _, choice := range openaiResp.Choices {
		if choice.Message.Content != nil {
			switch content := choice.Message.Content.(type) {
			case string:
				claudeResp.Content = append(claudeResp.Content, model.ClaudeContent{
					Type: "text",
					Text: content,
				})
			case []model.MessageContent:
				// Handle structured content
				for _, part := range content {
					if part.Type == "text" && part.Text != nil {
						claudeResp.Content = append(claudeResp.Content, model.ClaudeContent{
							Type: "text",
							Text: *part.Text,
						})
					}
				}
			}
		}

		// Handle tool calls
		if len(choice.Message.ToolCalls) > 0 {
			for _, toolCall := range choice.Message.ToolCalls {
				var input json.RawMessage
				if toolCall.Function.Arguments != nil {
					if argsStr, ok := toolCall.Function.Arguments.(string); ok {
						input = json.RawMessage(argsStr)
					} else if argsBytes, err := json.Marshal(toolCall.Function.Arguments); err == nil {
						input = json.RawMessage(argsBytes)
					}
				}
				claudeResp.Content = append(claudeResp.Content, model.ClaudeContent{
					Type:  "tool_use",
					ID:    toolCall.Id,
					Name:  toolCall.Function.Name,
					Input: input,
				})
			}
		}

		// Set stop reason based on finish reason
		switch choice.FinishReason {
		case "stop":
			claudeResp.StopReason = "end_turn"
		case "length":
			claudeResp.StopReason = "max_tokens"
		case "tool_calls":
			claudeResp.StopReason = "tool_use"
		case "content_filter":
			claudeResp.StopReason = "stop_sequence"
		}
	}

	// Marshal the Claude response
	claudeBody, err := json.Marshal(claudeResp)
	if err != nil {
		return nil, ErrorWrapper(err, "marshal_claude_response_failed", http.StatusInternalServerError)
	}

	// Create new response with Claude format
	newResp := &http.Response{
		StatusCode: resp.StatusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(claudeBody)),
	}

	// Copy headers but update content type
	for k, v := range resp.Header {
		newResp.Header[k] = v
	}
	newResp.Header.Set("Content-Type", "application/json")
	newResp.Header.Set("Content-Length", fmt.Sprintf("%d", len(claudeBody)))

	return newResp, nil
}

// convertStreamingToClaudeResponse converts a streaming OpenAI response to Claude format
func (a *Adaptor) convertStreamingToClaudeResponse(c *gin.Context, resp *http.Response, body []byte) (*http.Response, *model.ErrorWithStatusCode) {
	// For streaming responses, we need to convert each SSE event
	// This is more complex and would require parsing SSE events and converting them
	// For now, we'll create a simple streaming converter

	reader, writer := io.Pipe()

	go func() {
		defer writer.Close()

		// Parse SSE events from the body and convert them
		scanner := bufio.NewScanner(bytes.NewReader(body))
		for scanner.Scan() {
			line := scanner.Text()

			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")

				if data == "[DONE]" {
					// Send Claude-style done event
					writer.Write([]byte("event: message_stop\n"))
					writer.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
					break
				}

				// Parse OpenAI streaming chunk
				var chunk ChatCompletionsStreamResponse
				if err := json.Unmarshal([]byte(data), &chunk); err != nil {
					continue
				}

				// Convert to Claude streaming format
				if len(chunk.Choices) > 0 {
					choice := chunk.Choices[0]
					if choice.Delta.Content != nil {
						claudeChunk := map[string]interface{}{
							"type":  "content_block_delta",
							"index": 0,
							"delta": map[string]interface{}{
								"type": "text_delta",
								"text": choice.Delta.Content,
							},
						}

						claudeData, _ := json.Marshal(claudeChunk)
						writer.Write([]byte("event: content_block_delta\n"))
						writer.Write([]byte(fmt.Sprintf("data: %s\n\n", claudeData)))
					}
				}
			}
		}
	}()

	// Create new streaming response
	newResp := &http.Response{
		StatusCode: resp.StatusCode,
		Header:     make(http.Header),
		Body:       reader,
	}

	// Copy headers
	for k, v := range resp.Header {
		newResp.Header[k] = v
	}

	return newResp, nil
}

func (a *Adaptor) GetModelList() []string {
	return adaptor.GetModelListFromPricing(ModelRatios)
}

func (a *Adaptor) GetChannelName() string {
	channelName, _ := GetCompatibleChannelMeta(a.ChannelType)
	return channelName
}

// Pricing methods - OpenAI adapter manages its own model pricing
func (a *Adaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	return ModelRatios
}

func (a *Adaptor) GetModelRatio(modelName string) float64 {
	pricing := a.GetDefaultModelPricing()
	if price, exists := pricing[modelName]; exists {
		return price.Ratio
	}
	// Fallback to global pricing for unknown models
	return ratio.GetModelRatio(modelName, a.ChannelType)
}

func (a *Adaptor) GetCompletionRatio(modelName string) float64 {
	pricing := a.GetDefaultModelPricing()
	if price, exists := pricing[modelName]; exists {
		return price.CompletionRatio
	}
	// Fallback to global pricing for unknown models
	return ratio.GetCompletionRatio(modelName, a.ChannelType)
}
