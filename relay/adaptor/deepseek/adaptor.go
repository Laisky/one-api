package deepseek

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/adaptor/common/deepseekcompat"
	"github.com/songquanpeng/one-api/relay/adaptor/common/structuredjson"
	"github.com/songquanpeng/one-api/relay/adaptor/openai_compatible"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
)

type Adaptor struct {
	adaptor.DefaultPricingMethods
}

func (a *Adaptor) GetChannelName() string {
	return "deepseek"
}

func (a *Adaptor) GetModelList() []string {
	return adaptor.GetModelListFromPricing(ModelRatios)
}

// GetDefaultModelPricing returns the pricing information for DeepSeek models
// Based on official DeepSeek pricing: https://platform.deepseek.com/api-docs/pricing/
func (a *Adaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	return ModelRatios
}

// DefaultToolingConfig returns DeepSeek's provider-level tooling defaults (none published as of 2025-11-12).
func (a *Adaptor) DefaultToolingConfig() adaptor.ChannelToolConfig {
	return DeepseekToolingDefaults
}

func (a *Adaptor) GetModelRatio(modelName string) float64 {
	pricing := a.GetDefaultModelPricing()
	if price, exists := pricing[modelName]; exists {
		return price.Ratio
	}
	// Use default fallback from DefaultPricingMethods
	return a.DefaultPricingMethods.GetModelRatio(modelName)
}

func (a *Adaptor) GetCompletionRatio(modelName string) float64 {
	pricing := a.GetDefaultModelPricing()
	if price, exists := pricing[modelName]; exists {
		return price.CompletionRatio
	}
	// Use default fallback from DefaultPricingMethods
	return a.DefaultPricingMethods.GetCompletionRatio(modelName)
}

// Implement required adaptor interface methods (DeepSeek uses OpenAI-compatible API)
func (a *Adaptor) Init(meta *meta.Meta) {}

func (a *Adaptor) GetRequestURL(meta *meta.Meta) (string, error) {
	// Handle Claude Messages requests - convert to OpenAI Chat Completions endpoint
	requestPath := meta.RequestURLPath
	if idx := strings.Index(requestPath, "?"); idx >= 0 {
		requestPath = requestPath[:idx]
	}
	if requestPath == "/v1/messages" {
		// Claude Messages requests should use OpenAI's chat completions endpoint
		chatCompletionsPath := "/v1/chat/completions"
		return openai_compatible.GetFullRequestURL(meta.BaseURL, chatCompletionsPath, meta.ChannelType), nil
	}

	// DeepSeek uses OpenAI-compatible API endpoints
	return openai_compatible.GetFullRequestURL(meta.BaseURL, meta.RequestURLPath, meta.ChannelType), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) error {
	adaptor.SetupCommonRequestHeader(c, req, meta)
	req.Header.Set("Authorization", "Bearer "+meta.APIKey)
	return nil
}

func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	// DeepSeek is OpenAI-compatible, so we can pass the request through with minimal changes
	// Remove reasoning_effort as DeepSeek doesn't support it
	if request.ReasoningEffort != nil {
		request.ReasoningEffort = nil
	}

	normalizeDeepSeekThinkingConfig(c, request)

	normalizeDeepSeekToolMessageContent(c, request)

	if request.ResponseFormat != nil {
		if request.ResponseFormat.JsonSchema != nil {
			structuredjson.EnsureInstruction(request)
		}
		request.ResponseFormat = nil
	}

	return request, nil
}

// normalizeDeepSeekThinkingConfig coerces thinking.type into values accepted by DeepSeek.
// DeepSeek chat completion currently supports only enabled/disabled.
func normalizeDeepSeekThinkingConfig(c *gin.Context, request *model.GeneralOpenAIRequest) {
	if request == nil || request.Thinking == nil {
		return
	}

	originalType := request.Thinking.Type
	normalizedType, changed := deepseekcompat.NormalizeThinkingType(originalType, request.Thinking.BudgetTokens)
	if !changed {
		return
	}

	request.Thinking.Type = normalizedType
	gmw.GetLogger(c).Debug("normalized deepseek thinking type for provider compatibility",
		zap.String("model", request.Model),
		zap.String("original_type", originalType),
		zap.String("normalized_type", normalizedType),
		zap.Int("budget_tokens", request.Thinking.BudgetTokens),
	)
}

// normalizeDeepSeekToolMessageContent converts non-string tool message content into strings for DeepSeek compatibility.
// DeepSeek requires `messages[].content` for role=tool to be a string and rejects arrays/maps.
func normalizeDeepSeekToolMessageContent(c *gin.Context, request *model.GeneralOpenAIRequest) {
	lg := gmw.GetLogger(c)
	normalizedCount := 0

	for i := range request.Messages {
		message := &request.Messages[i]
		if message.Role != "tool" {
			continue
		}

		if _, ok := message.Content.(string); ok {
			continue
		}

		normalized := message.StringContent()
		if normalized == "" {
			if message.Content == nil {
				normalized = ""
			} else {
				encoded, err := json.Marshal(message.Content)
				if err != nil {
					lg.Debug("deepseek tool message fallback marshal failed",
						zap.Int("message_index", i),
						zap.String("original_content_type", fmt.Sprintf("%T", message.Content)),
						zap.Error(err),
					)
					normalized = fmt.Sprintf("%v", message.Content)
				} else {
					normalized = string(encoded)
				}
			}
		}

		message.Content = normalized
		normalizedCount++
		lg.Debug("normalized deepseek tool message content",
			zap.Int("message_index", i),
			zap.String("normalized_content_type", "string"),
			zap.Int("normalized_content_length", len(normalized)),
		)
	}

	if normalizedCount > 0 {
		lg.Debug("normalized deepseek tool messages for provider compatibility",
			zap.Int("normalized_count", normalizedCount),
			zap.Int("message_count", len(request.Messages)),
		)
	}
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, request *model.ImageRequest) (any, error) {
	return nil, errors.New("deepseek does not support image generation")
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error) {
	// Use the shared OpenAI-compatible Claude Messages conversion
	return openai_compatible.ConvertClaudeRequest(c, request)
}

func (a *Adaptor) DoRequest(c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	return adaptor.DoRequestHelper(a, c, meta, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	return openai_compatible.HandleClaudeMessagesResponse(c, resp, meta, func(c *gin.Context, resp *http.Response, promptTokens int, modelName string) (*model.ErrorWithStatusCode, *model.Usage) {
		if meta.IsStream {
			return openai_compatible.StreamHandler(c, resp, promptTokens, modelName)
		}
		return openai_compatible.Handler(c, resp, promptTokens, modelName)
	})
}
