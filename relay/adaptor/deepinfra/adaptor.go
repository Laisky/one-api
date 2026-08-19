// Package deepinfra implements the DeepInfra serverless inference adaptor.
//
// DeepInfra exposes several protocol surfaces under one Bearer-authenticated API:
//
//   - /v1/openai/* for OpenAI-compatible chat, completions, embeddings, and images
//   - /anthropic/v1/messages for native Anthropic Messages requests
//   - /v1/audio/* for OpenAI-compatible speech, transcription, and translation
//   - /v1/inference/{model} for model-native reranking
package deepinfra

import (
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

const defaultAnthropicVersion = "2023-06-01"

// Adaptor translates one-api relay requests to DeepInfra's public API surfaces.
type Adaptor struct {
	adaptor.DefaultPricingMethods
}

// Init initializes request-scoped state. DeepInfra does not require mutable state.
func (a *Adaptor) Init(meta *meta.Meta) {}

// GetChannelName returns the stable provider identifier used in logs and metrics.
func (a *Adaptor) GetChannelName() string {
	return "deepinfra"
}

// GetModelList returns a deterministic snapshot of supported DeepInfra models.
func (a *Adaptor) GetModelList() []string {
	models := make([]string, 0, len(ModelRatios))
	for modelName := range ModelRatios {
		models = append(models, modelName)
	}
	sort.Strings(models)
	return models
}

// GetDefaultModelPricing returns DeepInfra's provider-scoped pricing snapshot.
func (a *Adaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	return ModelRatios
}

// GetModelRatio returns the input-token or operation ratio for a DeepInfra model.
func (a *Adaptor) GetModelRatio(modelName string) float64 {
	if config, ok := ModelRatios[modelName]; ok {
		return config.Ratio
	}
	return a.DefaultPricingMethods.GetModelRatio(modelName)
}

// GetCompletionRatio returns the output-to-input price multiplier for a model.
func (a *Adaptor) GetCompletionRatio(modelName string) float64 {
	if config, ok := ModelRatios[modelName]; ok {
		return config.CompletionRatio
	}
	return a.DefaultPricingMethods.GetCompletionRatio(modelName)
}

// DefaultToolingConfig returns an empty provider-built-tool policy.
func (a *Adaptor) DefaultToolingConfig() adaptor.ChannelToolConfig {
	return adaptor.ChannelToolConfig{}
}

// GetRequestURL maps each one-api relay mode to the corresponding DeepInfra surface.
func (a *Adaptor) GetRequestURL(meta *meta.Meta) (string, error) {
	if meta == nil {
		return "", errors.New("meta is nil")
	}

	baseURL := adaptor.NormalizeBaseURL(meta.BaseURL)
	switch meta.Mode {
	case relaymode.ChatCompletions, relaymode.ResponseAPI:
		return adaptor.JoinBaseURLAndPath(baseURL, "/v1/openai/chat/completions"), nil
	case relaymode.Completions:
		return adaptor.JoinBaseURLAndPath(baseURL, "/v1/openai/completions"), nil
	case relaymode.Embeddings:
		return adaptor.JoinBaseURLAndPath(baseURL, "/v1/openai/embeddings"), nil
	case relaymode.ImagesGenerations:
		return adaptor.JoinBaseURLAndPath(baseURL, "/v1/openai/images/generations"), nil
	case relaymode.ImagesEdits:
		return adaptor.JoinBaseURLAndPath(baseURL, "/v1/images/edits"), nil
	case relaymode.AudioSpeech:
		return adaptor.JoinBaseURLAndPath(baseURL, "/v1/audio/speech"), nil
	case relaymode.AudioTranscription:
		return adaptor.JoinBaseURLAndPath(baseURL, "/v1/audio/transcriptions"), nil
	case relaymode.AudioTranslation:
		return adaptor.JoinBaseURLAndPath(baseURL, "/v1/audio/translations"), nil
	case relaymode.ClaudeMessages:
		return adaptor.JoinBaseURLAndPath(baseURL, "/anthropic/v1/messages"), nil
	case relaymode.Rerank:
		modelPath, err := escapeModelPath(meta.ActualModelName)
		if err != nil {
			return "", err
		}
		return adaptor.JoinBaseURLAndPath(baseURL, "/v1/inference/"+modelPath), nil
	default:
		return "", errors.Errorf("unsupported DeepInfra relay mode: %s", relaymode.String(meta.Mode))
	}
}

// escapeModelPath validates and escapes a provider/model identifier one segment at a time.
func escapeModelPath(modelName string) (string, error) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return "", errors.New("DeepInfra model name is empty")
	}

	segments := strings.Split(modelName, "/")
	for index, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" || segment == "." || segment == ".." {
			return "", errors.Errorf("invalid DeepInfra model path segment at index %d", index)
		}
		segments[index] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/"), nil
}

