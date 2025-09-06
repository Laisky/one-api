package openrouter

import (
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/adaptor/openai_compatible"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
)

// Adaptor represents the OpenRouter adapter implementation.
// It embeds DefaultPricingMethods to provide fallback pricing behavior
// for models that don't have specific OpenRouter pricing defined.
// This struct implements the adaptor.Adaptor interface to handle
// OpenRouter-specific API calls, authentication, and response processing.
type Adaptor struct {
	// DefaultPricingMethods provides fallback pricing methods for models
	// that don't have specific OpenRouter pricing configurations.
	// This ensures that even unknown models have reasonable default pricing.
	adaptor.DefaultPricingMethods
}

// Init initializes the OpenRouter adapter with the provided metadata.
// OpenRouter doesn't require any special initialization, so this method
// is intentionally empty. The adapter is ready to use immediately after creation.
//
// Parameters:
//   - meta: Request metadata containing channel configuration, API keys, and other context
func (a *Adaptor) Init(meta *meta.Meta) {}

// GetRequestURL constructs the appropriate request URL for OpenRouter API calls.
// This method handles URL transformation for different API endpoints and provides
// special handling for Claude Messages API requests by converting them to OpenAI format.
//
// Parameters:
//   - meta: Request metadata containing base URL, request path, and channel type
//
// Returns:
//   - string: The complete URL for the OpenRouter API request
//   - error: Any error encountered during URL construction
func (a *Adaptor) GetRequestURL(meta *meta.Meta) (string, error) {
	// Handle Claude Messages requests - convert to OpenAI Chat Completions endpoint
	// This allows Claude API users to seamlessly use OpenRouter without changing their code
	requestPath := meta.RequestURLPath

	// Strip query parameters from the path to get clean endpoint matching
	if idx := strings.Index(requestPath, "?"); idx >= 0 {
		requestPath = requestPath[:idx]
	}

	// Check if this is a Claude Messages API request that needs conversion
	if requestPath == "/v1/messages" {
		// Claude Messages requests should use OpenAI's chat completions endpoint
		// OpenRouter supports Claude models through the OpenAI-compatible interface
		chatCompletionsPath := "/v1/chat/completions"
		return openai_compatible.GetFullRequestURL(meta.BaseURL, chatCompletionsPath, meta.ChannelType), nil
	}

	// OpenRouter uses OpenAI-compatible API endpoints for all other requests
	// This includes chat completions, embeddings, and other standard OpenAI endpoints
	return openai_compatible.GetFullRequestURL(meta.BaseURL, meta.RequestURLPath, meta.ChannelType), nil
}

// SetupRequestHeader configures the HTTP headers required for OpenRouter API requests.
// This method sets up authentication and other necessary headers for successful API calls.
//
// Parameters:
//   - c: Gin context containing the original client request information
//   - req: HTTP request object that will be sent to OpenRouter
//   - meta: Request metadata containing API key and other configuration
//
// Returns:
//   - error: Any error encountered during header setup (always nil for OpenRouter)
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) error {
	// Apply common headers like Content-Type, Accept, and any X- prefixed custom headers
	adaptor.SetupCommonRequestHeader(c, req, meta)

	// Set the Authorization header with Bearer token format required by OpenRouter
	// OpenRouter uses standard Bearer token authentication with the API key
	req.Header.Set("Authorization", "Bearer "+meta.APIKey)
	// Note: This may need to be modified for the identifier "openrouter".
	req.Header.Set("HTTP-Referer", "https://github.com/Laisky/one-api")
	req.Header.Set("X-Title", config.SystemName) // use system name

	// OpenRouter header setup never fails, so we always return nil
	return nil
}

// ConvertRequest transforms the incoming request for OpenRouter API compatibility.
// Since OpenRouter uses OpenAI-compatible API format, most requests can pass through unchanged.
//
// Parameters:
//   - c: Gin context for request processing
//   - relayMode: The relay mode indicating the type of request being processed
//   - request: The standardized OpenAI request structure
//
// Returns:
//   - any: The converted request object (unchanged for OpenRouter)
//   - error: Any error encountered during conversion (always nil for standard requests)
func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	// OpenRouter is OpenAI-compatible, so we can pass the request through unchanged
	// No transformation is needed as OpenRouter accepts standard OpenAI request format
	return request, nil
}

// ConvertImageRequest handles image generation requests for OpenRouter.
// OpenRouter supports image generation through compatible models, so requests pass through.
//
// Parameters:
//   - c: Gin context for request processing
//   - request: The image generation request structure
//
// Returns:
//   - any: The converted image request (unchanged for OpenRouter)
//   - error: Any error encountered during conversion (always nil for image requests)
func (a *Adaptor) ConvertImageRequest(c *gin.Context, request *model.ImageRequest) (any, error) {
	// OpenRouter supports image generation through compatible models like DALL-E
	// Pass through the request unchanged as OpenRouter uses OpenAI-compatible format
	return request, nil
}

// ConvertClaudeRequest transforms Claude Messages API requests to OpenAI format.
// This enables seamless use of Claude models through OpenRouter's unified interface.
//
// Parameters:
//   - c: Gin context for request processing
//   - request: The Claude Messages API request structure
//
// Returns:
//   - any: The converted request in OpenAI format
//   - error: Any error encountered during the conversion process
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error) {
	// Use the shared OpenAI-compatible Claude Messages conversion utility
	// This converts Claude's message format to OpenAI's chat completion format
	// allowing Claude models to work seamlessly through OpenRouter
	return openai_compatible.ConvertClaudeRequest(c, request)
}

