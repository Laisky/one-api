// Package nvidia implements the adaptor for NVIDIA's hosted "NVIDIA API
// Catalog" / NIM inference service (https://build.nvidia.com/models).
//
// NVIDIA exposes an OpenAI-compatible surface at base URL
// https://integrate.api.nvidia.com/v1 with a shared
// "Authorization: Bearer nvapi-..." auth scheme:
//
//   - /v1/chat/completions - OpenAI-compatible chat completions (streaming SSE,
//     tools, JSON mode, and vision inputs on capable models)
//
// The Responses API and Anthropic Messages surfaces are not served natively by
// NVIDIA; one-api provides them through its shared OpenAI-compatible
// conversion/fallback layer, mirroring the SiliconFlow / Novita adaptors.
// NVIDIA also exposes some embedding models, but this adaptor does not advertise
// embeddings by default until model-specific request requirements are cataloged.
//
// Model IDs are the slash-namespaced dotted strings shown on each catalog card,
// e.g. nvidia/nemotron-3-ultra-550b-a55b or meta/llama-3.3-70b-instruct.
package nvidia

import (
	"io"
	"net/http"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

type Adaptor struct {
	adaptor.DefaultPricingMethods
}

func (a *Adaptor) Init(meta *meta.Meta) {}

// GetRequestURL forwards the original relay path to NVIDIA's OpenAI-compatible
// endpoints. The default BaseURL is https://integrate.api.nvidia.com/v1, whose
// version suffix lets GetFullRequestURL collapse the leading /v1 in the relay
// path so the final URL is .../v1/chat/completions (not .../v1/v1/...).
func (a *Adaptor) GetRequestURL(meta *meta.Meta) (string, error) {
	// Claude Messages requests are converted to OpenAI Chat Completions, so they
	// must target the chat completions endpoint.
	if meta.RequestURLPath == "/v1/messages" {
		chatCompletionsPath := "/v1/chat/completions"
		return openai_compatible.GetFullRequestURL(meta.BaseURL, chatCompletionsPath, meta.ChannelType), nil
	}

	return openai_compatible.GetFullRequestURL(meta.BaseURL, meta.RequestURLPath, meta.ChannelType), nil
}

// SetupRequestHeader applies NVIDIA's Bearer-token auth, shared across every
// surface (chat, embeddings).
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) error {
	adaptor.SetupCommonRequestHeader(c, req, meta)
	req.Header.Set("Authorization", "Bearer "+meta.APIKey)
	return nil
}

// ConvertRequest lowers a Chat request (or a Responses request converted to
// Chat by the fallback layer) to NVIDIA's hosted chat schema. Parameters are
// the request context (used only for DEBUG logging, may be nil), the relay
// mode and the chat request, which is normalized in place as before. It
// returns the normalized request, or an error for nil input.
//
// NVIDIA's documented effort control is the top-level reasoning_effort field;
// the Responses-style reasoning object is not part of its chat schema. An
// effort supplied only as reasoning.effort is therefore promoted, and the
// reasoning object is dropped. The effort is then kept only when the hosted
// model's catalog lists it (trimmed and lowercased); unlisted values, models
// without a published vocabulary and unknown models omit the field so NVIDIA
// applies its documented default. The hosted endpoint schema, rather than the
// creator's checkpoint capabilities, determines which models advertise efforts.
func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	requested := request.ReasoningEffort
	if requested == nil && request.Reasoning != nil {
		requested = request.Reasoning.Effort
	}
	request.Reasoning = nil
	request.ReasoningEffort = adaptor.SupportedReasoningEffort(ModelRatios, request.Model, requested)
	if requested != nil && request.ReasoningEffort == nil && c != nil {
		// Bound the caller-controlled value echoed in DEBUG logs.
		value := *requested
		if len(value) > maxLoggedReasoningEffort {
			value = value[:maxLoggedReasoningEffort]
		}
		lg := gmw.GetLogger(c)
		lg.Debug("dropped unsupported NVIDIA reasoning_effort; provider default applies",
			zap.String("model", request.Model),
			zap.String("requested_effort", value),
			zap.Strings("supported_efforts", ModelRatios[request.Model].SupportedReasoningEfforts),
		)
	}
	return request, nil
}

// maxLoggedReasoningEffort bounds the caller-controlled effort echoed in DEBUG logs.
const maxLoggedReasoningEffort = 32

func (a *Adaptor) ConvertImageRequest(c *gin.Context, request *model.ImageRequest) (any, error) {
	return nil, errors.New("nvidia does not support image generation")
}

// ConvertClaudeRequest converts Claude Messages API requests to the
// OpenAI-compatible Chat Completions shape using the shared converter, since
// NVIDIA does not expose a native Anthropic Messages endpoint.
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error) {
	return openai_compatible.ConvertClaudeRequest(c, request)
}

func (a *Adaptor) DoRequest(c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	return adaptor.DoRequestHelper(a, c, meta, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	// NVIDIA exposes an OpenAI-compatible /v1/embeddings endpoint, so route the
	// response through the shared embedding handler instead of the chat handler
	// (which expects a `choices` field embedding responses do not have).
	if meta.Mode == relaymode.Embeddings {
		errResp, embeddingUsage := openai_compatible.EmbeddingHandler(c, resp)
		return embeddingUsage, errResp
	}

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
	return "nvidia"
}

// GetDefaultModelPricing returns the pricing information for NVIDIA models.
// See models.go for why every hosted model is registered as free (Ratio 0).
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

// DefaultToolingConfig returns NVIDIA tooling defaults. NVIDIA does not publish
// per-tool metering for the hosted API, so no built-in tool pricing is declared.
func (a *Adaptor) DefaultToolingConfig() adaptor.ChannelToolConfig {
	return NvidiaToolingDefaults
}
