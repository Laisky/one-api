package aws

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/Laisky/errors/v2"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/relay/adaptor"
	anthropicAdaptor "github.com/songquanpeng/one-api/relay/adaptor/anthropic"
	"github.com/songquanpeng/one-api/relay/adaptor/aws/utils"
	"github.com/songquanpeng/one-api/relay/billing/ratio"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
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

func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}

	adaptor := GetAdaptor(request.Model)
	if adaptor == nil {
		return nil, errors.New("adaptor not found")
	}

	// Validate parameters using the new model-based validation
	if validationErr := ValidateUnsupportedParameters(request, request.Model); validationErr != nil {
		return nil, errors.Errorf("validation failed: %s", validationErr.Error.Message)
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
	return nil, nil
}

// Pricing methods - AWS adapter manages its own model pricing
func (a *Adaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	const MilliTokensUsd = 0.000001

	// Direct map definition - much easier to maintain and edit
	// Pricing from https://aws.amazon.com/bedrock/pricing/
	return map[string]adaptor.ModelConfig{
		// Claude Models on AWS Bedrock
		"claude-instant-1.2":         {Ratio: 0.8 * MilliTokensUsd, CompletionRatio: 3.125}, // $0.8/$2.5 per 1M tokens
		"claude-2.0":                 {Ratio: 8 * MilliTokensUsd, CompletionRatio: 3.125},   // $8/$25 per 1M tokens
		"claude-2.1":                 {Ratio: 8 * MilliTokensUsd, CompletionRatio: 3.125},   // $8/$25 per 1M tokens
		"claude-3-haiku-20240307":    {Ratio: 0.25 * MilliTokensUsd, CompletionRatio: 5},    // $0.25/$1.25 per 1M tokens
		"claude-3-sonnet-20240229":   {Ratio: 3 * MilliTokensUsd, CompletionRatio: 5},       // $3/$15 per 1M tokens
		"claude-3-opus-20240229":     {Ratio: 15 * MilliTokensUsd, CompletionRatio: 5},      // $15/$75 per 1M tokens
		"claude-opus-4-20250514":     {Ratio: 15 * MilliTokensUsd, CompletionRatio: 5},      // $15/$75 per 1M tokens
		"claude-opus-4-1-20250805":   {Ratio: 15 * MilliTokensUsd, CompletionRatio: 5},      // $15/$75 per 1M tokens
		"claude-3-5-sonnet-20240620": {Ratio: 3 * MilliTokensUsd, CompletionRatio: 5},       // $3/$15 per 1M tokens
		"claude-3-5-sonnet-20241022": {Ratio: 3 * MilliTokensUsd, CompletionRatio: 5},       // $3/$15 per 1M tokens
		"claude-3-5-sonnet-latest":   {Ratio: 3 * MilliTokensUsd, CompletionRatio: 5},       // $3/$15 per 1M tokens
		"claude-3-5-haiku-20241022":  {Ratio: 1 * MilliTokensUsd, CompletionRatio: 5},       // $1/$5 per 1M tokens
		"claude-3-7-sonnet-latest":   {Ratio: 3 * MilliTokensUsd, CompletionRatio: 5},       // $3/$15 per 1M tokens
		"claude-3-7-sonnet-20250219": {Ratio: 3 * MilliTokensUsd, CompletionRatio: 5},       // $3/$15 per 1M tokens
		"claude-sonnet-4-20250514":   {Ratio: 3 * MilliTokensUsd, CompletionRatio: 5},       // $3/$15 per 1M tokens

		// Llama Models on AWS Bedrock
		// Note: Pricing may need to be updated later; also this model is significantly faster on AWS GPUs.
		// Llama 4 models
		"llama4-maverick-17b-1m": {Ratio: 0.24 * MilliTokensUsd, CompletionRatio: 4.04}, // $0.00024/$0.00097 per 1K tokens
		"llama4-scout-17b-3.5m":  {Ratio: 0.17 * MilliTokensUsd, CompletionRatio: 3.88}, // $0.00017/$0.00066 per 1K tokens

		// Llama 3.3 models
		"llama3-3-70b-128k": {Ratio: 0.72 * MilliTokensUsd, CompletionRatio: 1}, // $0.00072/$0.00072 per 1K tokens

		// Llama 3.2 models
		"llama3-2-1b-131k":         {Ratio: 0.1 * MilliTokensUsd, CompletionRatio: 1},  // $0.0001/$0.0001 per 1K tokens
		"llama3-2-3b-131k":         {Ratio: 0.15 * MilliTokensUsd, CompletionRatio: 1}, // $0.00015/$0.00015 per 1K tokens
		"llama3-2-11b-vision-131k": {Ratio: 0.16 * MilliTokensUsd, CompletionRatio: 1}, // $0.00016/$0.00016 per 1K tokens
		"llama3-2-90b-128k":        {Ratio: 0.72 * MilliTokensUsd, CompletionRatio: 1}, // $0.00072/$0.00072 per 1K tokens

		// Llama 3.1 models
		"llama3-1-8b-128k":  {Ratio: 0.22 * MilliTokensUsd, CompletionRatio: 1}, // $0.00022/$0.00022 per 1K tokens
		"llama3-1-70b-128k": {Ratio: 0.72 * MilliTokensUsd, CompletionRatio: 1}, // $0.00072/$0.00072 per 1K tokens

		// Llama 3 models (updated pricing)
		"llama3-8b-8192":  {Ratio: 0.3 * MilliTokensUsd, CompletionRatio: 2},     // $0.0003/$0.0006 per 1K tokens
		"llama3-70b-8192": {Ratio: 2.65 * MilliTokensUsd, CompletionRatio: 1.32}, // $0.00265/$0.0035 per 1K tokens

		// Amazon Nova Models (if supported)
		"amazon-nova-micro":   {Ratio: 0.035 * MilliTokensUsd, CompletionRatio: 4.28}, // $0.035/$0.15 per 1M tokens
		"amazon-nova-lite":    {Ratio: 0.06 * MilliTokensUsd, CompletionRatio: 4.17},  // $0.06/$0.25 per 1M tokens
		"amazon-nova-pro":     {Ratio: 0.8 * MilliTokensUsd, CompletionRatio: 4},      // $0.8/$3.2 per 1M tokens
		"amazon-nova-premier": {Ratio: 2.4 * MilliTokensUsd, CompletionRatio: 4.17},   // $2.4/$10 per 1M tokens

		// Titan Models (if supported)
		"amazon-titan-text-lite":    {Ratio: 0.3 * MilliTokensUsd, CompletionRatio: 1.33}, // $0.3/$0.4 per 1M tokens
		"amazon-titan-text-express": {Ratio: 0.8 * MilliTokensUsd, CompletionRatio: 2},    // $0.8/$1.6 per 1M tokens
		"amazon-titan-embed-text":   {Ratio: 0.1 * MilliTokensUsd, CompletionRatio: 1},    // $0.1 per 1M tokens

		// Cohere Models (if supported)
		"cohere-command-text":       {Ratio: 1.5 * MilliTokensUsd, CompletionRatio: 1.33}, // $1.5/$2 per 1M tokens
		"cohere-command-light-text": {Ratio: 0.3 * MilliTokensUsd, CompletionRatio: 2},    // $0.3/$0.6 per 1M tokens

		// AI21 Models (if supported)
		"ai21-j2-mid":    {Ratio: 12.5 * MilliTokensUsd, CompletionRatio: 1}, // $12.5 per 1M tokens
		"ai21-j2-ultra":  {Ratio: 18.8 * MilliTokensUsd, CompletionRatio: 1}, // $18.8 per 1M tokens
		"ai21-jamba-1.5": {Ratio: 2 * MilliTokensUsd, CompletionRatio: 4},    // $2/$8 per 1M tokens

		// Mistral Models (Supported) - Updated pricing as of 2025-08-27 - Note: These are per 1K tokens, converted to 1M tokens using MilliTokensUsd
		//
		// Note: The Mistral Instruct model (mistral-7b, mixtral-8x7b) is currently considered legacy and unsupported.
		// Only the newest/latest models available in AWS Bedrock are supported.
		"mistral-7b":                 {Ratio: 0.15 * ratio.MilliTokensUsd, CompletionRatio: 1.33}, // $0.00015/$0.0002 per 1K tokens = $0.15/$0.2 per 1M tokens
		"mixtral-8x7b":               {Ratio: 0.45 * ratio.MilliTokensUsd, CompletionRatio: 1.56}, // $0.00045/$0.0007 per 1K tokens = $0.45/$0.7 per 1M tokens
		"mistral-small-2402":         {Ratio: 1 * ratio.MilliTokensUsd, CompletionRatio: 3},       // $0.001/$0.003 per 1K tokens = $1/$3 per 1M tokens
		"mistral-large-2402":         {Ratio: 4 * ratio.MilliTokensUsd, CompletionRatio: 3},       // $0.004/$0.012 per 1K tokens = $4/$12 per 1M tokens
		"mistral-pixtral-large-2502": {Ratio: 2 * ratio.MilliTokensUsd, CompletionRatio: 3},       // $0.002/$0.006 per 1K tokens = $2/$6 per 1M tokens
	}
}