// DoRequest performs the actual HTTP request to OpenRouter.
// This method is a wrapper around the adaptor.DoRequestHelper function,
// providing a standardized way to make requests to OpenRouter.
//
// Parameters:
//   - c: Gin context for request processing
//   - meta: Request metadata containing channel configuration, API keys, and other context
//   - requestBody: The request body to be sent to OpenRouter
//
// Returns:
//   - *http.Response: The HTTP response received from OpenRouter
//   - error: Any error encountered during the request, such as network errors or timeouts
func (a *Adaptor) DoRequest(c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	return adaptor.DoRequestHelper(a, c, meta, requestBody)
}

// DoResponse processes the response from OpenRouter and converts it to the standardized format.
// This method handles both streaming and non-streaming responses, and it extracts usage information.
//
// Parameters:
//   - c: Gin context for request processing
//   - resp: HTTP response from OpenRouter
//   - meta: Request metadata containing channel configuration and other context
//
// Returns:
//   - *model.Usage: Extracted usage information including token counts
//   - *model.ErrorWithStatusCode: Any error encountered during response processing, wrapped with HTTP status code
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	return openai_compatible.HandleClaudeMessagesResponse(c, resp, meta, func(c *gin.Context, resp *http.Response, promptTokens int, modelName string) (*model.ErrorWithStatusCode, *model.Usage) {
		if meta.IsStream {
			return openai_compatible.StreamHandler(c, resp, promptTokens, modelName)
		}
		return openai_compatible.Handler(c, resp, promptTokens, modelName)
	})
}

// GetModelList returns the list of all models supported by this OpenRouter adapter.
// The model list is derived from the pricing configuration to ensure consistency
// between available models and their pricing information.
//
// Returns:
//   - []string: Array of model names that can be used with OpenRouter
func (a *Adaptor) GetModelList() []string {
	// Generate model list from pricing map keys to ensure all priced models are available
	// This eliminates the need for duplicate model lists and ensures pricing consistency
	return adaptor.GetModelListFromPricing(a.GetDefaultModelPricing())
}

// GetChannelName returns the identifier string for this adapter type.
// This name is used for logging, monitoring, and channel identification purposes.
//
// Returns:
//   - string: The channel name "openrouter"
func (a *Adaptor) GetChannelName() string {
	// Return the standard identifier for OpenRouter channels
	return "openrouter"
}

// GetDefaultModelPricing returns the comprehensive pricing information for OpenRouter models.
// This includes 232+ models from multiple providers with accurate pricing ratios and
// completion multipliers based on OpenRouter's actual pricing structure.
//
// The pricing data reflects:
//   - Input token costs (Ratio field)
//   - Output token multipliers (CompletionRatio field)
//   - Provider-specific pricing differences
//   - Free tier and premium model distinctions
//
// Returns:
//   - map[string]adaptor.ModelConfig: Complete pricing configuration for all supported models
//
// Reference: https://openrouter.ai/models
func (a *Adaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	// Use the constants.go ModelRatios which contains the complete OpenRouter pricing database
	// This data is maintained separately to keep pricing information organized and updatable
	return ModelRatios
}

// GetModelRatio retrieves the input token pricing ratio for a specific model.
// This method implements a two-tier lookup system with fallback to default pricing.
//
// Parameters:
//   - modelName: The name of the model to get pricing for (e.g., "openai/gpt-4o")
//
// Returns:
//   - float64: The pricing ratio for input tokens (cost per milli-token)
//
// Lookup order:
//  1. Check OpenRouter-specific pricing from constants.go
//  2. Fall back to DefaultPricingMethods if model not found
func (a *Adaptor) GetModelRatio(modelName string) float64 {
	// Get the complete pricing database for this adapter
	pricing := a.GetDefaultModelPricing()

	// Check if we have specific OpenRouter pricing for this model
	if price, exists := pricing[modelName]; exists {
		// Return the OpenRouter-specific input token ratio
		return price.Ratio
	}

	// Use default fallback pricing from DefaultPricingMethods
	// This ensures unknown models still have reasonable pricing
	return a.DefaultPricingMethods.GetModelRatio(modelName)
}

// GetCompletionRatio retrieves the output token pricing multiplier for a specific model.
// This represents how much more expensive output tokens are compared to input tokens.
//
// Parameters:
//   - modelName: The name of the model to get completion ratio for
//
// Returns:
//   - float64: The completion ratio multiplier (e.g., 4.0 means output costs 4x input)
//
// Completion ratios vary by provider:
//   - OpenAI models: 2.0-4.0x (newer models have higher ratios)
//   - Anthropic models: 5.0x (consistent across Claude models)
//   - Meta models: 1.0x (equal input/output pricing)
//   - Free models: 1.0x (no output premium)
func (a *Adaptor) GetCompletionRatio(modelName string) float64 {
	// Get the complete pricing database for this adapter
	pricing := a.GetDefaultModelPricing()

	// Check if we have specific OpenRouter completion ratio for this model
	if price, exists := pricing[modelName]; exists {
		// Return the OpenRouter-specific completion ratio
		return price.CompletionRatio
	}

	// Use default fallback completion ratio from DefaultPricingMethods
	// This ensures unknown models still have reasonable output pricing
	return a.DefaultPricingMethods.GetCompletionRatio(modelName)
}
