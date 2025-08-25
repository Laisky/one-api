package xai

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/adaptor/openai_compatible"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

type Adaptor struct {
	adaptor.DefaultPricingMethods
}

// XAI Image Generation Response structures
type XAIImageData struct {
	B64Json       string `json:"b64_json,omitempty"`
	Url           string `json:"url,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

type XAIImageResponse struct {
	Created int64          `json:"created"`
	Data    []XAIImageData `json:"data"`
}

// OpenAI compatible image response structures
type ImageData struct {
	Url           string `json:"url,omitempty"`
	B64Json       string `json:"b64_json,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

type ImageResponse struct {
	Created int64       `json:"created"`
	Data    []ImageData `json:"data"`
}

// Implement required adaptor interface methods (XAI uses OpenAI-compatible API)
func (a *Adaptor) Init(meta *meta.Meta) {}

func (a *Adaptor) GetRequestURL(meta *meta.Meta) (string, error) {
	// Handle Claude Messages requests - convert to OpenAI Chat Completions endpoint
	if meta.RequestURLPath == "/v1/messages" {
		// Claude Messages requests should use OpenAI's chat completions endpoint
		chatCompletionsPath := "/v1/chat/completions"
		return openai_compatible.GetFullRequestURL(meta.BaseURL, chatCompletionsPath, meta.ChannelType), nil
	}

	// XAI uses OpenAI-compatible API endpoints
	return openai_compatible.GetFullRequestURL(meta.BaseURL, meta.RequestURLPath, meta.ChannelType), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) error {
	adaptor.SetupCommonRequestHeader(c, req, meta)
	req.Header.Set("Authorization", "Bearer "+meta.APIKey)
	return nil
}

func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	// XAI is OpenAI-compatible, so we can pass the request through with minimal changes
	// Remove reasoning_effort as XAI doesn't support it
	if request.ReasoningEffort != nil {
		request.ReasoningEffort = nil
	}
	return request, nil
}

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

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error) {
	// Use the shared OpenAI-compatible Claude Messages conversion
	return openai_compatible.ConvertClaudeRequest(c, request)
}

func (a *Adaptor) DoRequest(c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	return adaptor.DoRequestHelper(a, c, meta, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	// Handle image generation requests differently
	if meta.Mode == relaymode.ImagesGenerations {
		// TODO: Do we need a meta tag to include the actual model name for this image generation?
		return a.handleImageResponse(c, resp)
	}

	// Use the shared OpenAI-compatible response handling for text completions
	if meta.IsStream {
		err, usage = openai_compatible.StreamHandler(c, resp, meta.PromptTokens, meta.ActualModelName)
	} else {
		err, usage = openai_compatible.Handler(c, resp, meta.PromptTokens, meta.ActualModelName)
	}
	return
}

// handleImageResponse processes XAI image generation responses and converts them to OpenAI format
func (a *Adaptor) handleImageResponse(c *gin.Context, resp *http.Response) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	// Always close the upstream body
	defer resp.Body.Close()
	// Read the response body
	responseBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, openai_compatible.ErrorWrapper(readErr, "read_response_body_failed", http.StatusInternalServerError)
	}

	// Parse the XAI image response
	var xaiResponse XAIImageResponse
	if parseErr := json.Unmarshal(responseBody, &xaiResponse); parseErr != nil {
		return nil, openai_compatible.ErrorWrapper(parseErr, "parse_response_failed", http.StatusInternalServerError)
	}

	// Convert to OpenAI format
	var imageDataList []ImageData
	for _, xaiData := range xaiResponse.Data {
		imageData := ImageData{
			B64Json:       xaiData.B64Json,
			Url:           xaiData.Url,
			RevisedPrompt: xaiData.RevisedPrompt,
		}
		imageDataList = append(imageDataList, imageData)
	}

	// Create OpenAI-compatible response
	openaiResponse := &ImageResponse{
		Created: helper.GetTimestamp(),
		Data:    imageDataList,
		// Note: The Usage field might be better removed for xAI
	}

	// Return the response as JSON
	c.JSON(http.StatusOK, openaiResponse)

	// Per-image billing is handled by the controller; no token usage to return.
	return usage, nil
}

func (a *Adaptor) GetModelList() []string {
	return adaptor.GetModelListFromPricing(ModelRatios)
}

func (a *Adaptor) GetChannelName() string {
	return "xai"
}

// GetDefaultModelPricing returns the pricing information for XAI models
// Based on XAI pricing: https://console.x.ai/
func (a *Adaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	// Use the constants.go ModelRatios which already use ratio.MilliTokensUsd correctly
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
