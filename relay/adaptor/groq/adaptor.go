package groq

import (
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/adaptor/openai_compatible"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
)

type Adaptor struct {
	adaptor.DefaultPricingMethods
}

type groqUnsupportedContent struct {
	messageIndex int
	role         string
	contentTypes []string
}

func (a *Adaptor) GetChannelName() string {
	return "groq"
}

func (a *Adaptor) GetModelList() []string {
	return adaptor.GetModelListFromPricing(ModelRatios)
}

// GetDefaultModelPricing returns the pricing information for Groq models
// Based on Groq pricing: https://groq.com/pricing/
func (a *Adaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	return ModelRatios
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

// DefaultToolingConfig returns Groq's built-in tool pricing defaults.
func (a *Adaptor) DefaultToolingConfig() adaptor.ChannelToolConfig {
	return GroqToolingDefaults
}

// Implement required adaptor interface methods (Groq uses OpenAI-compatible API)
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

	// Groq uses OpenAI-compatible API endpoints
	return openai_compatible.GetFullRequestURL(meta.BaseURL, meta.RequestURLPath, meta.ChannelType), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) error {
	adaptor.SetupCommonRequestHeader(c, req, meta)
	req.Header.Set("Authorization", "Bearer "+meta.APIKey)
	return nil
}

// ConvertRequest converts a GeneralOpenAIRequest into a Groq-compatible request.
//
//   - reasoning: https://console.groq.com/docs/reasoning
func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	// Groq is OpenAI-compatible, so we can pass the request through with minimal changes
	logger := gmw.GetLogger(c)

	// Groq's OpenAI-compatible chat/completions endpoint does NOT accept the
	// Responses-API-style `reasoning` object, but it DOES accept `reasoning_effort`
	// for supported models (e.g. GPT-OSS).
	//
	// When requests come from the Response API fallback path, we may have
	// `reasoning.effort` populated without `reasoning_effort`. Translate it so
	// Groq can honor the user's intent.
	var promotedEffort bool
	if request.ReasoningEffort == nil && request.Reasoning != nil && request.Reasoning.Effort != nil {
		request.ReasoningEffort = request.Reasoning.Effort
		promotedEffort = true
	}

	// Normalize/guard: Groq GPT-OSS reasoning_effort supports {low, medium, high}.
	// If callers send unsupported values (e.g. "minimal" from GPT-5 semantics),
	// drop it to avoid upstream 400s.
	if request.ReasoningEffort != nil {
		val := strings.ToLower(strings.TrimSpace(*request.ReasoningEffort))
		if val != "low" && val != "medium" && val != "high" {
			logger.Debug("dropping unsupported groq reasoning_effort",
				zap.String("model", request.Model),
				zap.String("reasoning_effort", val),
			)
			request.ReasoningEffort = nil
		}
	}

	if request.Reasoning != nil {
		logger.Debug("dropping unsupported groq request field",
			zap.String("model", request.Model),
			zap.String("field", "reasoning"),
			zap.Bool("promoted_effort", promotedEffort),
		)
		request.Reasoning = nil
	}

	// GPT-OSS on Groq accepts text-only chat content. Reject image/audio parts early
	// so callers get a deterministic 4xx with actionable guidance.
	if isGroqTextOnlyModel(request.Model) {
		if unsupported := firstUnsupportedGroqContent(request.Messages); unsupported != nil {
			logger.Debug("rejecting unsupported groq multimodal request content",
				zap.String("model", request.Model),
				zap.Int("message_index", unsupported.messageIndex),
				zap.String("message_role", unsupported.role),
				zap.Strings("content_types", unsupported.contentTypes),
			)
			return nil, errors.Errorf(
				"validation failed: groq model %q only supports text content in chat messages; messages[%d] (role=%q) contains unsupported content types: %s",
				request.Model,
				unsupported.messageIndex,
				unsupported.role,
				strings.Join(unsupported.contentTypes, ","),
			)
		}
	}

	request.TopK = nil // Groq does not support TopK

	return request, nil
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, request *model.ImageRequest) (any, error) {
	return nil, errors.New("groq does not support image generation")
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error) {
	// Use the shared OpenAI-compatible Claude Messages conversion
	return openai_compatible.ConvertClaudeRequest(c, request)
}