// SetupRequestHeader applies DeepInfra Bearer authentication and Anthropic version headers.
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) error {
	if req == nil {
		return errors.New("request is nil")
	}
	if meta == nil {
		return errors.New("meta is nil")
	}

	adaptor.SetupCommonRequestHeader(c, req, meta)
	req.Header.Set("Authorization", "Bearer "+meta.APIKey)

	if meta.Mode == relaymode.ClaudeMessages {
		anthropicVersion := defaultAnthropicVersion
		if c != nil && c.Request != nil {
			if incoming := strings.TrimSpace(c.Request.Header.Get("anthropic-version")); incoming != "" {
				anthropicVersion = incoming
			}
			if beta := strings.TrimSpace(c.Request.Header.Get("anthropic-beta")); beta != "" {
				req.Header.Set("anthropic-beta", beta)
			}
		}
		req.Header.Set("anthropic-version", anthropicVersion)
	}
	return nil
}

// ConvertRequest forwards OpenAI-compatible requests without provider-specific mutation.
func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return request, nil
}

// ConvertImageRequest forwards OpenAI-compatible image requests unchanged.
func (a *Adaptor) ConvertImageRequest(c *gin.Context, request *model.ImageRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return request, nil
}

// ConvertClaudeRequest enables native Anthropic Messages passthrough for DeepInfra.
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	if c != nil {
		c.Set(ctxkey.ClaudeModel, request.Model)
		c.Set(ctxkey.ClaudeMessagesNative, true)
		c.Set(ctxkey.ClaudeDirectPassthrough, true)
	}
	return request, nil
}

// DoRequest sends the converted request through the shared HTTP transport.
func (a *Adaptor) DoRequest(c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	return adaptor.DoRequestHelper(a, c, meta, requestBody)
}

// DoResponse dispatches DeepInfra responses to the matching response handler.
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	if meta == nil {
		return nil, openai_compatible.ErrorWrapper(errors.New("meta is nil"), "invalid_meta", http.StatusInternalServerError)
	}

	switch meta.Mode {
	case relaymode.Completions:
		if meta.IsStream {
			err, usage = handleCompletionStream(c, resp, meta.PromptTokens, meta.ActualModelName)
			return
		}
		err, usage = handleCompletionResponse(c, resp, meta.PromptTokens, meta.ActualModelName)
		return
	case relaymode.Rerank:
		err, usage = handleRerankResponse(c, resp, meta.ActualModelName, meta.PromptTokens)
		return
	case relaymode.Embeddings:
		err, usage = openai_compatible.EmbeddingHandler(c, resp)
		return
	case relaymode.ImagesGenerations, relaymode.ImagesEdits:
		err, usage = openai.ImageHandler(c, resp)
		return
	}

	// Native Claude Messages and audio requests use controller-level passthrough
	// paths. Chat and Response-fallback modes use DeepInfra's OpenAI-compatible
	// Chat Completions response shape.
	if meta.IsStream {
		err, usage = openai_compatible.StreamHandler(c, resp, meta.PromptTokens, meta.ActualModelName)
		return
	}
	err, usage = openai_compatible.Handler(c, resp, meta.PromptTokens, meta.ActualModelName)
	return
}
