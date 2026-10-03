// Package cerebras implements the adaptor for Cerebras Inference
// (https://inference-docs.cerebras.ai), the hosted OpenAI-compatible inference
// service backed by Cerebras' wafer-scale (CS-3) hardware.
//
// Cerebras exposes an OpenAI-compatible surface at base URL
// https://api.cerebras.ai/v1 with a shared "Authorization: Bearer <api_key>"
// auth scheme:
//
//   - /v1/chat/completions - OpenAI-compatible chat completions (streaming SSE,
//     tools / parallel tool calling, JSON mode, structured outputs, and the
//     standard reasoning_effort parameter on reasoning models)
//   - /v1/models           - OpenAI-shaped model listing
//
// Cerebras does NOT serve embeddings, a native Anthropic Messages endpoint, or a
// native Responses API. one-api provides the Responses API and Anthropic
// Messages surfaces through its shared OpenAI-compatible conversion/fallback
// layer, mirroring the NVIDIA / SiliconFlow / Novita adaptors.
//
// Model IDs are the plain slugs shown on each model card, e.g. gpt-oss-120b.
package cerebras

import (
	"io"
	"net/http"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

type Adaptor struct {
	adaptor.DefaultPricingMethods
}

func (a *Adaptor) Init(meta *meta.Meta) {}

// GetRequestURL forwards the original relay path to Cerebras' OpenAI-compatible
// endpoints. The default BaseURL is https://api.cerebras.ai/v1, whose version
// suffix lets GetFullRequestURL collapse the leading /v1 in the relay path so
// the final URL is .../v1/chat/completions (not .../v1/v1/...).
func (a *Adaptor) GetRequestURL(meta *meta.Meta) (string, error) {
	// Claude Messages requests are converted to OpenAI Chat Completions, so they
	// must target the chat completions endpoint.
	if meta.RequestURLPath == "/v1/messages" {
		chatCompletionsPath := "/v1/chat/completions"
		return openai_compatible.GetFullRequestURL(meta.BaseURL, chatCompletionsPath, meta.ChannelType), nil
	}

	return openai_compatible.GetFullRequestURL(meta.BaseURL, meta.RequestURLPath, meta.ChannelType), nil
}

// SetupRequestHeader applies Cerebras' Bearer-token auth.
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) error {
	adaptor.SetupCommonRequestHeader(c, req, meta)
	req.Header.Set("Authorization", "Bearer "+meta.APIKey)
	return nil
}

// ConvertRequest passes OpenAI-style chat requests through unchanged. Cerebras'
// chat completions endpoint is a faithful superset of the OpenAI schema and, in
// particular, accepts reasoning_effort as a standard top-level parameter
// (gpt-oss-120b is a reasoning model), so no normalization is required.
func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return request, nil
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, request *model.ImageRequest) (any, error) {
	return nil, errors.New("cerebras does not support image generation")
}

// ConvertClaudeRequest converts Claude Messages API requests to the
// OpenAI-compatible Chat Completions shape using the shared converter, since
// Cerebras does not expose a native Anthropic Messages endpoint.
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error) {
	return openai_compatible.ConvertClaudeRequest(c, request)
}

func (a *Adaptor) DoRequest(c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	return adaptor.DoRequestHelper(a, c, meta, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	// Cerebras serves only OpenAI-compatible chat completions. Claude Messages
	// and Response API requests are converted into that shape upstream, so route
	// every response through the shared chat handlers (with Claude Messages
	// re-encoding when the original request was an Anthropic Messages call).
	return openai_compatible.HandleClaudeMessagesResponse(c, resp, meta, func(c *gin.Context, resp *http.Response, promptTokens int, modelName string) (*model.ErrorWithStatusCode, *model.Usage) {
		if meta.IsStream {
			return openai_compatible.StreamHandler(c, resp, promptTokens, modelName)
		}
		return openai_compatible.Handler(c, resp, promptTokens, modelName)
	})
}

func (a *Adaptor) GetModelList() []string {
	return adaptor.GetModelListFromPricing(ModelRatios)
}

func (a *Adaptor) GetChannelName() string {
	return "cerebras"
}

// GetDefaultModelPricing returns the per-token pricing for Cerebras models.
func (a *Adaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	return ModelRatios
}

func (a *Adaptor) GetModelRatio(modelName string) float64 {
	if price, exists := ModelRatios[modelName]; exists {
		return price.Ratio
	}
	return a.DefaultPricingMethods.GetModelRatio(modelName)
}

func (a *Adaptor) GetCompletionRatio(modelName string) float64 {
	if price, exists := ModelRatios[modelName]; exists {
		return price.CompletionRatio
	}
	return a.DefaultPricingMethods.GetCompletionRatio(modelName)
}

// DefaultToolingConfig returns Cerebras tooling defaults. Cerebras does not
// publish per-tool metering for the hosted API, so no built-in tool pricing is
// declared.
func (a *Adaptor) DefaultToolingConfig() adaptor.ChannelToolConfig {
	return CerebrasToolingDefaults
}
