package xai

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// ResponseAPIInputTokensDetails models the nested usage block returned by the OpenAI Response API.
// The schema is not stable yet (especially for web-search fields), so we keep a map of additional
// properties while still projecting the common fields into strong types.
type ResponseAPIInputTokensDetails struct {
	CachedTokens int `json:"cached_tokens,omitempty"`
	AudioTokens  int `json:"audio_tokens,omitempty"`
	TextTokens   int `json:"text_tokens,omitempty"`
	ImageTokens  int `json:"image_tokens,omitempty"`
	WebSearch    any `json:"web_search,omitempty"`
	additional   map[string]any
}

// ResponseAPIOutputTokensDetails models the completion-side usage details returned by the Response API.
type ResponseAPIOutputTokensDetails struct {
	ReasoningTokens          int `json:"reasoning_tokens,omitempty"`
	AudioTokens              int `json:"audio_tokens,omitempty"`
	AcceptedPredictionTokens int `json:"accepted_prediction_tokens,omitempty"`
	RejectedPredictionTokens int `json:"rejected_prediction_tokens,omitempty"`
	TextTokens               int `json:"text_tokens,omitempty"`
	CachedTokens             int `json:"cached_tokens,omitempty"`
	additional               map[string]any
}

// ResponseAPIUsage represents the token usage information structure for x.AI Response API responses.
// It contains input, output, and total token counts plus detailed breakdowns for billing and monitoring purposes.
type ResponseAPIUsage struct {
	InputTokens         int                             `json:"input_tokens"`
	OutputTokens        int                             `json:"output_tokens"`
	TotalTokens         int                             `json:"total_tokens"`
	InputTokensDetails  *ResponseAPIInputTokensDetails  `json:"input_tokens_details,omitempty"`
	OutputTokensDetails *ResponseAPIOutputTokensDetails `json:"output_tokens_details,omitempty"`
}

// ResponseAPIResponse represents the minimal structure for x.AI Response API responses.
// It contains usage information for billing purposes.
type ResponseAPIResponse struct {
	Usage *ResponseAPIUsage `json:"usage,omitempty"`
}

// Adaptor implements the relay adaptor interface for x.AI API.
// It handles request routing, conversion, and response processing for all x.AI supported modes.
type Adaptor struct {
	adaptor.DefaultPricingMethods
}

