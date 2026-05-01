package aws

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor"
	anthropicAdaptor "github.com/Laisky/one-api/relay/adaptor/anthropic"
	"github.com/Laisky/one-api/relay/adaptor/aws/utils"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

var _ adaptor.Adaptor = new(Adaptor)

type Adaptor struct {
	awsAdapter utils.AwsAdapter
	Config     aws.Config
	Meta       *meta.Meta
	AwsClient  *bedrockruntime.Client
}

func (a *Adaptor) Init(meta *meta.Meta) {
	a.Meta = meta
	defaultConfig, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(meta.Config.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			meta.Config.AK, meta.Config.SK, "")))
	if err != nil {
		return
	}
	a.Config = defaultConfig
	a.AwsClient = bedrockruntime.NewFromConfig(defaultConfig)
}

// DefaultToolingConfig returns Bedrock AgentCore tooling defaults (search, tool invocation, identity, memory fees).
func (a *Adaptor) DefaultToolingConfig() adaptor.ChannelToolConfig {
	return AWSToolingDefaults
}

func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}

	// Check if the model supports embedding for embedding requests
	if relayMode == relaymode.Embeddings {
		capabilities := GetModelCapabilities(request.Model)
		if !capabilities.SupportsEmbedding {
			return nil, errors.Errorf("model '%s' does not support embedding", request.Model)
		}
	}

	adaptor := GetAdaptor(request.Model)
	if adaptor == nil {
		return nil, errors.New("adaptor not found")
	}

	// Validate parameters using the new model-based validation
	if validationErr := ValidateUnsupportedParameters(request, request.Model); validationErr != nil {
		return nil, errors.Errorf("validation failed: %s", validationErr.Error.Message)
	}

	// Prefer max_completion_tokens; for providers that do not support it, map to max_tokens
	capabilities := GetModelCapabilities(request.Model)
	if request.MaxCompletionTokens != nil && *request.MaxCompletionTokens > 0 && !capabilities.SupportsMaxCompletionTokens {
		// Always prefer MaxCompletionTokens value
		request.MaxTokens = *request.MaxCompletionTokens
	}

	a.awsAdapter = adaptor
	return adaptor.ConvertRequest(c, relayMode, request)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	if a.awsAdapter == nil {
		return nil, utils.WrapErr(errors.New("awsAdapter is nil"))
	}
	return a.awsAdapter.DoResponse(c, a.AwsClient, meta)
}

func (a *Adaptor) GetModelList() (models []string) {
	for model := range adaptors {
		models = append(models, model)
	}
	return
}

func (a *Adaptor) GetChannelName() string {
	return "aws"
}

func (a *Adaptor) GetRequestURL(meta *meta.Meta) (string, error) {
	return "", nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) error {
	return nil
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, request *model.ImageRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}

	// Check if the model supports image generation
	capabilities := GetModelCapabilities(request.Model)
	if !capabilities.SupportsImageGeneration {
		return nil, errors.Errorf("model '%s' does not support image generation", request.Model)
	}

	// Initialize the AWS adapter based on the model
	adaptor := GetAdaptor(request.Model)
	if adaptor == nil {
		return nil, errors.New("adaptor not found for model: " + request.Model)
	}
	a.awsAdapter = adaptor

	// Store the image request in context for the Titan or Canvas adapter to use later
	c.Set(ctxkey.ImageRequest, *request)
	c.Set(ctxkey.RequestModel, request.Model)

	// For image generation, we need to convert to GeneralOpenAIRequest format
	// and then let the specific adapter handle the conversion
	generalRequest := &model.GeneralOpenAIRequest{
		Model: request.Model,
	}

	return adaptor.ConvertRequest(c, relaymode.ImagesGenerations, generalRequest)
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}

	// Check if this model supports Claude Messages API (v1/messages)
	// Only Claude models should use this endpoint; other models should use v1/chat/completions
	if !IsClaudeModel(request.Model) {
		return nil, errors.Errorf("model '%s' does not support the v1/messages endpoint. Please use v1/chat/completions instead", request.Model)
	}

	// AWS Bedrock supports Claude Messages natively. Do not convert payload.
	// Just set context for billing/routing and mark direct pass-through.
	sub := GetAdaptor(request.Model)
	if sub == nil {
		return nil, errors.New("adaptor not found for model: " + request.Model)
	}
	a.awsAdapter = sub
	c.Set(ctxkey.ClaudeMessagesNative, true)
	c.Set(ctxkey.ClaudeDirectPassthrough, true)
	c.Set(ctxkey.OriginalClaudeRequest, request)
	c.Set(ctxkey.RequestModel, request.Model)
	// Also parse into anthropic.Request for AWS SDK payload building
	if parsed, perr := anthropicAdaptor.ConvertClaudeRequest(c, *request); perr == nil {
		c.Set(ctxkey.ConvertedRequest, parsed)
	} else {
		return nil, perr
	}
	// Return the original request object; controller will forward original body
	return request, nil
}

