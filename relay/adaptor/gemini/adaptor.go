package gemini

import (
	"fmt"
	"io"
	"maps"
	"net/http"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	channelhelper "github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/geminiOpenaiCompatible"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

type Adaptor struct {
}

// Init prepares the adaptor for one request; Gemini keeps no per-request state.
func (a *Adaptor) Init(meta *meta.Meta) {
}

// GetRequestURL returns the Gemini endpoint for meta's model: batchEmbedContents
// for embeddings, streamGenerateContent (SSE) for streams, generateContent
// otherwise. It never returns an error.
func (a *Adaptor) GetRequestURL(meta *meta.Meta) (string, error) {
	version := resolveGeminiAPIVersion(meta.ActualModelName, meta.Config.APIVersion)
	action := ""
	switch meta.Mode {
	case relaymode.Embeddings:
		action = "batchEmbedContents"
	default:
		action = "generateContent"
	}

	if meta.IsStream {
		action = "streamGenerateContent?alt=sse"
	}

	return fmt.Sprintf("%s/%s/models/%s:%s", meta.BaseURL, version, meta.ActualModelName, action), nil
}

// SetupRequestHeader sets the common headers and the channel key, both as the
// x-goog-api-key header and the key query parameter. It always returns nil.
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) error {
	channelhelper.SetupCommonRequestHeader(c, req, meta)
	req.Header.Set("x-goog-api-key", meta.APIKey)
	query := req.URL.Query()
	query.Add("key", meta.APIKey)
	req.URL.RawQuery = query.Encode()
	return nil
}

// ConvertRequest converts an OpenAI-compatible request into Gemini-native payloads for the selected relay mode.
// Parameters: c is the request context, relayMode selects the endpoint family, and request is the validated OpenAI-compatible request.
// Returns: the converted Gemini payload or an error when conversion fails.
func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	switch relayMode {
	case relaymode.Embeddings:
		geminiEmbeddingRequest, err := ConvertEmbeddingRequest(*request)
		if err != nil {
			return nil, errors.Wrap(err, "convert gemini embedding request")
		}
		return geminiEmbeddingRequest, nil
	default:
		if err := ValidateNativeToolSupport(c, request); err != nil {
			return nil, errors.Wrap(err, "validate native Gemini tools")
		}
		geminiRequest, err := ConvertNativeRequestChecked(c, *request)
		if err != nil {
			return nil, errors.Wrap(err, "convert native Gemini request")
		}
		return geminiRequest, nil
	}
}

// ConvertImageRequest passes an image request through unchanged; it returns an
// error for a nil request.
func (a *Adaptor) ConvertImageRequest(_ *gin.Context, request *model.ImageRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return request, nil
}

// ConvertClaudeRequest lowers Claude content through the common native Gemini pipeline.
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error) {
	converted, err := ConvertClaudeToNativeChat(c, request)
	if err != nil {
		return nil, err
	}
	return a.ConvertRequest(c, relaymode.ChatCompletions, converted)
}

// DoRequest sends the converted request through the shared channel HTTP path
// and returns the upstream response or the transport error.
func (a *Adaptor) DoRequest(c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	return channelhelper.DoRequestHelper(a, c, meta, requestBody)
}

// DoResponse renders the Gemini upstream response for the client format of the
// request (OpenAI-compatible or Claude Messages) and returns the measured
// usage or a relay error.
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	// Handle Claude Messages response conversion
	if isClaudeConversion, exists := c.Get(ctxkey.ClaudeMessagesConversion); exists && isClaudeConversion.(bool) {
		claudeResp, convertErr := a.convertToClaudeResponse(c, resp, meta)
		if convertErr != nil {
			return nil, convertErr
		}

		// Replace the original response with the converted Claude response
		// We need to update the response in the context so the controller can use it
		c.Set(ctxkey.ConvertedResponse, claudeResp)

		// For Claude Messages conversion, we don't return usage separately
		// The usage is included in the Claude response body, so return nil usage
		return nil, nil
	}

	if meta.IsStream {
		var responseText string
		var streamUsage *model.Usage
		err, responseText, streamUsage = StreamHandler(c, resp)
		// Prefer Gemini's authoritative usageMetadata (cached prompt + reasoning tokens) over
		// the text-based estimate; only fall back when no usageMetadata was present in the stream.
		if streamUsage != nil {
			usage = streamUsage
		} else {
			usage = openai.ResponseText2Usage(responseText, meta.ActualModelName, meta.PromptTokens)
			usage.BillingEstimateReason = "gemini_stream_missing_usage_receipt"
		}
	} else {
		switch meta.Mode {
		case relaymode.Embeddings:
			err, usage = EmbeddingHandler(c, resp, meta.PromptTokens)
		default:
			err, usage = Handler(c, resp, meta.PromptTokens, meta.ActualModelName)
		}
	}
	return
}

// GetModelList returns the Gemini model IDs that have bundled pricing.
func (a *Adaptor) GetModelList() []string {
	return channelhelper.GetModelListFromPricing(ModelRatios)
}

// GetChannelName returns the channel display name.
func (a *Adaptor) GetChannelName() string {
	return "google gemini"
}

// DefaultToolingConfig exposes Google's default grounded web search tooling
// metadata so channel policy builders can merge in provider pricing.
func (a *Adaptor) DefaultToolingConfig() channelhelper.ChannelToolConfig {
	return GeminiToolingDefaults
}

// DefaultToolingConfigForModel returns Google's grounded web search tooling
// metadata for a specific model, billing Gemini 3.x web_search at $14/1K queries
// and Gemini 2.5 and earlier at $35/1K queries. The tooling-policy builder prefers
// this over DefaultToolingConfig when present.
func (a *Adaptor) DefaultToolingConfigForModel(model string) channelhelper.ChannelToolConfig {
	return geminiOpenaiCompatible.GeminiToolingDefaultsForModel(model)
}

// Pricing methods - Gemini adapter manages its own model pricing
func (a *Adaptor) GetDefaultModelPricing() map[string]channelhelper.ModelConfig {
	pricing := maps.Clone(ModelRatios)
	name := geminiOpenaiCompatible.GeminiNanoBanana21Model
	if cfg, ok := pricing[name]; ok {
		pricing[name] = geminiOpenaiCompatible.NativeGeminiModelConfig(name, cfg)
	}
	return pricing
}

// GetModelRatio returns the bundled input ratio of modelName, or the adaptor
// default when the model has no bundled pricing.
func (a *Adaptor) GetModelRatio(modelName string) float64 {
	pricing := a.GetDefaultModelPricing()
	if price, exists := pricing[modelName]; exists {
		return price.Ratio
	}
	// Default Gemini pricing - use global constant for consistency
	return 5 * ratio.MilliTokensUsd // Default quota-based pricing
}

// GetCompletionRatio returns the bundled completion ratio of modelName, or the
// adaptor default when the model has no bundled pricing.
func (a *Adaptor) GetCompletionRatio(modelName string) float64 {
	pricing := a.GetDefaultModelPricing()
	if price, exists := pricing[modelName]; exists {
		return price.CompletionRatio
	}
	// Default completion ratio for Gemini
	return 3.0
}
