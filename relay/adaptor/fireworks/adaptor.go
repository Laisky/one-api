// Package fireworks implements the adaptor for Fireworks AI's serverless inference
// API (https://docs.fireworks.ai/api-reference/introduction).
//
// Fireworks exposes all surfaces at base URL https://api.fireworks.ai/inference
// with a shared "Authorization: Bearer <api_key>" auth scheme:
//
//   - /v1/chat/completions — OpenAI-compatible chat completions
//   - /v1/completions      — OpenAI-compatible text completions
//   - /v1/embeddings       — OpenAI-compatible embeddings
//   - /v1/responses        — OpenAI-compatible Responses API
//   - /v1/messages         — Anthropic-compatible Messages API
//   - /v1/rerank           — OpenAI-style list envelope with relevance scores
//
// Every surface is forwarded natively: chat/completions/embeddings/responses
// reuse the OpenAI-compatible request and response shape, and Claude Messages
// requests are proxied verbatim to Fireworks' /v1/messages endpoint via the
// ClaudeDirectPassthrough flag so the controller streams Anthropic SSE events
// without any conversion.
//
// Model IDs must be the full resource name, e.g.
// accounts/fireworks/models/kimi-k2p5.
package fireworks

import (
	"io"
	"net/http"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/adaptor/openai_compatible"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

type Adaptor struct {
	adaptor.DefaultPricingMethods
}

func (a *Adaptor) Init(meta *meta.Meta) {}

func (a *Adaptor) GetChannelName() string {
	return "fireworks"
}

func (a *Adaptor) GetModelList() []string {
	return adaptor.GetModelListFromPricing(ModelRatios)
}

func (a *Adaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	return ModelRatios
}

func (a *Adaptor) DefaultToolingConfig() adaptor.ChannelToolConfig {
	return FireworksToolingDefaults
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

// GetRequestURL forwards the original relay path verbatim to Fireworks. The
// default BaseURL is https://api.fireworks.ai/inference, and every surface
// (/v1/chat/completions, /v1/embeddings, /v1/responses, /v1/messages) is
// reachable at BaseURL + path.
func (a *Adaptor) GetRequestURL(meta *meta.Meta) (string, error) {
	return openai_compatible.GetFullRequestURL(meta.BaseURL, meta.RequestURLPath, meta.ChannelType), nil
}

// SetupRequestHeader applies Fireworks' Bearer-token auth, which is shared
// across every surface (chat, responses, embeddings, Claude Messages).
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) error {
	adaptor.SetupCommonRequestHeader(c, req, meta)
	req.Header.Set("Authorization", "Bearer "+meta.APIKey)
	return nil
}

// ConvertRequest passes OpenAI-style requests through unchanged. Fireworks is a
// superset of the OpenAI chat/completions/embeddings/responses schema, so no
// normalization is required for the common cases.
func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return request, nil
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, request *model.ImageRequest) (any, error) {
	// Fireworks image generation uses per-model workflow routes
	// (/v1/workflows/<model>/text_to_image) that differ from OpenAI's shape, so
	// we do not attempt a generic conversion here. Users wanting image output
	// can target those endpoints directly through a Proxy-type channel.
	return nil, errors.New("fireworks image generation is not supported via this adaptor; use the per-model text_to_image workflow endpoint directly")
}

// ConvertClaudeRequest enables native passthrough to Fireworks' Anthropic-
// compatible /v1/messages endpoint. Fireworks speaks the Anthropic Messages
// schema natively, so forwarding the raw request body preserves tool_use,
// thinking blocks, streaming events, and cache_control annotations without any
// lossy conversion.
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	c.Set(ctxkey.ClaudeModel, request.Model)
	c.Set(ctxkey.ClaudeMessagesNative, true)
	c.Set(ctxkey.ClaudeDirectPassthrough, true)
	return request, nil
}

// ConvertRerankRequest forwards the canonical rerank DTO unchanged. Fireworks'
// POST /v1/rerank speaks the same {model, query, documents, top_n,
// return_documents} schema that one-api uses internally, so no translation is
// needed — we just validate that a request was actually supplied.
func (a *Adaptor) ConvertRerankRequest(c *gin.Context, request *model.RerankRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return request, nil
}

func (a *Adaptor) DoRequest(c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	return adaptor.DoRequestHelper(a, c, meta, requestBody)
}

// DoResponse dispatches upstream response handling per relay mode. Claude
// Messages requests are handled upstream by the controller via direct
// passthrough, so DoResponse only needs to cover OpenAI-compatible surfaces.
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	switch meta.Mode {
	case relaymode.Rerank:
		err, usage = handleRerankResponse(c, resp, meta.PromptTokens)
		return
	case relaymode.Embeddings:
		err, usage = openai_compatible.EmbeddingHandler(c, resp)
		return
	case relaymode.ResponseAPI:
		if meta.IsStream {
			var responseText string
			err, responseText, usage = openai.ResponseAPIDirectStreamHandler(c, resp, meta.Mode)
			if usage == nil || usage.TotalTokens == 0 {
				usage = openai.ResponseText2Usage(responseText, meta.ActualModelName, meta.PromptTokens)
			}
			return
		}
		err, usage = openai.ResponseAPIDirectHandler(c, resp, meta.PromptTokens, meta.ActualModelName)
		return
	}

	if meta.IsStream {
		err, usage = openai_compatible.StreamHandler(c, resp, meta.PromptTokens, meta.ActualModelName)
		return
	}
	err, usage = openai_compatible.Handler(c, resp, meta.PromptTokens, meta.ActualModelName)
	return
}