func (a *Adaptor) GetModelRatio(modelName string) float64 {
	pricing := a.GetDefaultModelPricing()
	if price, exists := pricing[modelName]; exists {
		return price.Ratio
	}
	// Default AWS pricing (Claude-like)
	return 3 * 0.000001 // Default USD pricing
}

func (a *Adaptor) GetCompletionRatio(modelName string) float64 {
	pricing := a.GetDefaultModelPricing()
	if price, exists := pricing[modelName]; exists {
		return price.CompletionRatio
	}
	// Default completion ratio for AWS
	return 5.0
}

// UnsupportedParameter represents a parameter that is not supported by a provider
type UnsupportedParameter struct {
	Name        string
	Description string
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
	SupportsFrequencyPenalty    bool
	SupportsPresencePenalty     bool
	SupportsTopLogprobs         bool
	SupportsPrediction          bool
	SupportsMaxCompletionTokens bool
}

// GetModelCapabilities returns the capabilities for a model based on its adapter type
func GetModelCapabilities(modelName string) ProviderCapabilities {
	adaptorType := adaptors[modelName]
	if awsArnMatch != nil && awsArnMatch.MatchString(modelName) {
		adaptorType = AwsClaude
	}

	switch adaptorType {
	case AwsClaude:
		return ProviderCapabilities{
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
			SupportsFrequencyPenalty:    false,
			SupportsPresencePenalty:     false,
			SupportsTopLogprobs:         false,
			SupportsPrediction:          false,
			SupportsMaxCompletionTokens: false,
		}
	case AwsLlama3:
		return ProviderCapabilities{
			SupportsTools:               true, // Llama3 supports tools via Converse API
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
			SupportsFrequencyPenalty:    false,
			SupportsPresencePenalty:     false,
			SupportsTopLogprobs:         false,
			SupportsPrediction:          false,
			SupportsMaxCompletionTokens: false,
		}
	case AwsMistral:
		return ProviderCapabilities{
			SupportsTools:               true, // Mistral supports tools
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
			SupportsFrequencyPenalty:    false,
			SupportsPresencePenalty:     false,
			SupportsTopLogprobs:         false,
			SupportsPrediction:          false,
			SupportsMaxCompletionTokens: false,
		}
	default:
		// Default to minimal capabilities for unknown models
		return ProviderCapabilities{}
	}
}

