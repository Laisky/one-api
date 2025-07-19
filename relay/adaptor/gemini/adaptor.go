package gemini

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/random"
	channelhelper "github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

type Adaptor struct {
}

func (a *Adaptor) Init(meta *meta.Meta) {
}

func (a *Adaptor) GetRequestURL(meta *meta.Meta) (string, error) {
	defaultVersion := config.GeminiVersion
	if strings.Contains(meta.ActualModelName, "gemini-2") ||
		strings.Contains(meta.ActualModelName, "gemini-1.5") ||
		strings.Contains(meta.ActualModelName, "gemma-3") {
		defaultVersion = "v1beta"
	}

	version := helper.AssignOrDefault(meta.Config.APIVersion, defaultVersion)
	action := ""
	switch meta.Mode {
	case relaymode.Embeddings:
		action = "batchEmbedContents"
	default:
		action = "generateContent"
	}

	if meta.IsStream {
		action = "streamGenerateContent?alt=sse"
	}

	return fmt.Sprintf("%s/%s/models/%s:%s", meta.BaseURL, version, meta.ActualModelName, action), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) error {
	channelhelper.SetupCommonRequestHeader(c, req, meta)
	req.Header.Set("x-goog-api-key", meta.APIKey)
	req.URL.Query().Add("key", meta.APIKey)
	return nil
}

func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	switch relayMode {
	case relaymode.Embeddings:
		geminiEmbeddingRequest := ConvertEmbeddingRequest(*request)
		return geminiEmbeddingRequest, nil
	default:
		geminiRequest := ConvertRequest(*request)
		return geminiRequest, nil
	}
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

	// Convert Claude Messages API request to OpenAI format first
	openaiRequest := &model.GeneralOpenAIRequest{
		Model:       request.Model,
		MaxTokens:   request.MaxTokens,
		Temperature: request.Temperature,
		TopP:        request.TopP,
		Stream:      request.Stream != nil && *request.Stream,
		Stop:        request.StopSequences,
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

	// Now convert using Gemini's existing logic
	return a.ConvertRequest(c, relaymode.ChatCompletions, openaiRequest)
}