func (a *Adaptor) DoRequest(c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	// AWS Bedrock doesn't use HTTP requests - it uses the AWS SDK directly
	// For Claude Messages API, we should return nil to indicate DoResponse should handle everything
	// But we need to ensure the controller doesn't try to access a nil response
	if a.awsAdapter == nil {
		return nil, errors.New("AWS sub-adapter not initialized")
	}

	// Add logging to match other adapters that use DoRequestHelper
	// Since AWS uses SDK directly, we manually add the upstream request logging here
	lg := gmw.GetLogger(c).With(
		zap.String("url", "AWS Bedrock SDK"),
		zap.Int("channelId", meta.ChannelId),
		zap.Int("userId", meta.UserId),
		zap.String("model", meta.ActualModelName),
		zap.String("channelName", a.GetChannelName()),
	)
	// Log upstream request for billing tracking (matches common.go:70)
	lg.Info("sending request to upstream channel")

	// For AWS Bedrock, we don't make HTTP requests - we use the AWS SDK directly
	// Return nil response to indicate DoResponse should handle the entire flow
	return nil, nil
}

// GetDefaultModelPricing returns the AWS Bedrock pricing and metadata table.
// The canonical map lives in ratios.go to keep this file under the codebase
// length budget. Callers receive a fresh shallow copy so mutations do not
// affect the shared table.
func (a *Adaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	out := make(map[string]adaptor.ModelConfig, len(awsBedrockModelPricing))
	for k, v := range awsBedrockModelPricing {
		out[k] = v
	}
	return out
}

func (a *Adaptor) GetModelRatio(modelName string) float64 {
	pricing := a.GetDefaultModelPricing()
	if price, exists := pricing[modelName]; exists {
		return price.Ratio
	}
	// Default AWS pricing (Claude-like)
	return 3 * ratio.MilliTokensUsd // Default USD pricing in internal quota units
}

func (a *Adaptor) GetCompletionRatio(modelName string) float64 {
	pricing := a.GetDefaultModelPricing()
	if price, exists := pricing[modelName]; exists {
		return price.CompletionRatio
	}
	// Default completion ratio for AWS
	return 5.0
}

// ProviderCapabilities defines what features are supported by different AWS providers
type ProviderCapabilities struct {
	SupportsTools               bool
	SupportsFunctions           bool
	SupportsLogprobs            bool
	SupportsResponseFormat      bool
	SupportsReasoningEffort     bool
	SupportsModalities          bool
	SupportsAudio               bool
	SupportsWebSearch           bool
	SupportsThinking            bool
	SupportsLogitBias           bool
	SupportsServiceTier         bool
	SupportsParallelToolCalls   bool
	SupportsTopLogprobs         bool
	SupportsPrediction          bool
	SupportsMaxCompletionTokens bool
	SupportsStop                bool
	SupportsImageGeneration     bool
	SupportsEmbedding           bool
}

// isEmbeddingModel checks if a model name indicates it's an embedding model.
//
// TODO: This function needs improvement, as it's currently used for 'amazon-titan-embed-text' and may not cover all cases.
func isEmbeddingModel(modelName string) bool { return strings.Contains(modelName, "embed") }

// isImageGenerationModel checks if a model name indicates it's an image generation model.
//
// TODO: This function needs improvement, as it's currently used for 'amazon-titan-image-generator' and 'amazon-nova-canvas' (image generator) and may not cover all cases.
func isImageGenerationModel(modelName string) bool {
	return strings.Contains(modelName, "image") || strings.Contains(modelName, "canvas")
}

