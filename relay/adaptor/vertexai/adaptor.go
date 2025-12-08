package vertexai

import (
	"fmt"
	"io"
	"maps"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/relay/adaptor"
	channelhelper "github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/adaptor/geminiOpenaiCompatible"
	vertexaiClaude "github.com/songquanpeng/one-api/relay/adaptor/vertexai/claude"
	"github.com/songquanpeng/one-api/relay/adaptor/vertexai/deepseek"
	"github.com/songquanpeng/one-api/relay/adaptor/vertexai/imagen"
	"github.com/songquanpeng/one-api/relay/adaptor/vertexai/openai"
	"github.com/songquanpeng/one-api/relay/adaptor/vertexai/qwen"
	"github.com/songquanpeng/one-api/relay/adaptor/vertexai/veo"
	"github.com/songquanpeng/one-api/relay/billing/ratio"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
	relayModel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

var _ adaptor.Adaptor = new(Adaptor)

const channelName = "vertexai"

// IsRequireGlobalEndpoint determines if the given model requires a global endpoint
//
//   - https://cloud.google.com/vertex-ai/generative-ai/docs/models/gemini/2-5-pro
//   - https://cloud.google.com/vertex-ai/generative-ai/docs/learn/locations#global-preview
//
// IsRequireGlobalEndpoint determines if the given model requires a global endpoint
// by checking whether its numeric version is at least 2.5.
func IsRequireGlobalEndpoint(model string) bool {
	return geminiOpenaiCompatible.GeminiVersionAtLeast(model, 2.5)
}

type Adaptor struct {
}

func (a *Adaptor) Init(meta *meta.Meta) {
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, request *model.ImageRequest) (any, error) {
	meta := meta.GetByContext(c)

	if request.ResponseFormat == nil || *request.ResponseFormat != "b64_json" {
		return nil, errors.New("only support b64_json response format")
	}

	adaptor := GetAdaptor(meta.ActualModelName)
	if adaptor == nil {
		return nil, errors.Errorf("cannot found vertex image adaptor for model %s", meta.ActualModelName)
	}

	return adaptor.ConvertImageRequest(c, request)
}

func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	meta := meta.GetByContext(c)

	adaptor := GetAdaptor(meta.ActualModelName)
	if adaptor == nil {
		return nil, errors.Errorf("cannot found vertex chat adaptor for model %s", meta.ActualModelName)
	}

	return adaptor.ConvertRequest(c, relayMode, request)
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}

	meta := meta.GetByContext(c)

	adaptor := GetAdaptor(meta.ActualModelName)
	if adaptor == nil {
		return nil, errors.Errorf("cannot found vertex adaptor for model %s", meta.ActualModelName)
	}

	// Convert Claude Messages API request to OpenAI format first
	openaiRequest := &model.GeneralOpenAIRequest{
		Model:       request.Model,
		MaxTokens:   request.MaxTokens,
		Temperature: request.Temperature,
		TopP:        request.TopP,
		Stream:      request.Stream != nil && *request.Stream,
		Stop:        request.StopSequences,
		Thinking:    request.Thinking,
	}

	// Add system message if present
	if request.System != "" {
		systemMessage := model.Message{
			Role:    "system",
			Content: request.System,
		}
		openaiRequest.Messages = append(openaiRequest.Messages, systemMessage)
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
									if mediaType, exists := sourceMap["media_type"]; exists {
										if data, exists := sourceMap["data"]; exists {
											if mediaTypeStr, ok := mediaType.(string); ok {
												if dataStr, ok := data.(string); ok {
													imageURL := fmt.Sprintf("data:%s;base64,%s", mediaTypeStr, dataStr)
													contentParts = append(contentParts, model.MessageContent{
														Type: "image_url",
														ImageURL: &model.ImageURL{
															Url: imageURL,
														},
													})
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
			if len(contentParts) > 0 {
				openaiMessage.Content = contentParts
			}
		}

		openaiRequest.Messages = append(openaiRequest.Messages, openaiMessage)
	}

	// Convert tools if present
	if len(request.Tools) > 0 {
		var tools []model.Tool
		for _, claudeTool := range request.Tools {
			tool := model.Tool{
				Type: "function",
				Function: &model.Function{
					Name:        claudeTool.Name,
					Description: claudeTool.Description,
					Parameters:  claudeTool.InputSchema.(map[string]any),
				},
			}
			tools = append(tools, tool)
		}
		openaiRequest.Tools = tools
	}

	// Convert tool choice if present
	if request.ToolChoice != nil {
		openaiRequest.ToolChoice = request.ToolChoice
	}

	// Mark this as a Claude Messages conversion for response handling
	c.Set(ctxkey.ClaudeMessagesConversion, true)
	c.Set(ctxkey.OriginalClaudeRequest, request)

	// Now convert the OpenAI request to VertexAI format using existing logic
	return adaptor.ConvertRequest(c, relaymode.ChatCompletions, openaiRequest)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	adaptor := GetAdaptor(meta.ActualModelName)
	if adaptor == nil {
		return nil, &relayModel.ErrorWithStatusCode{
			StatusCode: http.StatusInternalServerError,
			Error: relayModel.Error{
				Message:  "adaptor not found",
				RawError: errors.New("adaptor not found"),
			},
		}
	}

	return adaptor.DoResponse(c, resp, meta)
}

func (a *Adaptor) GetModelList() []string {
	// Aggregate model lists from all subadaptors
	var models []string

	// Add models from each subadaptor
	models = append(models, adaptor.GetModelListFromPricing(vertexaiClaude.ModelRatios)...)
	models = append(models, adaptor.GetModelListFromPricing(imagen.ModelRatios)...)
	models = append(models, adaptor.GetModelListFromPricing(geminiOpenaiCompatible.ModelRatios)...)
	models = append(models, adaptor.GetModelListFromPricing(veo.ModelRatios)...)
	models = append(models, adaptor.GetModelListFromPricing(deepseek.ModelRatios)...)
	models = append(models, adaptor.GetModelListFromPricing(openai.ModelRatios)...)
	models = append(models, adaptor.GetModelListFromPricing(qwen.ModelRatios)...)

	// Add VertexAI-specific models
	models = append(models, "gemini-embedding-001", "aqa")

	return models
}

func (a *Adaptor) GetChannelName() string {
	return channelName
}

// Pricing methods - VertexAI adapter aggregates pricing from subadaptors
// Following DRY principles by importing ratios from each subadaptor
func (a *Adaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	// Import pricing from subadaptors to eliminate redundancy
	pricing := make(map[string]adaptor.ModelConfig)

	// Import Claude models from claude subadaptor
	maps.Copy(pricing, vertexaiClaude.ModelRatios)

	// Import Imagen models from imagen subadaptor
	maps.Copy(pricing, imagen.ModelRatios)

	// Import Gemini models from geminiOpenaiCompatible (shared with VertexAI)
	maps.Copy(pricing, geminiOpenaiCompatible.ModelRatios)

	// Import Veo models from veo subadaptor
	maps.Copy(pricing, veo.ModelRatios)

	// Import DeepSeek models from deepseek subadaptor
	maps.Copy(pricing, deepseek.ModelRatios)

	// Import OpenAI models from openai subadaptor
	maps.Copy(pricing, openai.ModelRatios)

	// Import Qwen models from qwen subadaptor
	maps.Copy(pricing, qwen.ModelRatios)

	// Add VertexAI-specific models that don't belong to subadaptors
	// Using global ratio.MilliTokensUsd = 0.5 for consistent quota-based pricing

	// VertexAI-specific models
	pricing["gemini-embedding-001"] = adaptor.ModelConfig{Ratio: 0.15 * ratio.MilliTokensUsd, CompletionRatio: 1}
	pricing["aqa"] = adaptor.ModelConfig{Ratio: 1, CompletionRatio: 1}

	return pricing
}

func (a *Adaptor) GetModelRatio(modelName string) float64 {
	pricing := a.GetDefaultModelPricing()
	if price, exists := pricing[modelName]; exists {
		return price.Ratio
	}
	// Default VertexAI pricing (similar to Gemini)
	return 0.5 * ratio.MilliTokensUsd // Default quota-based pricing
}

func (a *Adaptor) GetCompletionRatio(modelName string) float64 {
	pricing := a.GetDefaultModelPricing()
	if price, exists := pricing[modelName]; exists {
		return price.CompletionRatio
	}
	// Default completion ratio for VertexAI
	return 3.0
}

// DefaultToolingConfig returns Vertex AI tooling defaults (grounding, enterprise search, and Claude web search fees).
func (a *Adaptor) DefaultToolingConfig() adaptor.ChannelToolConfig {
	return VertexAIToolingDefaults
}

// ModelEndpointType represents different endpoint types for VertexAI models
type ModelEndpointType int

const (
	EndpointTypeDeepSeek ModelEndpointType = iota
	EndpointTypeOpenAI
	EndpointTypeQwen
	EndpointTypeImagen
	EndpointTypeClaude
	EndpointTypeGemini
)

// getModelEndpointType determines the endpoint type based on model name
func getModelEndpointType(modelName string) ModelEndpointType {
	switch {
	case isDeepSeekModel(modelName):
		return EndpointTypeDeepSeek
	case isOpenAIModel(modelName):
		return EndpointTypeOpenAI
	case isQwenModel(modelName):
		return EndpointTypeQwen
	case isImagenModel(modelName):
		return EndpointTypeImagen
	case strings.Contains(modelName, "claude"):
		return EndpointTypeClaude
	default:
		return EndpointTypeGemini
	}
}

func (a *Adaptor) GetRequestURL(meta *meta.Meta) (string, error) {
	// Validate required VertexAI configuration
	if meta.Config.VertexAIProjectID == "" {
		return "", errors.Errorf("VertexAI project ID is required but not configured for channel")
	}

	endpointType := getModelEndpointType(meta.ActualModelName)

	switch endpointType {
	case EndpointTypeDeepSeek:
		return a.buildDeepSeekURL(meta)
	case EndpointTypeOpenAI:
		return a.buildOpenAIURL(meta)
	case EndpointTypeQwen:
		return a.buildQwenURL(meta)
	case EndpointTypeImagen:
		return a.buildImagenURL(meta)
	case EndpointTypeClaude:
		return a.buildClaudeURL(meta)
	case EndpointTypeGemini:
		return a.buildGeminiURL(meta)
	default:
		return a.buildGeminiURL(meta) // fallback to Gemini
	}
}

// buildDeepSeekURL builds URL for DeepSeek models
func (a *Adaptor) buildDeepSeekURL(meta *meta.Meta) (string, error) {
	// DeepSeek models use OpenAI-compatible API with custom endpoint structure
	baseHost, location := getDeepSeekEndpointConfig(meta.ActualModelName)

	// Handle custom base URL and region
	baseHost, location = a.applyCustomHostAndRegion(baseHost, location, meta)

	return fmt.Sprintf("https://%s/v1/projects/%s/locations/%s/endpoints/openapi/chat/completions",
		baseHost, meta.Config.VertexAIProjectID, location), nil
}

// buildOpenAIURL builds URL for OpenAI GPT-OSS models
func (a *Adaptor) buildOpenAIURL(meta *meta.Meta) (string, error) {
	// OpenAI GPT-OSS models use OpenAI-compatible API with global endpoint
	baseHost := "aiplatform.googleapis.com"
	location := "global"

	// Handle custom base URL and region
	baseHost, location = a.applyCustomHostAndRegion(baseHost, location, meta)

	return fmt.Sprintf("https://%s/v1/projects/%s/locations/%s/endpoints/openapi/chat/completions",
		baseHost, meta.Config.VertexAIProjectID, location), nil
}

// buildQwenURL builds URL for Qwen models
func (a *Adaptor) buildQwenURL(meta *meta.Meta) (string, error) {
	// Different Qwen models use different endpoints
	baseHost, location := getQwenEndpointConfig(meta.ActualModelName)

	// Handle custom base URL and region
	baseHost, location = a.applyCustomHostAndRegion(baseHost, location, meta)

	return fmt.Sprintf("https://%s/v1/projects/%s/locations/%s/endpoints/openapi/chat/completions",
		baseHost, meta.Config.VertexAIProjectID, location), nil
}

// buildImagenURL builds URL for Imagen models
func (a *Adaptor) buildImagenURL(meta *meta.Meta) (string, error) {
	// Imagen models use the :predict endpoint
	// Docs: https://cloud.google.com/vertex-ai/generative-ai/docs/model-reference/imagen-api
	baseHost, location := a.getDefaultHostAndLocation(meta)

	return fmt.Sprintf("https://%s/v1/projects/%s/locations/%s/publishers/google/models/%s:predict",
		baseHost, meta.Config.VertexAIProjectID, location, meta.ActualModelName), nil
}

// buildClaudeURL builds URL for Claude models
func (a *Adaptor) buildClaudeURL(meta *meta.Meta) (string, error) {
	// Claude models use rawPredict
	baseHost, location := a.getDefaultHostAndLocation(meta)

	return fmt.Sprintf("https://%s/v1/projects/%s/locations/%s/publishers/anthropic/models/%s:rawPredict",
		baseHost, meta.Config.VertexAIProjectID, location, meta.ActualModelName), nil
}

// buildGeminiURL builds URL for Gemini and other text models
func (a *Adaptor) buildGeminiURL(meta *meta.Meta) (string, error) {
	// Gemini (and other text models) use generateContent / streamGenerateContent
	var suffix string
	if meta.IsStream {
		suffix = "streamGenerateContent?alt=sse"
	} else {
		suffix = "generateContent"
	}

	baseHost, location := a.getDefaultHostAndLocation(meta)

	return fmt.Sprintf("https://%s/v1/projects/%s/locations/%s/publishers/google/models/%s:%s",
		baseHost, meta.Config.VertexAIProjectID, location, meta.ActualModelName, suffix), nil
}

// getDefaultHostAndLocation returns the default host and location for standard VertexAI models
func (a *Adaptor) getDefaultHostAndLocation(meta *meta.Meta) (baseHost, location string) {
	location = "us-central1"
	baseHost = "us-central1-aiplatform.googleapis.com"

	// Check if model requires global endpoint
	if IsRequireGlobalEndpoint(meta.ActualModelName) {
		location = "global"
		baseHost = "aiplatform.googleapis.com"
	} else if meta.Config.Region != "" {
		location = meta.Config.Region
		baseHost = fmt.Sprintf("%s-aiplatform.googleapis.com", location)
	}

	// Handle custom base URL
	if meta.BaseURL != "" {
		baseHost = strings.TrimPrefix(meta.BaseURL, "https://")
		baseHost = strings.TrimPrefix(baseHost, "http://")
		baseHost = strings.TrimSuffix(baseHost, "/")
	}

	return baseHost, location
}

// applyCustomHostAndRegion applies custom base URL and region overrides
func (a *Adaptor) applyCustomHostAndRegion(baseHost, location string, meta *meta.Meta) (string, string) {
	// Handle custom base URL
	if meta.BaseURL != "" {
		baseHost = strings.TrimPrefix(meta.BaseURL, "https://")
		baseHost = strings.TrimPrefix(baseHost, "http://")
		baseHost = strings.TrimSuffix(baseHost, "/")
	}

	// Handle custom region if specified
	if meta.Config.Region != "" {
		location = meta.Config.Region
	}

	return baseHost, location
}

// isImagenModel returns true if the model name belongs to Vertex AI Imagen family.
// Imagen models require the :predict endpoint and reject generateContent.
func isImagenModel(model string) bool {
	if model == "" {
		return false
	}
	return strings.HasPrefix(model, "imagen-") || strings.HasPrefix(model, "imagegeneration@")
}

// isDeepSeekModel returns true if the model name belongs to DeepSeek family.
// DeepSeek models use OpenAI-compatible API with custom endpoint.
func isDeepSeekModel(model string) bool {
	if model == "" {
		return false
	}
	return strings.HasPrefix(model, "deepseek-ai/")
}

// getDeepSeekEndpointConfig returns the appropriate endpoint configuration for DeepSeek models.
// Different DeepSeek models use different endpoints and regions.
func getDeepSeekEndpointConfig(model string) (baseHost, defaultRegion string) {
	switch model {
	case "deepseek-ai/deepseek-r1-0528-maas":
		// DeepSeek R1 uses us-central1 endpoint
		return "us-central1-aiplatform.googleapis.com", "us-central1"
	case "deepseek-ai/deepseek-v3.1-maas":
		// DeepSeek V3.1 uses us-west2 endpoint
		return "us-west2-aiplatform.googleapis.com", "us-west2"
	default:
		// Default to us-west2 for any new DeepSeek models
		return "us-west2-aiplatform.googleapis.com", "us-west2"
	}
}

// isOpenAIModel returns true if the model name belongs to OpenAI GPT-OSS family.
// OpenAI GPT-OSS models use OpenAI-compatible API with global endpoint.
func isOpenAIModel(model string) bool {
	if model == "" {
		return false
	}
	return strings.HasPrefix(model, "openai/")
}

// isQwenModel returns true if the model name belongs to Qwen family.
// Qwen models use OpenAI-compatible API with custom endpoint.
func isQwenModel(model string) bool {
	if model == "" {
		return false
	}
	return strings.HasPrefix(model, "qwen/")
}

// getQwenEndpointConfig returns the appropriate endpoint configuration for Qwen models.
// Different Qwen models use different endpoints and regions.
func getQwenEndpointConfig(model string) (baseHost, defaultRegion string) {
	switch model {
	case "qwen/qwen3-next-80b-a3b-instruct-maas":
		// Qwen3-next uses global endpoint
		return "aiplatform.googleapis.com", "global"
	case "qwen/qwen3-coder-480b-a35b-instruct-maas",
		"qwen/qwen3-235b-a22b-instruct-2507-maas":
		// Qwen3-coder and Qwen3-235b use us-south1 endpoint
		return "us-south1-aiplatform.googleapis.com", "us-central1"
	default:
		// Default to us-south1 for any new Qwen models
		return "us-south1-aiplatform.googleapis.com", "us-central1"
	}
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) error {
	adaptor.SetupCommonRequestHeader(c, req, meta)
	token, err := getToken(c, meta.ChannelId, meta.Config.VertexAIADC)
	if err != nil {
		return errors.Wrap(err, "get Vertex AI token")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return nil
}

func (a *Adaptor) DoRequest(c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	return channelhelper.DoRequestHelper(a, c, meta, requestBody)
}
