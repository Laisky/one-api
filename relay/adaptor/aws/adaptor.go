package aws

import (
	"context"
	"fmt"
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

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/relay/adaptor"
	anthropicAdaptor "github.com/songquanpeng/one-api/relay/adaptor/anthropic"
	"github.com/songquanpeng/one-api/relay/adaptor/aws/utils"
	"github.com/songquanpeng/one-api/relay/billing/ratio"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
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

// Pricing methods - AWS adapter manages its own model pricing
func (a *Adaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	// Direct map definition - much easier to maintain and edit
	// Pricing from https://aws.amazon.com/bedrock/pricing/
	return map[string]adaptor.ModelConfig{
		// Claude Models on AWS Bedrock
		"claude-instant-1.2":         {Ratio: 0.8 * ratio.MilliTokensUsd, CompletionRatio: 3.0, CachedInputRatio: 0.08 * ratio.MilliTokensUsd, CacheWrite5mRatio: 1.0 * ratio.MilliTokensUsd, CacheWrite1hRatio: 1.6 * ratio.MilliTokensUsd},
		"claude-2.0":                 {Ratio: 8 * ratio.MilliTokensUsd, CompletionRatio: 3.0, CachedInputRatio: 0.8 * ratio.MilliTokensUsd, CacheWrite5mRatio: 10 * ratio.MilliTokensUsd, CacheWrite1hRatio: 16 * ratio.MilliTokensUsd},
		"claude-2.1":                 {Ratio: 8 * ratio.MilliTokensUsd, CompletionRatio: 3.0, CachedInputRatio: 0.8 * ratio.MilliTokensUsd, CacheWrite5mRatio: 10 * ratio.MilliTokensUsd, CacheWrite1hRatio: 16 * ratio.MilliTokensUsd},
		"claude-3-haiku-20240307":    {Ratio: 0.25 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.03 * ratio.MilliTokensUsd, CacheWrite5mRatio: 0.30 * ratio.MilliTokensUsd, CacheWrite1hRatio: 0.5 * ratio.MilliTokensUsd},
		"claude-3-5-haiku-20241022":  {Ratio: 0.8 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.08 * ratio.MilliTokensUsd, CacheWrite5mRatio: 1.0 * ratio.MilliTokensUsd, CacheWrite1hRatio: 1.6 * ratio.MilliTokensUsd},
		"claude-3-sonnet-20240229":   {Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd},
		"claude-3-5-sonnet-latest":   {Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd},
		"claude-3-5-sonnet-20240620": {Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd},
		"claude-3-5-sonnet-20241022": {Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd},
		"claude-3-7-sonnet-latest":   {Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd},
		"claude-3-7-sonnet-20250219": {Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd},
		"claude-sonnet-4-0":          {Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd},
		"claude-sonnet-4-20250514":   {Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd},
		"claude-sonnet-4-5":          {Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd},
		"claude-sonnet-4-5-20250929": {Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd},
		"claude-sonnet-4-6":          {Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CacheWrite5mRatio: 3.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 6 * ratio.MilliTokensUsd},
		"claude-3-opus-20240229":     {Ratio: 15 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 1.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 18.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 30 * ratio.MilliTokensUsd},
		"claude-opus-4-0":            {Ratio: 15 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 1.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 18.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 30 * ratio.MilliTokensUsd},
		"claude-opus-4-20250514":     {Ratio: 15 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 1.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 18.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 30 * ratio.MilliTokensUsd},
		"claude-opus-4-1":            {Ratio: 15 * ratio.MilliTokensUsd, CompletionRatio: 5.0, CachedInputRatio: 1.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 18.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 30 * ratio.MilliTokensUsd},
		"claude-opus-4-1-20250805":   {Ratio: 15 * ratio.MilliTokensUsd, CompletionRatio: 75.0 / 15, CachedInputRatio: 1.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 18.75 * ratio.MilliTokensUsd, CacheWrite1hRatio: 30 * ratio.MilliTokensUsd},
		"claude-opus-4-5":            {Ratio: 5 * ratio.MilliTokensUsd, CompletionRatio: 25.0 / 5, CachedInputRatio: 0.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 6.25 * ratio.MilliTokensUsd, CacheWrite1hRatio: 10 * ratio.MilliTokensUsd},
		"claude-opus-4-5-20251101":   {Ratio: 5 * ratio.MilliTokensUsd, CompletionRatio: 25.0 / 5, CachedInputRatio: 0.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 6.25 * ratio.MilliTokensUsd, CacheWrite1hRatio: 10 * ratio.MilliTokensUsd},
		"claude-opus-4-6":            {Ratio: 5 * ratio.MilliTokensUsd, CompletionRatio: 25.0 / 5, CachedInputRatio: 0.5 * ratio.MilliTokensUsd, CacheWrite5mRatio: 6.25 * ratio.MilliTokensUsd, CacheWrite1hRatio: 10 * ratio.MilliTokensUsd},

		// Llama Models on AWS Bedrock
		// Note: Pricing may need to be updated later; also this model is significantly faster on AWS GPUs.
		// Llama 4 models - Pricing given per 1K tokens; normalize to $/1M then to $/token via ratio.MilliTokensUsd
		"llama4-maverick-17b-1m": {Ratio: 0.24 * ratio.MilliTokensUsd, CompletionRatio: 4.04}, // $0.24/$0.97 per 1M → $/token
		"llama4-scout-17b-3.5m":  {Ratio: 0.17 * ratio.MilliTokensUsd, CompletionRatio: 3.88}, // $0.17/$0.66 per 1M → $/token

		// Llama 3.3 models
		"llama3-3-70b-128k": {Ratio: 0.72 * ratio.MilliTokensUsd, CompletionRatio: 1}, // $0.72/$0.72 per 1M → $/token

		// Llama 3.2 models
		"llama3-2-1b-131k":         {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 1},  // $0.1/$0.1 per 1M → $/token
		"llama3-2-3b-131k":         {Ratio: 0.15 * ratio.MilliTokensUsd, CompletionRatio: 1}, // $0.15/$0.15 per 1M → $/token
		"llama3-2-11b-vision-131k": {Ratio: 0.16 * ratio.MilliTokensUsd, CompletionRatio: 1}, // $0.16/$0.16 per 1M → $/token
		"llama3-2-90b-128k":        {Ratio: 0.72 * ratio.MilliTokensUsd, CompletionRatio: 1}, // $0.72/$0.72 per 1M → $/token

		// Llama 3.1 models
		"llama3-1-8b-128k":  {Ratio: 0.22 * ratio.MilliTokensUsd, CompletionRatio: 1}, // $0.22/$0.22 per 1M → $/token
		"llama3-1-70b-128k": {Ratio: 0.72 * ratio.MilliTokensUsd, CompletionRatio: 1}, // $0.72/$0.72 per 1M → $/token

		// Llama 3 models
		"llama3-8b-8192":  {Ratio: 0.3 * ratio.MilliTokensUsd, CompletionRatio: 2},     // $0.3/$0.6 per 1M → $/token
		"llama3-70b-8192": {Ratio: 2.65 * ratio.MilliTokensUsd, CompletionRatio: 1.32}, // $2.65/$3.5 per 1M → $/token
		// Amazon Nova Models (if supported)
		"amazon-nova-micro":   {Ratio: 0.035 * ratio.MilliTokensUsd, CompletionRatio: 4.28}, // $0.035/$0.15 per 1M tokens
		"amazon-nova-lite":    {Ratio: 0.06 * ratio.MilliTokensUsd, CompletionRatio: 4.17},  // $0.06/$0.25 per 1M tokens
		"amazon-nova-pro":     {Ratio: 0.8 * ratio.MilliTokensUsd, CompletionRatio: 4},      // $0.8/$3.2 per 1M tokens
		"amazon-nova-premier": {Ratio: 2.4 * ratio.MilliTokensUsd, CompletionRatio: 4.17},   // $2.4/$10 per 1M tokens

		// Titan Models (if supported)
		"amazon-titan-text-lite":    {Ratio: 0.3 * ratio.MilliTokensUsd, CompletionRatio: 1.33}, // $0.3/$0.4 per 1M tokens
		"amazon-titan-text-express": {Ratio: 0.8 * ratio.MilliTokensUsd, CompletionRatio: 2},    // $0.8/$1.6 per 1M tokens
		"amazon-titan-embed-text":   {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 1},    // $0.1 per 1M tokens

		// Cohere Models (Supported) - Note: These are per 1K tokens, converted to 1M tokens using ratio.MilliTokensUsd
		"command-r":      {Ratio: 0.5 * ratio.MilliTokensUsd, CompletionRatio: 3}, // $0.5/$2 per 1M tokens
		"command-r-plus": {Ratio: 3 * ratio.MilliTokensUsd, CompletionRatio: 5},   // $3/$5 per 1M tokens

		// Qwen Models (Supported) - Note: These are per 1K tokens, converted to 1M tokens using MilliTokensUsd
		"qwen3-235b":       {Ratio: 0.22 * ratio.MilliTokensUsd, CompletionRatio: 4},    // $0.00022/$0.00088 per 1K tokens = $0.22/$0.88 per 1M tokens
		"qwen3-32b":        {Ratio: 0.15 * ratio.MilliTokensUsd, CompletionRatio: 4},    // $0.00015/$0.0006 per 1K tokens = $0.15/$0.6 per 1M tokens
		"qwen3-coder-30b":  {Ratio: 0.15 * ratio.MilliTokensUsd, CompletionRatio: 4},    // $0.00015/$0.0006 per 1K tokens = $0.15/$0.6 per 1M tokens
		"qwen3-coder-480b": {Ratio: 0.22 * ratio.MilliTokensUsd, CompletionRatio: 8.18}, // $0.00022/$0.0018 per 1K tokens = $0.22/$1.8 per 1M tokens

		// AI21 Models (if supported)
		"ai21-j2-mid":    {Ratio: 12.5 * ratio.MilliTokensUsd, CompletionRatio: 1}, // $12.5 per 1M tokens
		"ai21-j2-ultra":  {Ratio: 18.8 * ratio.MilliTokensUsd, CompletionRatio: 1}, // $18.8 per 1M tokens
		"ai21-jamba-1.5": {Ratio: 2 * ratio.MilliTokensUsd, CompletionRatio: 4},    // $2/$8 per 1M tokens

		// DeepSeek Models (Supported) - Updated pricing as of 2025-09-19 - Note: These are per 1K tokens, converted to 1M tokens using MilliTokensUsd
		"deepseek-r1":   {Ratio: 1.35 * ratio.MilliTokensUsd, CompletionRatio: 4},   // $0.00135/$0.0054 per 1K tokens = $1.35/$5.4 per 1M tokens
		"deepseek-v3.1": {Ratio: 0.58 * ratio.MilliTokensUsd, CompletionRatio: 2.9}, // $0.00058/$0.00168 per 1K tokens = $0.58/$1.68 per 1M tokens

		// Mistral Models (Supported) - Updated pricing as of 2025-08-27 - Note: These are per 1K tokens, converted to 1M tokens using ratio.MilliTokensUsd
		//
		// Note: The Mistral Instruct model (mistral-7b, mixtral-8x7b) is currently considered legacy and unsupported.
		// Only the newest/latest models available in AWS Bedrock are supported.
		"mistral-7b-instruct":        {Ratio: 0.15 * ratio.MilliTokensUsd, CompletionRatio: 1.33}, // $0.15/$0.2 per 1M tokens
		"mistral-8x7b-instruct":      {Ratio: 0.45 * ratio.MilliTokensUsd, CompletionRatio: 1.56}, // $0.45/$0.7 per 1M tokens
		"mistral-large":              {Ratio: 4 * ratio.MilliTokensUsd, CompletionRatio: 3},       // $4/$12 per 1M tokens
		"mistral-7b":                 {Ratio: 0.15 * ratio.MilliTokensUsd, CompletionRatio: 1.33}, // $0.00015/$0.0002 per 1K tokens = $0.15/$0.2 per 1M tokens
		"mixtral-8x7b":               {Ratio: 0.45 * ratio.MilliTokensUsd, CompletionRatio: 1.56}, // $0.00045/$0.0007 per 1K tokens = $0.45/$0.7 per 1M tokens
		"mistral-small-2402":         {Ratio: 1 * ratio.MilliTokensUsd, CompletionRatio: 3},       // $0.001/$0.003 per 1K tokens = $1/$3 per 1M tokens
		"mistral-large-2402":         {Ratio: 4 * ratio.MilliTokensUsd, CompletionRatio: 3},       // $0.004/$0.012 per 1K tokens = $4/$12 per 1M tokens
		"mistral-pixtral-large-2502": {Ratio: 2 * ratio.MilliTokensUsd, CompletionRatio: 3},       // $0.002/$0.006 per 1K tokens = $2/$6 per 1M tokens

		// OpenAI OSS Models (Supported) - Updated pricing as of 2025-09-13 - Note: These are per 1K tokens, converted to 1M tokens using ratio.MilliTokensUsd
		// These models work similarly to DeepSeek-R1 with reasoning content and use converse method
		"gpt-oss-20b":  {Ratio: 0.07 * ratio.MilliTokensUsd, CompletionRatio: 4.29}, // $0.00007/$0.0003 per 1K tokens = $0.07/$0.3 per 1M tokens
		"gpt-oss-120b": {Ratio: 0.15 * ratio.MilliTokensUsd, CompletionRatio: 4},    // $0.00015/$0.0006 per 1K tokens = $0.15/$0.6 per 1M tokens

		// Writer Models (Supported) - Updated pricing as of 2025-09-17 - Note: These are per 1K tokens, converted to 1M tokens using MilliTokensUsd
		// Simple text/chat models without reasoning content support.
		"palmyra-x4": {Ratio: 2.5 * ratio.MilliTokensUsd, CompletionRatio: 4},  // $0.0025/$0.010 per 1K tokens = $2.5/$10 per 1M tokens
		"palmyra-x5": {Ratio: 0.6 * ratio.MilliTokensUsd, CompletionRatio: 10}, // $0.0006/$0.006 per 1K tokens = $0.6/$6 per 1M tokens
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

	// Do not treat max_completion_tokens as unsupported; we'll map it to max_tokens if needed

	// Check for stop support
	if request.Stop != nil && !capabilities.SupportsStop {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "stop",
			Description: "Stop parameter is not supported by this model",
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
				Message:  errorMessage,
				Type:     model.ErrorTypeInvalidRequest,
				Code:     "unsupported_parameter",
				RawError: errors.New(errorMessage),
			},
		}
	}

	return nil
}