// GetModelCapabilities returns the capabilities for a model based on its adapter type and specific model characteristics
// This function now uses the same model registry as GetModelList for consistency
//
// Note: This implementation provides a flexible foundation for future enhancements,
// allowing for easy addition of model-specific capabilities.
func GetModelCapabilities(modelName string) ProviderCapabilities {
	adaptorType := adaptors[modelName]
	if awsArnMatch != nil && awsArnMatch.MatchString(modelName) {
		adaptorType = AwsClaude
	}

	// If model is not in registry, return minimal capabilities
	if adaptorType == 0 {
		return ProviderCapabilities{
			SupportsImageGeneration: false,
			SupportsEmbedding:       false,
		}
	}

	// Get base capabilities for the adapter type
	var baseCapabilities ProviderCapabilities

	switch adaptorType {
	case AwsClaude:
		baseCapabilities = ProviderCapabilities{
			SupportsTools:               true,  // Claude supports tools via Anthropic format
			SupportsFunctions:           false, // Claude doesn't support OpenAI functions
			SupportsLogprobs:            false,
			SupportsResponseFormat:      true, // Claude supports some response formats
			SupportsReasoningEffort:     false,
			SupportsModalities:          false,
			SupportsAudio:               false,
			SupportsWebSearch:           false,
			SupportsThinking:            true, // Claude supports thinking
			SupportsLogitBias:           false,
			SupportsServiceTier:         false,
			SupportsParallelToolCalls:   false,
			SupportsTopLogprobs:         false,
			SupportsPrediction:          false,
			SupportsMaxCompletionTokens: false,
			SupportsStop:                false, // Claude models use different parameter handling
			SupportsImageGeneration:     false, // Claude models don't support image generation
			SupportsEmbedding:           false, // Claude models don't support embedding
		}
	case AwsCohere:
		baseCapabilities = ProviderCapabilities{
			SupportsTools:               true,  // Cohere models on AWS Bedrock support tool calling via Converse API
			SupportsFunctions:           false, // Cohere doesn't support OpenAI functions
			SupportsLogprobs:            false,
			SupportsResponseFormat:      false,
			SupportsReasoningEffort:     false,
			SupportsModalities:          false,
			SupportsAudio:               false,
			SupportsWebSearch:           false,
			SupportsThinking:            false,
			SupportsLogitBias:           false,
			SupportsServiceTier:         false,
			SupportsParallelToolCalls:   false,
			SupportsTopLogprobs:         false,
			SupportsPrediction:          false,
			SupportsMaxCompletionTokens: false,
			SupportsStop:                true,  // Cohere Command R models support stop parameter
			SupportsImageGeneration:     false, // Cohere Command R models don't support image generation
			SupportsEmbedding:           false, // Cohere Command R models don't support embedding
		}
	case AwsQwen:
		baseCapabilities = ProviderCapabilities{
			SupportsTools:               true,  // Qwen models on AWS Bedrock support tool calling via Converse API
			SupportsFunctions:           false, // Qwen doesn't support OpenAI functions
			SupportsLogprobs:            false,
			SupportsResponseFormat:      false,
			SupportsReasoningEffort:     true,
			SupportsModalities:          false,
			SupportsAudio:               false,
			SupportsWebSearch:           false,
			SupportsThinking:            false,
			SupportsLogitBias:           false,
			SupportsServiceTier:         false,
			SupportsParallelToolCalls:   false,
			SupportsTopLogprobs:         false,
			SupportsPrediction:          false,
			SupportsMaxCompletionTokens: false,
			SupportsStop:                true,  // Qwen models support stop parameter
			SupportsImageGeneration:     false, // Qwen models don't support image generation
			SupportsEmbedding:           false, // Qwen models don't support embedding
		}
	case AwsDeepSeek:
		baseCapabilities = ProviderCapabilities{
			SupportsTools:               false,
			SupportsFunctions:           false,
			SupportsLogprobs:            false,
			SupportsResponseFormat:      false,
			SupportsReasoningEffort:     true, // DeepSeek V3.1 supports reasoning
			SupportsModalities:          false,
			SupportsAudio:               false,
			SupportsWebSearch:           false,
			SupportsThinking:            false,
			SupportsLogitBias:           false,
			SupportsServiceTier:         false,
			SupportsParallelToolCalls:   false,
			SupportsTopLogprobs:         false,
			SupportsPrediction:          false,
			SupportsMaxCompletionTokens: false,
			SupportsStop:                true,  // DeepSeek models support stop parameter
			SupportsImageGeneration:     false, // DeepSeek models don't support image generation
			SupportsEmbedding:           false, // DeepSeek models don't support embedding
		}
	case AwsLlama3:
		baseCapabilities = ProviderCapabilities{
			SupportsTools:               false, // Currently unsupported. May be implemented in the future.
			SupportsFunctions:           false,
			SupportsLogprobs:            false,
			SupportsResponseFormat:      false,
			SupportsReasoningEffort:     false,
			SupportsModalities:          false,
			SupportsAudio:               false,
			SupportsWebSearch:           false,
			SupportsThinking:            false,
			SupportsLogitBias:           false,
			SupportsServiceTier:         false,
			SupportsParallelToolCalls:   false,
			SupportsTopLogprobs:         false,
			SupportsPrediction:          false,
			SupportsMaxCompletionTokens: false,
			SupportsStop:                true,  // Llama models support stop parameter
			SupportsImageGeneration:     false, // Llama models don't support image generation
			SupportsEmbedding:           false, // Llama models don't support embedding
		}
	case AwsMistral:
		baseCapabilities = ProviderCapabilities{
			// Disabled for now due to inconsistencies with the AWS Go SDK's documentation and behavior.
			// Yesterday, it worked for counting tokens with this model using the invoke method, but the converse method doesn't work with tool calling.
			// Furthermore, the token counting functionality has been disabled for this model in the invoke method.
			// Therefore, function tool calling for this model is disabled because the converse method doesn't work with function tool calling,
			// and using the invoke method doesn't provide token usage information, unlike the converse method.
			SupportsTools:               false,
			SupportsFunctions:           false,
			SupportsLogprobs:            false,
			SupportsResponseFormat:      false,
			SupportsReasoningEffort:     false,
			SupportsModalities:          false,
			SupportsAudio:               false,
			SupportsWebSearch:           false,
			SupportsThinking:            false,
			SupportsLogitBias:           false,
			SupportsServiceTier:         false,
			SupportsParallelToolCalls:   false,
			SupportsTopLogprobs:         false,
			SupportsPrediction:          false,
			SupportsMaxCompletionTokens: false,
			SupportsStop:                true,  // Mistral models support stop parameter
			SupportsImageGeneration:     false, // Mistral models don't support image generation
			SupportsEmbedding:           false, // Mistral models don't support embedding
		}
	case AwsOpenAI:
		baseCapabilities = ProviderCapabilities{
			SupportsTools:               false, // OpenAI OSS models don't support tool calling yet
			SupportsFunctions:           false, // OpenAI OSS models don't support OpenAI functions
			SupportsLogprobs:            false,
			SupportsResponseFormat:      false,
			SupportsReasoningEffort:     false,
			SupportsModalities:          false,
			SupportsAudio:               false,
			SupportsWebSearch:           false,
			SupportsThinking:            false,
			SupportsLogitBias:           false,
			SupportsServiceTier:         false,
			SupportsParallelToolCalls:   false,
			SupportsTopLogprobs:         false,
			SupportsPrediction:          false,
			SupportsMaxCompletionTokens: false,
			SupportsStop:                false, // OpenAI OSS models don't support stop parameter
			SupportsImageGeneration:     false, // OpenAI OSS models don't support image generation
			SupportsEmbedding:           false, // OpenAI OSS models don't support embedding
		}
	case AwsWriter:
		baseCapabilities = ProviderCapabilities{
			SupportsTools:               false, // Writer models don't support tool calling yet - only chat conversation is supported for now
			SupportsFunctions:           false, // Writer models don't support OpenAI functions - only chat conversation is supported for now
			SupportsLogprobs:            false,
			SupportsResponseFormat:      false,
			SupportsReasoningEffort:     false, // Writer models don't support reasoning content
			SupportsModalities:          false,
			SupportsAudio:               false,
			SupportsWebSearch:           false,
			SupportsThinking:            false,
			SupportsLogitBias:           false,
			SupportsServiceTier:         false,
			SupportsParallelToolCalls:   false,
			SupportsTopLogprobs:         false,
			SupportsPrediction:          false,
			SupportsMaxCompletionTokens: false,
			SupportsStop:                true,  // Writer models support stop sequences
			SupportsImageGeneration:     false, // Writer models don't support image generation
			SupportsEmbedding:           false, // Writer models don't support embedding
		}
	default:
		// Default to minimal capabilities for unknown models
		return ProviderCapabilities{
			SupportsImageGeneration: false,
			SupportsEmbedding:       false,
		}
	}

	// Override capabilities based on specific model characteristics
	// This ensures consistency with the actual model registry used by GetModelList
	if isEmbeddingModel(modelName) {
		// Embedding models only support embedding, not text generation or image generation
		baseCapabilities.SupportsEmbedding = true
		baseCapabilities.SupportsImageGeneration = false
	} else if isImageGenerationModel(modelName) {
		// Image generation models only support image generation, not embedding
		baseCapabilities.SupportsImageGeneration = true
		baseCapabilities.SupportsEmbedding = false
	} else {
		// Text models don't support embedding or image generation unless specifically indicated
		baseCapabilities.SupportsImageGeneration = false
		baseCapabilities.SupportsEmbedding = false
	}

	return baseCapabilities
}