func (a *Adaptor) DoRequest(c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	return channelhelper.DoRequestHelper(a, c, meta, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	// Handle Claude Messages response conversion
	if isClaudeConversion, exists := c.Get(ctxkey.ClaudeMessagesConversion); exists && isClaudeConversion.(bool) {
		claudeResp, convertErr := a.convertToClaudeResponse(c, resp, meta)
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

	if meta.IsStream {
		var responseText string
		err, responseText = StreamHandler(c, resp)
		usage = openai.ResponseText2Usage(responseText, meta.ActualModelName, meta.PromptTokens)
	} else {
		switch meta.Mode {
		case relaymode.Embeddings:
			err, usage = EmbeddingHandler(c, resp)
		default:
			err, usage = Handler(c, resp, meta.PromptTokens, meta.ActualModelName)
		}
	}
	return
}

// convertToClaudeResponse converts Gemini response format to Claude Messages format
func (a *Adaptor) convertToClaudeResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (*http.Response, *model.ErrorWithStatusCode) {
	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, openai.ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	resp.Body.Close()

	// Check if it's a streaming response
	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		// Handle streaming response conversion (not implemented yet)
		return a.convertStreamingToClaudeResponse(c, resp, body, meta)
	}

	// Handle non-streaming response conversion
	return a.convertNonStreamingToClaudeResponse(c, resp, body, meta)
}

// convertNonStreamingToClaudeResponse converts a non-streaming Gemini response to Claude format
func (a *Adaptor) convertNonStreamingToClaudeResponse(c *gin.Context, resp *http.Response, body []byte, meta *meta.Meta) (*http.Response, *model.ErrorWithStatusCode) {
	var geminiResp ChatResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
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
		ID:      fmt.Sprintf("msg_%s", random.GetRandomString(29)), // Generate a Claude-style ID
		Type:    "message",
		Role:    "assistant",
		Model:   meta.ActualModelName,
		Content: []model.ClaudeContent{},
		Usage: model.ClaudeUsage{
			InputTokens:  0, // Will be set from usage metadata
			OutputTokens: 0, // Will be set from usage metadata
		},
		StopReason: "end_turn",
	}

	// Extract usage information if available
	if geminiResp.UsageMetadata != nil {
		claudeResp.Usage.InputTokens = geminiResp.UsageMetadata.PromptTokenCount
		if geminiResp.UsageMetadata.TotalTokenCount > 0 && geminiResp.UsageMetadata.PromptTokenCount > 0 {
			claudeResp.Usage.OutputTokens = geminiResp.UsageMetadata.TotalTokenCount - geminiResp.UsageMetadata.PromptTokenCount
		}
	}

	// Convert candidates to content
	if len(geminiResp.Candidates) > 0 {
		candidate := geminiResp.Candidates[0]

		// Set stop reason based on finish reason
		switch candidate.FinishReason {
		case "STOP":
			claudeResp.StopReason = "end_turn"
		case "MAX_TOKENS":
			claudeResp.StopReason = "max_tokens"
		case "SAFETY":
			claudeResp.StopReason = "stop_sequence"
		case "RECITATION":
			claudeResp.StopReason = "stop_sequence"
		default:
			claudeResp.StopReason = "end_turn"
		}

		// Convert parts to Claude content
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				claudeResp.Content = append(claudeResp.Content, model.ClaudeContent{
					Type: "text",
					Text: part.Text,
				})
			}

			// Handle function calls if present
			if part.FunctionCall != nil {
				var input json.RawMessage
				if part.FunctionCall.Arguments != nil {
					if argsBytes, err := json.Marshal(part.FunctionCall.Arguments); err == nil {
						input = json.RawMessage(argsBytes)
					}
				}
				claudeResp.Content = append(claudeResp.Content, model.ClaudeContent{
					Type:  "tool_use",
					ID:    fmt.Sprintf("toolu_%s", random.GetRandomString(29)), // Generate a Claude-style tool ID
					Name:  part.FunctionCall.FunctionName,
					Input: input,
				})
			}
		}
	}

	// Marshal the Claude response
	claudeBody, err := json.Marshal(claudeResp)
	if err != nil {
		return nil, openai.ErrorWrapper(err, "marshal_claude_response_failed", http.StatusInternalServerError)
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

// convertStreamingToClaudeResponse converts a streaming Gemini response to Claude format
func (a *Adaptor) convertStreamingToClaudeResponse(c *gin.Context, resp *http.Response, body []byte, meta *meta.Meta) (*http.Response, *model.ErrorWithStatusCode) {
	// For now, return an error for streaming conversion as it's more complex
	// This can be implemented later if needed
	return nil, openai.ErrorWrapper(errors.New("streaming Claude Messages conversion not yet implemented for Gemini"), "streaming_conversion_not_implemented", http.StatusNotImplemented)
}

func (a *Adaptor) GetModelList() []string {
	return channelhelper.GetModelListFromPricing(ModelRatios)
}

func (a *Adaptor) GetChannelName() string {
	return "google gemini"
}

// Pricing methods - Gemini adapter manages its own model pricing
func (a *Adaptor) GetDefaultModelPricing() map[string]channelhelper.ModelConfig {
	const MilliTokensUsd = 0.000001

	// Direct map definition - much easier to maintain and edit
	// Pricing from https://ai.google.dev/pricing
	return ModelRatios
}

func (a *Adaptor) GetModelRatio(modelName string) float64 {
	pricing := a.GetDefaultModelPricing()
	if price, exists := pricing[modelName]; exists {
		return price.Ratio
	}
	// Default Gemini pricing
	return 0.5 * 0.000001 // Default USD pricing
}

func (a *Adaptor) GetCompletionRatio(modelName string) float64 {
	pricing := a.GetDefaultModelPricing()
	if price, exists := pricing[modelName]; exists {
		return price.CompletionRatio
	}
	// Default completion ratio for Gemini
	return 3.0
}
