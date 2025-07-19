package cloudflare

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

type Adaptor struct {
	meta *meta.Meta
}

// ConvertImageRequest implements adaptor.Adaptor.
func (*Adaptor) ConvertImageRequest(_ *gin.Context, request *model.ImageRequest) (any, error) {
	return nil, errors.New("not implemented")
}

// ConvertImageRequest implements adaptor.Adaptor.

func (a *Adaptor) Init(meta *meta.Meta) {
	a.meta = meta
}

// WorkerAI cannot be used across accounts with AIGateWay
// https://developers.cloudflare.com/ai-gateway/providers/workersai/#openai-compatible-endpoints
// https://gateway.ai.cloudflare.com/v1/{account_id}/{gateway_id}/workers-ai
func (a *Adaptor) isAIGateWay(baseURL string) bool {
	return strings.HasPrefix(baseURL, "https://gateway.ai.cloudflare.com") && strings.HasSuffix(baseURL, "/workers-ai")
}

func (a *Adaptor) GetRequestURL(meta *meta.Meta) (string, error) {
	isAIGateWay := a.isAIGateWay(meta.BaseURL)
	var urlPrefix string
	if isAIGateWay {
		urlPrefix = meta.BaseURL
	} else {
		urlPrefix = fmt.Sprintf("%s/client/v4/accounts/%s/ai", meta.BaseURL, meta.Config.UserID)
	}

	switch meta.Mode {
	case relaymode.ChatCompletions:
		return fmt.Sprintf("%s/v1/chat/completions", urlPrefix), nil
	case relaymode.Embeddings:
		return fmt.Sprintf("%s/v1/embeddings", urlPrefix), nil
	default:
		if isAIGateWay {
			return fmt.Sprintf("%s/%s", urlPrefix, meta.ActualModelName), nil
		}
		return fmt.Sprintf("%s/run/%s", urlPrefix, meta.ActualModelName), nil
	}
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) error {
	adaptor.SetupCommonRequestHeader(c, req, meta)
	req.Header.Set("Authorization", "Bearer "+meta.APIKey)
	return nil
}

func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	switch relayMode {
	case relaymode.Completions:
		return ConvertCompletionsRequest(*request), nil
	case relaymode.ChatCompletions, relaymode.Embeddings:
		return request, nil
	default:
		return nil, errors.New("not implemented")
	}
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

	// Now convert using Cloudflare's existing logic
	return a.ConvertRequest(c, relaymode.ChatCompletions, openaiRequest)
}

