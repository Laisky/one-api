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

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/common/deepseekcompat"
	"github.com/Laisky/one-api/relay/adaptor/common/structuredjson"
	"github.com/Laisky/one-api/relay/adaptor/common/toolnamesafe"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
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

// GetDefaultModelPricing returns current DeepSeek model pricing and capability metadata.
// Parameters: none. Returns: the official-model-ID keyed pricing configuration map.
// Source: https://api-docs.deepseek.com/quick_start/pricing/
func (a *Adaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	return ModelRatios
}

// DefaultToolingConfig returns DeepSeek's provider-level tooling defaults.
// Parameters: none. Returns: an empty separate-pricing map because built-in web
// search is billed through normal model token usage as of 2026-08-01.
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
	normalizeDeepSeekReasoningEffort(request)
	ensureDeepSeekStreamUsage(request)

	normalizeDeepSeekThinkingConfig(c, request)
	normalizeDeepSeekMessageReasoning(request)

	normalizeDeepSeekToolMessageContent(c, request)

	if rewrites := toolnamesafe.SanitizeRequestToolNames(c, request); rewrites > 0 {
		gmw.GetLogger(c).Debug("sanitized deepseek tool/function names for provider compatibility",
			zap.String("model", request.Model),
			zap.Int("rewritten_count", rewrites),
		)
	}

	if request.ResponseFormat != nil {
		if strings.EqualFold(request.ResponseFormat.Type, "json_object") && request.ResponseFormat.JsonSchema == nil {
			// DeepSeek supports the standard JSON object mode directly.
		} else if request.ResponseFormat.JsonSchema != nil {
			structuredjson.EnsureInstruction(request)
			request.ResponseFormat = nil
		} else {
			request.ResponseFormat = nil
		}
	}

	return request, nil
}

// normalizeDeepSeekReasoningEffort maps portable reasoning levels to the
// values accepted by DeepSeek and clears unsupported values.
// Parameters: request is the mutable OpenAI-style request to normalize.
// Returns: nothing; request.ReasoningEffort is updated in place.
func normalizeDeepSeekReasoningEffort(request *model.GeneralOpenAIRequest) {
	if request == nil || request.ReasoningEffort == nil {
		return
	}

	effort := strings.ToLower(strings.TrimSpace(*request.ReasoningEffort))
	switch effort {
	case "low", "high", "max":
	case "medium", "xhigh":
		// DeepSeek accepts these compatibility aliases as high.
		effort = "high"
	default:
		request.ReasoningEffort = nil
		return
	}

	request.ReasoningEffort = &effort
}

// ensureDeepSeekStreamUsage requests the upstream usage event needed to
// reconcile streaming quota estimates with authoritative token accounting.
// Parameters: request is the mutable OpenAI-style request to update.
// Returns: nothing; stream_options.include_usage is enabled when streaming.
func ensureDeepSeekStreamUsage(request *model.GeneralOpenAIRequest) {
	if request == nil || !request.Stream {
		return
	}
	if request.StreamOptions == nil {
		request.StreamOptions = &model.StreamOptions{}
	}
	request.StreamOptions.IncludeUsage = true
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
		zap.Intp("budget_tokens", request.Thinking.BudgetTokens),
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
	converted, err := openai_compatible.ConvertClaudeRequest(c, request)
	if err != nil {
		return nil, errors.Wrap(err, "convert Claude request for DeepSeek")
	}
	chatRequest, ok := converted.(*model.GeneralOpenAIRequest)
	if !ok {
		return nil, errors.Errorf("unexpected DeepSeek Claude conversion type %T", converted)
	}
	ensureDeepSeekStreamUsage(chatRequest)
	if request.Thinking != nil {
		thinking := *request.Thinking
		chatRequest.Thinking = &thinking
	}
	normalizeDeepSeekThinkingConfig(c, chatRequest)
	normalizeDeepSeekMessageReasoning(chatRequest)
	normalizeDeepSeekToolMessageContent(c, chatRequest)
	return chatRequest, nil
}

// normalizeDeepSeekMessageReasoning converts portable reasoning fields on
// assistant history into DeepSeek's reasoning_content contract. The request is
// mutated in place; existing provider-native reasoning takes precedence.
func normalizeDeepSeekMessageReasoning(request *model.GeneralOpenAIRequest) {
	if request == nil {
		return
	}

	for idx := range request.Messages {
		message := &request.Messages[idx]
		if message.Role != "assistant" {
			continue
		}

		if message.ReasoningContent == nil {
			switch {
			case message.Reasoning != nil:
				reasoning := *message.Reasoning
				message.ReasoningContent = &reasoning
			case message.Thinking != nil:
				reasoning := *message.Thinking
				message.ReasoningContent = &reasoning
			}
		}
		message.Reasoning = nil
		message.Thinking = nil

		if len(message.ToolCalls) > 0 && message.Content == nil {
			message.Content = ""
		}
	}
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