// ValidateUnsupportedParameters checks for unsupported parameters and returns an error if any are found
// Now uses model names instead of provider names
func ValidateUnsupportedParameters(request *model.GeneralOpenAIRequest, modelName string) *model.ErrorWithStatusCode {
	capabilities := GetModelCapabilities(modelName)
	var unsupportedParams []UnsupportedParameter

	// Check for tools support
	if len(request.Tools) > 0 && !capabilities.SupportsTools {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "tools",
			Description: "Tool calling is not supported by this model",
		})
	}

	// Check for tool_choice support
	if request.ToolChoice != nil && !capabilities.SupportsTools {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "tool_choice",
			Description: "Tool choice is not supported by this model",
		})
	}

	// Check for parallel_tool_calls support
	if request.ParallelTooCalls != nil && !capabilities.SupportsParallelToolCalls {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "parallel_tool_calls",
			Description: "Parallel tool calls are not supported by this model",
		})
	}

	// Check for functions support (deprecated OpenAI feature)
	if len(request.Functions) > 0 && !capabilities.SupportsFunctions {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "functions",
			Description: "Functions (deprecated OpenAI feature) are not supported by this model. Use 'tools' instead",
		})
	}

	// Check for function_call support
	if request.FunctionCall != nil && !capabilities.SupportsFunctions {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "function_call",
			Description: "Function call (deprecated OpenAI feature) is not supported by this model. Use 'tool_choice' instead",
		})
	}

	// Check for logprobs support
	if request.Logprobs != nil && *request.Logprobs && !capabilities.SupportsLogprobs {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "logprobs",
			Description: "Log probabilities are not supported by this model",
		})
	}

	// Check for top_logprobs support
	if request.TopLogprobs != nil && !capabilities.SupportsTopLogprobs {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "top_logprobs",
			Description: "Top log probabilities are not supported by this model",
		})
	}

	// Check for logit_bias support
	if request.LogitBias != nil && !capabilities.SupportsLogitBias {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "logit_bias",
			Description: "Logit bias is not supported by this model",
		})
	}

	// Check for response_format support
	if request.ResponseFormat != nil && !capabilities.SupportsResponseFormat {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "response_format",
			Description: "Response format is not supported by this model",
		})
	}

	// Check for reasoning_effort support
	if request.ReasoningEffort != nil && !capabilities.SupportsReasoningEffort {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "reasoning_effort",
			Description: "Reasoning effort is not supported by this model",
		})
	}

	// Check for modalities support
	if len(request.Modalities) > 0 && !capabilities.SupportsModalities {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "modalities",
			Description: "Modalities are not supported by this model",
		})
	}

	// Check for audio support
	if request.Audio != nil && !capabilities.SupportsAudio {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "audio",
			Description: "Audio input/output is not supported by this model",
		})
	}

	// Check for web_search_options support
	if request.WebSearchOptions != nil && !capabilities.SupportsWebSearch {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "web_search_options",
			Description: "Web search is not supported by this model",
		})
	}

	// Check for thinking support (Anthropic-specific)
	if request.Thinking != nil && !capabilities.SupportsThinking {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "thinking",
			Description: "Extended thinking is not supported by this model",
		})
	}

	// Check for service_tier support
	if request.ServiceTier != nil && !capabilities.SupportsServiceTier {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "service_tier",
			Description: "Service tier is not supported by this model",
		})
	}

	// Check for prediction support
	if request.Prediction != nil && !capabilities.SupportsPrediction {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "prediction",
			Description: "Prediction is not supported by this model",
		})
	}

	// Check for max_completion_tokens support
	if request.MaxCompletionTokens != nil && !capabilities.SupportsMaxCompletionTokens {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "max_completion_tokens",
			Description: "max_completion_tokens is not supported by this model. Use 'max_tokens' instead",
		})
	}

	// Check for frequency_penalty support
	if request.FrequencyPenalty != nil && !capabilities.SupportsFrequencyPenalty {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "frequency_penalty",
			Description: "Frequency penalty is not supported by this model",
		})
	}

	// Check for presence_penalty support
	if request.PresencePenalty != nil && !capabilities.SupportsPresencePenalty {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "presence_penalty",
			Description: "Presence penalty is not supported by this model",
		})
	}

	// If we found unsupported parameters, return an error
	if len(unsupportedParams) > 0 {
		var errorMessage string
		if len(unsupportedParams) == 1 {
			errorMessage = fmt.Sprintf("Unsupported parameter '%s': %s",
				unsupportedParams[0].Name, unsupportedParams[0].Description)
		} else {
			errorMessage = fmt.Sprintf("Unsupported parameters for model '%s':", modelName)
			for _, param := range unsupportedParams {
				errorMessage += fmt.Sprintf("\n- %s: %s", param.Name, param.Description)
			}
		}

		return &model.ErrorWithStatusCode{
			StatusCode: http.StatusBadRequest,
			Error: model.Error{
				Message: errorMessage,
				Type:    "invalid_request_error",
				Code:    "unsupported_parameter",
			},
		}
	}

	return nil
}
