// Package jina implements Jina AI's Search Foundation API and OCR chat API.
package jina

import (
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// Adaptor serves embeddings, reranking, and OCR through Jina's native API.
type Adaptor struct {
	adaptor.DefaultPricingMethods
}

var (
	_ adaptor.Adaptor                 = (*Adaptor)(nil)
	_ adaptor.StructuredRerankAdaptor = (*Adaptor)(nil)
)

// Init accepts request metadata; this adaptor has no mutable per-request state.
func (a *Adaptor) Init(_ *meta.Meta) {}

// GetChannelName returns the stable provider identifier without requiring Init.
func (a *Adaptor) GetChannelName() string { return "jina" }

// GetModelList returns inference IDs, not the namespaced IDs of Jina's catalog.
func (a *Adaptor) GetModelList() []string {
	return adaptor.GetModelListFromPricing(ModelRatios)
}

// GetDefaultModelPricing returns the researched Jina model configurations.
func (a *Adaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	return ModelRatios
}

// GetModelRatio resolves the input price for a model or the standard fallback.
func (a *Adaptor) GetModelRatio(name string) float64 {
	if cfg, ok := ModelRatios[name]; ok {
		return cfg.Ratio
	}
	return a.DefaultPricingMethods.GetModelRatio(name)
}

// GetCompletionRatio resolves the output/input price ratio for a model.
func (a *Adaptor) GetCompletionRatio(name string) float64 {
	if cfg, ok := ModelRatios[name]; ok {
		return cfg.CompletionRatio
	}
	return a.DefaultPricingMethods.GetCompletionRatio(name)
}

// GetRequestURL maps supported relay modes to native paths and normalizes /v1.
// Responses requests reach this adaptor through the shared chat fallback.
func (a *Adaptor) GetRequestURL(m *meta.Meta) (string, error) {
	if m == nil {
		return "", errors.New("jina request metadata is nil")
	}
	var path string
	switch m.Mode {
	case relaymode.Embeddings:
		path = "/v1/embeddings"
	case relaymode.Rerank:
		path = "/v1/rerank"
	case relaymode.ChatCompletions, relaymode.ClaudeMessages:
		path = "/v1/chat/completions"
	default:
		return "", errors.Errorf("jina does not natively support relay mode %d", m.Mode)
	}
	base := strings.TrimRight(m.BaseURL, "/")
	if base == "" {
		base = "https://api.jina.ai"
	}
	return strings.TrimSuffix(base, "/v1") + path, nil
}

// SetupRequestHeader applies the channel's Bearer key, never the caller's key.
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Request, m *meta.Meta) error {
	adaptor.SetupCommonRequestHeader(c, req, m)
	req.Header.Set("Authorization", "Bearer "+m.APIKey)
	return nil
}

// ConvertImageRequest rejects image generation; Jina's images are inputs only.
func (a *Adaptor) ConvertImageRequest(_ *gin.Context, _ *model.ImageRequest) (any, error) {
	return nil, errors.New("jina does not support image generation; use embeddings or OCR chat completions")
}

// ConvertClaudeRequest uses the shared Messages conversion, then normalizes OCR options.
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error) {
	converted, err := openai_compatible.ConvertClaudeRequest(c, request)
	if err != nil {
		return nil, err
	}
	if chat, ok := converted.(*model.GeneralOpenAIRequest); ok {
		return a.ConvertRequest(c, relaymode.ChatCompletions, chat)
	}
	return converted, nil
}

// SupportsStructuredRerank reports that native text/image documents are preserved.
func (a *Adaptor) SupportsStructuredRerank() bool { return true }

// DoRequest dispatches with this receiver so Jina routing and authentication apply.
func (a *Adaptor) DoRequest(c *gin.Context, m *meta.Meta, body io.Reader) (*http.Response, error) {
	return adaptor.DoRequestHelper(a, c, m, body)
}

// DoResponse normalizes search usage and reuses shared chat/Messages handling.
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, m *meta.Meta) (*model.Usage, *model.ErrorWithStatusCode) {
	if m.Mode == relaymode.Embeddings || m.Mode == relaymode.Rerank {
		return handleSearchResponse(c, resp, m.Mode)
	}
	return openai_compatible.HandleClaudeMessagesResponse(c, resp, m, func(c *gin.Context, resp *http.Response, promptTokens int, modelName string) (*model.ErrorWithStatusCode, *model.Usage) {
		if m.IsStream {
			return openai_compatible.StreamHandler(c, resp, promptTokens, modelName)
		}
		return openai_compatible.Handler(c, resp, promptTokens, modelName)
	})
}