func (a *Adaptor) DoRequest(c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	// Log request details for debugging
	logger := gmw.GetLogger(c)
	logger.Debug("sending request to groq",
		zap.String("model", meta.ActualModelName),
		zap.String("url_path", meta.RequestURLPath),
		zap.Bool("is_stream", meta.IsStream))

	return adaptor.DoRequestHelper(a, c, meta, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	// Add logging for debugging
	logger := gmw.GetLogger(c)
	logger.Debug("processing groq response",
		zap.String("model", meta.ActualModelName),
		zap.Bool("is_stream", meta.IsStream),
		zap.Int("status_code", resp.StatusCode),
		zap.String("content_type", resp.Header.Get("Content-Type")))

	return openai_compatible.HandleClaudeMessagesResponse(c, resp, meta, func(c *gin.Context, resp *http.Response, promptTokens int, modelName string) (*model.ErrorWithStatusCode, *model.Usage) {
		if meta.IsStream {
			return openai_compatible.StreamHandler(c, resp, promptTokens, modelName)
		}
		return openai_compatible.Handler(c, resp, promptTokens, modelName)
	})
}

// isGroqTextOnlyModel reports whether the target Groq model currently accepts text-only
// chat content blocks.
func isGroqTextOnlyModel(modelName string) bool {
	normalized := strings.ToLower(strings.TrimSpace(modelName))
	return strings.HasPrefix(normalized, "openai/gpt-oss")
}

// firstUnsupportedGroqContent finds the first message that includes non-text content
// parts for models that require text-only content.
func firstUnsupportedGroqContent(messages []model.Message) *groqUnsupportedContent {
	for idx, msg := range messages {
		contentTypes := nonTextGroqContentTypes(msg.Content)
		if len(contentTypes) == 0 {
			continue
		}
		role := strings.TrimSpace(msg.Role)
		if role == "" {
			role = "unknown"
		}
		return &groqUnsupportedContent{
			messageIndex: idx,
			role:         role,
			contentTypes: contentTypes,
		}
	}

	return nil
}

// nonTextGroqContentTypes returns non-text content types observed in a message content
// payload, deduplicated and sorted for stable logging/error output.
func nonTextGroqContentTypes(content any) []string {
	var nonText []string

	addNonText := func(partType string) {
		normalized := normalizeGroqContentType(partType)
		if normalized == "" || normalized == model.ContentTypeText {
			return
		}
		nonText = append(nonText, normalized)
	}

	switch typed := content.(type) {
	case nil, string:
		// text-only by definition.
	case []model.MessageContent:
		for _, part := range typed {
			partType := strings.TrimSpace(part.Type)
			switch {
			case partType == "" && part.Text != nil:
				addNonText(model.ContentTypeText)
			case partType == "" && part.ImageURL != nil:
				addNonText(model.ContentTypeImageURL)
			case partType == "" && part.InputAudio != nil:
				addNonText(model.ContentTypeInputAudio)
			default:
				addNonText(partType)
			}
		}
	case []any:
		for _, rawPart := range typed {
			partMap, ok := rawPart.(map[string]any)
			if !ok {
				addNonText("unknown")
				continue
			}

			partType, _ := partMap["type"].(string)
			partType = strings.TrimSpace(partType)
			switch {
			case partType == "" && partMap["text"] != nil:
				addNonText(model.ContentTypeText)
			case partType == "" && partMap["image_url"] != nil:
				addNonText(model.ContentTypeImageURL)
			case partType == "" && partMap["input_audio"] != nil:
				addNonText(model.ContentTypeInputAudio)
			case partType == "":
				addNonText("unknown")
			default:
				addNonText(partType)
			}
		}
	default:
		addNonText("unknown")
	}

	if len(nonText) == 0 {
		return nil
	}

	slices.Sort(nonText)
	return slices.Compact(nonText)
}

// normalizeGroqContentType normalizes OpenAI/Responses content type names to Groq chat
// content type names for validation.
func normalizeGroqContentType(partType string) string {
	switch strings.ToLower(strings.TrimSpace(partType)) {
	case "", "text", "input_text", "output_text":
		return model.ContentTypeText
	case "input_image":
		return model.ContentTypeImageURL
	default:
		return strings.ToLower(strings.TrimSpace(partType))
	}
}