func (a *Adaptor) DoRequest(c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	return adaptor.DoRequestHelper(a, c, meta, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	if meta.IsStream {
		err, usage = StreamHandler(c, resp, meta.PromptTokens, meta.ActualModelName)
	} else {
		err, usage = Handler(c, resp, meta.PromptTokens, meta.ActualModelName)
	}
	return
}

func (a *Adaptor) GetModelList() []string {
	return adaptor.GetModelListFromPricing(ModelRatios)
}

func (a *Adaptor) GetChannelName() string {
	return "cloudflare"
}

// Pricing methods - Cloudflare adapter manages its own model pricing
func (a *Adaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	const MilliTokensUsd = 0.000001

	// Direct map definition - much easier to maintain and edit
	// Pricing from https://developers.cloudflare.com/workers-ai/platform/pricing/
	// Cloudflare Workers AI has very competitive pricing
	return map[string]adaptor.ModelConfig{
		// Meta Llama Models
		"@cf/meta/llama-3.1-8b-instruct":         {Ratio: 0.125 * MilliTokensUsd, CompletionRatio: 1}, // $0.125 per 1M tokens
		"@cf/meta/llama-2-7b-chat-fp16":          {Ratio: 0.125 * MilliTokensUsd, CompletionRatio: 1}, // $0.125 per 1M tokens
		"@cf/meta/llama-2-7b-chat-int8":          {Ratio: 0.125 * MilliTokensUsd, CompletionRatio: 1}, // $0.125 per 1M tokens
		"@cf/meta/llama-3-8b-instruct":           {Ratio: 0.125 * MilliTokensUsd, CompletionRatio: 1}, // $0.125 per 1M tokens
		"@cf/meta-llama/llama-2-7b-chat-hf-lora": {Ratio: 0.125 * MilliTokensUsd, CompletionRatio: 1}, // $0.125 per 1M tokens

		// Mistral Models
		"@cf/mistral/mistral-7b-instruct-v0.1":      {Ratio: 0.125 * MilliTokensUsd, CompletionRatio: 1}, // $0.125 per 1M tokens
		"@hf/mistralai/mistral-7b-instruct-v0.2":    {Ratio: 0.125 * MilliTokensUsd, CompletionRatio: 1}, // $0.125 per 1M tokens
		"@cf/mistral/mistral-7b-instruct-v0.2-lora": {Ratio: 0.125 * MilliTokensUsd, CompletionRatio: 1}, // $0.125 per 1M tokens
		"@hf/thebloke/mistral-7b-instruct-v0.1-awq": {Ratio: 0.125 * MilliTokensUsd, CompletionRatio: 1}, // $0.125 per 1M tokens

		// DeepSeek Models
		"@hf/thebloke/deepseek-coder-6.7b-base-awq":     {Ratio: 0.125 * MilliTokensUsd, CompletionRatio: 1}, // $0.125 per 1M tokens
		"@hf/thebloke/deepseek-coder-6.7b-instruct-awq": {Ratio: 0.125 * MilliTokensUsd, CompletionRatio: 1}, // $0.125 per 1M tokens
		"@cf/deepseek-ai/deepseek-math-7b-base":         {Ratio: 0.125 * MilliTokensUsd, CompletionRatio: 1}, // $0.125 per 1M tokens
		"@cf/deepseek-ai/deepseek-math-7b-instruct":     {Ratio: 0.125 * MilliTokensUsd, CompletionRatio: 1}, // $0.125 per 1M tokens

		// Other Models
		"@cf/thebloke/discolm-german-7b-v1-awq":      {Ratio: 0.125 * MilliTokensUsd, CompletionRatio: 1}, // $0.125 per 1M tokens
		"@cf/tiiuae/falcon-7b-instruct":              {Ratio: 0.125 * MilliTokensUsd, CompletionRatio: 1}, // $0.125 per 1M tokens
		"@cf/google/gemma-2b-it-lora":                {Ratio: 0.125 * MilliTokensUsd, CompletionRatio: 1}, // $0.125 per 1M tokens
		"@hf/google/gemma-7b-it":                     {Ratio: 0.125 * MilliTokensUsd, CompletionRatio: 1}, // $0.125 per 1M tokens
		"@cf/google/gemma-7b-it-lora":                {Ratio: 0.125 * MilliTokensUsd, CompletionRatio: 1}, // $0.125 per 1M tokens
		"@hf/nousresearch/hermes-2-pro-mistral-7b":   {Ratio: 0.125 * MilliTokensUsd, CompletionRatio: 1}, // $0.125 per 1M tokens
		"@hf/thebloke/llama-2-13b-chat-awq":          {Ratio: 0.125 * MilliTokensUsd, CompletionRatio: 1}, // $0.125 per 1M tokens
		"@hf/thebloke/llamaguard-7b-awq":             {Ratio: 0.125 * MilliTokensUsd, CompletionRatio: 1}, // $0.125 per 1M tokens
		"@hf/thebloke/neural-chat-7b-v3-1-awq":       {Ratio: 0.125 * MilliTokensUsd, CompletionRatio: 1}, // $0.125 per 1M tokens
		"@cf/openchat/openchat-3.5-0106":             {Ratio: 0.125 * MilliTokensUsd, CompletionRatio: 1}, // $0.125 per 1M tokens
		"@hf/thebloke/openhermes-2.5-mistral-7b-awq": {Ratio: 0.125 * MilliTokensUsd, CompletionRatio: 1}, // $0.125 per 1M tokens
		"@cf/microsoft/phi-2":                        {Ratio: 0.125 * MilliTokensUsd, CompletionRatio: 1}, // $0.125 per 1M tokens

		// Qwen Models
		"@cf/qwen/qwen1.5-0.5b-chat":    {Ratio: 0.125 * MilliTokensUsd, CompletionRatio: 1}, // $0.125 per 1M tokens
		"@cf/qwen/qwen1.5-1.8b-chat":    {Ratio: 0.125 * MilliTokensUsd, CompletionRatio: 1}, // $0.125 per 1M tokens
		"@cf/qwen/qwen1.5-14b-chat-awq": {Ratio: 0.125 * MilliTokensUsd, CompletionRatio: 1}, // $0.125 per 1M tokens
		"@cf/qwen/qwen1.5-7b-chat-awq":  {Ratio: 0.125 * MilliTokensUsd, CompletionRatio: 1}, // $0.125 per 1M tokens

		// Specialized Models
		"@cf/defog/sqlcoder-7b-2":                {Ratio: 0.125 * MilliTokensUsd, CompletionRatio: 1}, // $0.125 per 1M tokens
		"@hf/nexusflow/starling-lm-7b-beta":      {Ratio: 0.125 * MilliTokensUsd, CompletionRatio: 1}, // $0.125 per 1M tokens
		"@cf/tinyllama/tinyllama-1.1b-chat-v1.0": {Ratio: 0.125 * MilliTokensUsd, CompletionRatio: 1}, // $0.125 per 1M tokens
		"@hf/thebloke/zephyr-7b-beta-awq":        {Ratio: 0.125 * MilliTokensUsd, CompletionRatio: 1}, // $0.125 per 1M tokens
	}
}

func (a *Adaptor) GetModelRatio(modelName string) float64 {
	pricing := a.GetDefaultModelPricing()
	if price, exists := pricing[modelName]; exists {
		return price.Ratio
	}
	// Default Cloudflare pricing
	return 0.125 * 0.000001 // Default USD pricing
}

func (a *Adaptor) GetCompletionRatio(modelName string) float64 {
	pricing := a.GetDefaultModelPricing()
	if price, exists := pricing[modelName]; exists {
		return price.CompletionRatio
	}
	// Default completion ratio for Cloudflare
	return 1.0
}