// ImageData represents a single image data item in OpenAI-compatible image responses.
// It contains the image URL, base64 data, and revised prompt information.
type ImageData struct {
	URL           string `json:"url,omitempty"`
	B64Json       string `json:"b64_json,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

// ImageResponse represents the OpenAI-compatible image generation response structure.
// It contains the creation timestamp and array of generated images.
type ImageResponse struct {
	Created int64       `json:"created"`
	Data    []ImageData `json:"data"`
}

// ToModelUsage converts ResponseAPIUsage to the standard model.Usage format.
// Returns nil if the ResponseAPIUsage instance is nil.
func (r *ResponseAPIUsage) ToModelUsage() *model.Usage {
	if r == nil {
		return nil
	}

	usage := &model.Usage{
		PromptTokens:     r.InputTokens,
		CompletionTokens: r.OutputTokens,
		TotalTokens:      r.TotalTokens,
	}
	usage.PromptTokensDetails = r.InputTokensDetails.toModel()
	usage.CompletionTokensDetails = r.OutputTokensDetails.toModel()
	return usage
}

// toModel converts ResponseAPIInputTokensDetails to model.UsagePromptTokensDetails.
func (d *ResponseAPIInputTokensDetails) toModel() *model.UsagePromptTokensDetails {
	if d == nil {
		return nil
	}

	details := &model.UsagePromptTokensDetails{
		CachedTokens: d.CachedTokens,
		AudioTokens:  d.AudioTokens,
		TextTokens:   d.TextTokens,
		ImageTokens:  d.ImageTokens,
	}
	return details
}

// toModel converts ResponseAPIOutputTokensDetails to model.UsageCompletionTokensDetails.
func (d *ResponseAPIOutputTokensDetails) toModel() *model.UsageCompletionTokensDetails {
	if d == nil {
		return nil
	}

	return &model.UsageCompletionTokensDetails{
		ReasoningTokens:          d.ReasoningTokens,
		AudioTokens:              d.AudioTokens,
		AcceptedPredictionTokens: d.AcceptedPredictionTokens,
		RejectedPredictionTokens: d.RejectedPredictionTokens,
		TextTokens:               d.TextTokens,
		CachedTokens:             d.CachedTokens,
	}
}

// UnmarshalJSON customizes JSON unmarshaling for ResponseAPIInputTokensDetails.
func (d *ResponseAPIInputTokensDetails) UnmarshalJSON(data []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return errors.Wrap(err, "unmarshal xai input tokens details")
	}

	// Reset existing values so the struct can be reused.
	*d = ResponseAPIInputTokensDetails{}
	if len(raw) == 0 {
		return nil
	}

	additional := make(map[string]any)
	for key, value := range raw {
		switch key {
		case "cached_tokens":
			if v, ok := value.(float64); ok {
				d.CachedTokens = int(v)
			}
		case "audio_tokens":
			if v, ok := value.(float64); ok {
				d.AudioTokens = int(v)
			}
		case "text_tokens":
			if v, ok := value.(float64); ok {
				d.TextTokens = int(v)
			}
		case "image_tokens":
			if v, ok := value.(float64); ok {
				d.ImageTokens = int(v)
			}
		case "web_search":
			d.WebSearch = value
		default:
			additional[key] = value
		}
	}

	if len(additional) > 0 {
		d.additional = additional
	}

	return nil
}

// UnmarshalJSON customizes JSON unmarshaling for ResponseAPIOutputTokensDetails.
func (d *ResponseAPIOutputTokensDetails) UnmarshalJSON(data []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return errors.Wrap(err, "unmarshal xai output tokens details")
	}

	*d = ResponseAPIOutputTokensDetails{}
	if len(raw) == 0 {
		return nil
	}

	additional := make(map[string]any)
	for key, value := range raw {
		switch key {
		case "reasoning_tokens":
			if v, ok := value.(float64); ok {
				d.ReasoningTokens = int(v)
			}
		case "audio_tokens":
			if v, ok := value.(float64); ok {
				d.AudioTokens = int(v)
			}
		case "accepted_prediction_tokens":
			if v, ok := value.(float64); ok {
				d.AcceptedPredictionTokens = int(v)
			}
		case "rejected_prediction_tokens":
			if v, ok := value.(float64); ok {
				d.RejectedPredictionTokens = int(v)
			}
		case "text_tokens":
			if v, ok := value.(float64); ok {
				d.TextTokens = int(v)
			}
		case "cached_tokens":
			if v, ok := value.(float64); ok {
				d.CachedTokens = int(v)
			}
		default:
			additional[key] = value
		}
	}

	if len(additional) > 0 {
		d.additional = additional
	}

	return nil
}

// Init initializes the x.AI adaptor with the provided metadata.
// Currently, no initialization is required for x.AI.
func (a *Adaptor) Init(meta *meta.Meta) {}

// GetRequestURL constructs the appropriate API endpoint URL based on the request path and mode.
// It handles routing for Chat Completions, Claude Messages, Response API, and other x.AI endpoints.
// Returns the full URL with base URL and path, or an error if URL construction fails.
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
	if requestPath == "/v1/responses" {
		// XAI supports Response API natively - preserve query parameters
		return openai_compatible.GetFullRequestURL(meta.BaseURL, meta.RequestURLPath, meta.ChannelType), nil
	}

	// XAI uses OpenAI-compatible API endpoints
	return openai_compatible.GetFullRequestURL(meta.BaseURL, meta.RequestURLPath, meta.ChannelType), nil
}

// SetupRequestHeader configures the HTTP request headers for X.AI API calls.
// It sets up common headers including authorization with the API key.
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) error {
	adaptor.SetupCommonRequestHeader(c, req, meta)
	req.Header.Set("Authorization", "Bearer "+meta.APIKey)
	return nil
}

// ConvertRequest converts and validates OpenAI-compatible requests for x.AI.
// It removes unsupported parameters like reasoning_effort and adjusts model-specific parameters.
// Returns the modified request or an error if conversion fails.
func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	// XAI is OpenAI-compatible, so we can pass the request through with minimal changes
	// Remove reasoning_effort as XAI doesn't support it
	if request.ReasoningEffort != nil {
		request.ReasoningEffort = nil
	}
	// Remove presence_penalty and frequency_penalty for certain grok-4 models as they don't support them
	switch request.Model {
	case "grok-4-0709", "grok-4.20-0309-reasoning", "grok-4.20-0309-non-reasoning", "grok-4.20-multi-agent-0309", "grok-4-1-fast-reasoning", "grok-4-1-fast-non-reasoning":
		if request.PresencePenalty != nil {
			request.PresencePenalty = nil
		}
		if request.FrequencyPenalty != nil {
			request.FrequencyPenalty = nil
		}
	}
	return request, nil
}

// ConvertImageRequest converts and validates image generation requests for x.AI.
// It ensures correct model naming and removes unsupported parameters like quality, size, and style.
// Returns the modified request or an error if conversion fails.
func (a *Adaptor) ConvertImageRequest(c *gin.Context, request *model.ImageRequest) (any, error) {
	// XAI supports image generation with grok-2-image model
	// The API is OpenAI-compatible, so we can pass the request through with minimal changes

	// Ensure we're using the correct model name for xAI
	if request.Model == "grok-2-image" {
		// XAI API uses grok-2-image as the model name
		request.Model = "grok-2-image"
	}

	// XAI doesn't support quality, size, or style parameters according to their docs
	// Remove unsupported parameters
	request.Quality = ""
	request.Size = ""
	request.Style = ""

	return request, nil
}

// ConvertClaudeRequest converts Claude Messages API requests to OpenAI-compatible format.
// It uses the shared OpenAI-compatible Claude Messages conversion logic.
// Returns the converted request or an error if conversion fails.
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error) {
	// Use the shared OpenAI-compatible Claude Messages conversion
	return openai_compatible.ConvertClaudeRequest(c, request)
}

// DoRequest sends the HTTP request to the x.AI API endpoint.
// It uses the common request helper for standard HTTP handling.
// Returns the HTTP response or an error if the request fails.
func (a *Adaptor) DoRequest(c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	return adaptor.DoRequestHelper(a, c, meta, requestBody)
}

// DoResponse processes HTTP responses from x.AI API based on the relay mode.
// It handles different response types including images, Response API, and Claude Messages.
// Returns usage information and any errors encountered during processing.
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	// Handle image generation requests differently
	if meta.Mode == relaymode.ImagesGenerations {
		// TODO: Do we need a meta tag to include the actual model name for this image generation?
		return a.handleImageResponse(c, resp)
	}

	// Handle Response API requests - XAI supports Response API natively
	if meta.Mode == relaymode.ResponseAPI {
		return a.handleResponseAPIResponse(c, resp, meta)
	}

	return openai_compatible.HandleClaudeMessagesResponse(c, resp, meta, func(c *gin.Context, resp *http.Response, promptTokens int, modelName string) (*model.ErrorWithStatusCode, *model.Usage) {
		if meta.IsStream {
			return openai_compatible.StreamHandler(c, resp, promptTokens, modelName)
		}
		return openai_compatible.Handler(c, resp, promptTokens, modelName)
	})
}

// handleResponseAPIResponse processes xAI Response API responses and extracts usage information.
// It handles both streaming and non-streaming Response API responses, passing them through
// to the client while extracting billing information from the usage field.
func (a *Adaptor) handleResponseAPIResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	// For streaming Response API, pass through directly
	if meta.IsStream {
		// Copy response to client
		for key, values := range resp.Header {
			for _, value := range values {
				c.Writer.Header().Add(key, value)
			}
		}
		if resp.Header.Get("Content-Type") == "" {
			c.Writer.Header().Set("Content-Type", "text/event-stream")
		}
		c.Writer.WriteHeader(resp.StatusCode)
		if _, copyErr := io.Copy(c.Writer, resp.Body); copyErr != nil {
			return nil, openai_compatible.ErrorWrapper(copyErr, "copy_response_body_failed", http.StatusInternalServerError)
		}
		resp.Body.Close()
		return nil, nil
	}

	// For non-streaming, read the entire response
	responseBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, openai_compatible.ErrorWrapper(errors.Wrap(readErr, "read response body"), "read_response_body_failed", http.StatusInternalServerError)
	}
	resp.Body.Close()

	// Try to parse as Response API response to extract usage
	var responseAPIResp ResponseAPIResponse
	if parseErr := json.Unmarshal(responseBody, &responseAPIResp); parseErr != nil {
		// Non-fatal; continue without usage
	} else if responseAPIResp.Usage != nil {
		usage = responseAPIResp.Usage.ToModelUsage()
	}

	// Copy response to client
	for key, values := range resp.Header {
		for _, value := range values {
			c.Writer.Header().Add(key, value)
		}
	}
	if resp.Header.Get("Content-Type") == "" {
		c.Writer.Header().Set("Content-Type", "application/json")
	}
	c.Writer.WriteHeader(resp.StatusCode)
	if _, writeErr := c.Writer.Write(responseBody); writeErr != nil {
		return nil, openai_compatible.ErrorWrapper(writeErr, "write_response_body_failed", http.StatusInternalServerError)
	}

	return usage, nil
}

// handleImageResponse processes XAI image generation responses and converts them to OpenAI format.
// It parses the x.AI image response, converts it to OpenAI-compatible format,
// and returns the response to the client with appropriate billing information.
func (a *Adaptor) handleImageResponse(c *gin.Context, resp *http.Response) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	// Always close the upstream body
	defer resp.Body.Close()
	// Read the response body
	responseBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, openai_compatible.ErrorWrapper(readErr, "read_response_body_failed", http.StatusInternalServerError)
	}

	// Parse the XAI image response
	var xaiResponse ImageResponse
	if parseErr := json.Unmarshal(responseBody, &xaiResponse); parseErr != nil {
		return nil, openai_compatible.ErrorWrapper(errors.Wrap(parseErr, "parse xai image response"), "parse_response_failed", http.StatusInternalServerError)
	}

	// Convert to OpenAI format
	var imageDataList []ImageData
	for _, xaiData := range xaiResponse.Data {
		imageData := ImageData{
			URL:           xaiData.URL,
			B64Json:       xaiData.B64Json,
			RevisedPrompt: xaiData.RevisedPrompt,
		}
		imageDataList = append(imageDataList, imageData)
	}

	// Create OpenAI-compatible response
	openaiResponse := &ImageResponse{
		Created: helper.GetTimestamp(),
		Data:    imageDataList,
	}

	// Return the response as JSON
	c.JSON(http.StatusOK, openaiResponse)

	// Per-image billing is handled by the controller; no token usage to return.
	return usage, nil
}

// GetModelList returns a list of supported model names for X.AI.
// The list is derived from the pricing configuration in constants.go.
func (a *Adaptor) GetModelList() []string {
	return adaptor.GetModelListFromPricing(ModelRatios)
}

// GetChannelName returns the channel identifier for X.AI.
func (a *Adaptor) GetChannelName() string {
	return "xai"
}

// GetDefaultModelPricing returns the pricing information for XAI models.
// Based on XAI pricing: https://console.x.ai/
// The pricing includes input, output, and cached input token ratios.
func (a *Adaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	// Use the constants.go ModelRatios which already use ratio.MilliTokensUsd correctly
	return ModelRatios
}

// GetModelRatio returns the pricing ratio for input tokens of a specific model.
// Falls back to default pricing methods if the model is not found in X.AI pricing.
func (a *Adaptor) GetModelRatio(modelName string) float64 {
	pricing := a.GetDefaultModelPricing()
	if price, exists := pricing[modelName]; exists {
		return price.Ratio
	}
	// Use default fallback from DefaultPricingMethods
	return a.DefaultPricingMethods.GetModelRatio(modelName)
}

// GetCompletionRatio returns the pricing ratio for output tokens of a specific model.
// Falls back to default pricing methods if the model is not found in X.AI pricing.
func (a *Adaptor) GetCompletionRatio(modelName string) float64 {
	pricing := a.GetDefaultModelPricing()
	if price, exists := pricing[modelName]; exists {
		return price.CompletionRatio
	}
	// Use default fallback from DefaultPricingMethods
	return a.DefaultPricingMethods.GetCompletionRatio(modelName)
}

// DefaultToolingConfig returns xAI tooling defaults (web, X, code execution, and related fees).
func (a *Adaptor) DefaultToolingConfig() adaptor.ChannelToolConfig {
	return XAIToolingDefaults
}
