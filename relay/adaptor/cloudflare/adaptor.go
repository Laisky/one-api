package cloudflare

import (
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor"
	openaiadaptor "github.com/Laisky/one-api/relay/adaptor/openai"
	openaicompatible "github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

const (
	legacyAIGatewayHost       = "gateway.ai.cloudflare.com"
	legacyAIGatewayPathSuffix = "/workers-ai"
)

// Adaptor implements the Cloudflare Workers AI relay using Cloudflare's
// OpenAI-compatible HTTP endpoints.
type Adaptor struct{}

// Init initializes the adaptor for a request. Cloudflare does not require
// request-local adaptor state.
func (a *Adaptor) Init(_ *meta.Meta) {}

// GetRequestURL returns the Cloudflare endpoint that matches the incoming relay
// mode. Requests converted from legacy Completions or Claude Messages use the
// Chat Completions endpoint so their request and response schemas remain paired.
func (a *Adaptor) GetRequestURL(metaInfo *meta.Meta) (string, error) {
	if metaInfo == nil {
		return "", errors.New("cloudflare meta is nil")
	}

	apiBaseURL, err := cloudflareAPIBaseURL(metaInfo.BaseURL, metaInfo.Config.UserID)
	if err != nil {
		return "", errors.Wrap(err, "build cloudflare API base URL")
	}

	endpoint, err := cloudflareOpenAIEndpoint(metaInfo.Mode)
	if err != nil {
		return "", err
	}
	return apiBaseURL + endpoint, nil
}

// SetupRequestHeader configures authentication and content negotiation for a
// Cloudflare Workers AI request.
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Request, metaInfo *meta.Meta) error {
	adaptor.SetupCommonRequestHeader(c, req, metaInfo)
	req.Header.Set("Authorization", "Bearer "+metaInfo.APIKey)
	req.Header.Set("Content-Type", "application/json")
	return nil
}

// ConvertRequest converts supported OpenAI request modes to the schema accepted
// by the selected Cloudflare OpenAI-compatible endpoint.
func (a *Adaptor) ConvertRequest(_ *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("cloudflare request is nil")
	}

	switch relayMode {
	case relaymode.ChatCompletions, relaymode.Embeddings, relaymode.ResponseAPI:
		return request, nil
	case relaymode.Completions:
		return convertCompletionToChatRequest(request)
	default:
		return nil, errors.Errorf("cloudflare relay mode %d is not supported", relayMode)
	}
}

// ConvertImageRequest reports that image multipart requests are not implemented
// by this text-focused Cloudflare adaptor.
func (a *Adaptor) ConvertImageRequest(_ *gin.Context, _ *model.ImageRequest) (any, error) {
	return nil, errors.New("cloudflare image requests are not supported")
}

// ConvertClaudeRequest converts Anthropic Claude Messages input into an OpenAI
// Chat Completions request while preserving tools, structured content, streaming,
// and the context markers needed for response conversion.
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error) {
	converted, err := openaicompatible.ConvertClaudeRequest(c, request)
	if err != nil {
		return nil, errors.Wrap(err, "convert Claude Messages request for Cloudflare")
	}
	return converted, nil
}

// DoRequest sends a Cloudflare request using the common adaptor transport.
func (a *Adaptor) DoRequest(c *gin.Context, metaInfo *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	return adaptor.DoRequestHelper(a, c, metaInfo, requestBody)
}

// DoResponse handles Cloudflare's OpenAI-compatible responses, including native
// Responses API output and conversion back to Claude Messages when requested.
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, metaInfo *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	return (&openaiadaptor.Adaptor{}).DoResponse(c, resp, metaInfo)
}

// GetModelList returns the currently supported Cloudflare model identifiers.
func (a *Adaptor) GetModelList() []string {
	return ModelList
}

// GetChannelName returns the human-readable provider name.
func (a *Adaptor) GetChannelName() string {
	return "cloudflare"
}

// GetDefaultModelPricing returns Cloudflare's built-in model metadata and
// pricing table.
func (a *Adaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	return ModelRatios
}

// GetModelRatio returns the configured input-token price ratio for a model.
func (a *Adaptor) GetModelRatio(modelName string) float64 {
	if price, ok := ModelRatios[modelName]; ok {
		return price.Ratio
	}
	return 5 * ratio.MilliTokensUsd
}

// GetCompletionRatio returns the output-to-input price ratio for a model.
func (a *Adaptor) GetCompletionRatio(modelName string) float64 {
	if price, ok := ModelRatios[modelName]; ok {
		return price.CompletionRatio
	}
	return 1
}

// DefaultToolingConfig returns the provider-level tool capabilities. Per-model
// metadata remains authoritative for whether a particular model supports tools.
func (a *Adaptor) DefaultToolingConfig() adaptor.ChannelToolConfig {
	return CloudflareToolingDefaults
}

// cloudflareOpenAIEndpoint maps one-api relay modes to Cloudflare's supported
// OpenAI-compatible endpoints.
func cloudflareOpenAIEndpoint(relayMode int) (string, error) {
	switch relayMode {
	case relaymode.ChatCompletions, relaymode.Completions, relaymode.ClaudeMessages:
		return "/v1/chat/completions", nil
	case relaymode.Embeddings:
		return "/v1/embeddings", nil
	case relaymode.ResponseAPI:
		return "/v1/responses", nil
	default:
		return "", errors.Errorf("cloudflare relay mode %d has no OpenAI-compatible endpoint", relayMode)
	}
}

// cloudflareAPIBaseURL normalizes either the standard Workers AI base URL, a
// full account-scoped Workers AI URL, or the legacy AI Gateway workers-ai URL.
func cloudflareAPIBaseURL(rawBaseURL, accountID string) (string, error) {
	rawBaseURL = strings.TrimSpace(rawBaseURL)
	if rawBaseURL == "" {
		return "", errors.New("cloudflare base URL is empty")
	}

	parsed, err := url.Parse(rawBaseURL)
	if err != nil {
		return "", errors.Wrap(err, "parse cloudflare base URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.Errorf("cloudflare base URL must use http or https, got %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return "", errors.New("cloudflare base URL must include a host")
	}
	if parsed.User != nil {
		return "", errors.New("cloudflare base URL must not include credentials")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("cloudflare base URL must not include a query or fragment")
	}

	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = ""
	normalized := strings.TrimRight(parsed.String(), "/")

	segments := splitURLPath(parsed.Path)
	if strings.EqualFold(parsed.Hostname(), legacyAIGatewayHost) {
		switch {
		case hasPathSuffix(segments, "workers-ai", "v1"):
			return strings.TrimSuffix(normalized, "/v1"), nil
		case hasPathSuffix(segments, "workers-ai"):
			return normalized, nil
		default:
			return "", errors.Errorf("legacy Cloudflare AI Gateway URL must end with %s or %s/v1", legacyAIGatewayPathSuffix, legacyAIGatewayPathSuffix)
		}
	}

	if hasPathSuffix(segments, "client", "v4", "accounts", "*", "ai", "v1") {
		return strings.TrimSuffix(normalized, "/v1"), nil
	}
	if hasPathSuffix(segments, "client", "v4", "accounts", "*", "ai") {
		return normalized, nil
	}

	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return "", errors.New("cloudflare account ID is required")
	}
	if strings.ContainsAny(accountID, "/?#") {
		return "", errors.New("cloudflare account ID contains invalid path characters")
	}

	escapedAccountID := url.PathEscape(accountID)
	switch {
	case hasPathSuffix(segments, "client", "v4", "accounts", "*"):
		return normalized + "/ai", nil
	case hasPathSuffix(segments, "client", "v4"):
		return normalized + "/accounts/" + escapedAccountID + "/ai", nil
	default:
		return normalized + "/client/v4/accounts/" + escapedAccountID + "/ai", nil
	}
}

// splitURLPath returns non-empty path segments for suffix matching.
func splitURLPath(path string) []string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 1 && parts[0] == "" {
		return nil
	}
	return parts
}

// hasPathSuffix reports whether path segments end with the supplied pattern.
// The literal "*" matches any non-empty segment.
func hasPathSuffix(segments []string, pattern ...string) bool {
	if len(segments) < len(pattern) {
		return false
	}
	start := len(segments) - len(pattern)
	for i, expected := range pattern {
		actual := segments[start+i]
		if expected == "*" {
			if actual == "" {
				return false
			}
			continue
		}
		if !strings.EqualFold(actual, expected) {
			return false
		}
	}
	return true
}

// convertCompletionToChatRequest converts a legacy text completion prompt to a
// single user message while preserving shared sampling and streaming fields.
func convertCompletionToChatRequest(request *model.GeneralOpenAIRequest) (*model.GeneralOpenAIRequest, error) {
	prompt := strings.TrimSpace(request.Prompt)
	if prompt == "" {
		return nil, errors.New("cloudflare completion prompt is empty")
	}

	converted := *request
	converted.Prompt = ""
	converted.Messages = []model.Message{{Role: "user", Content: request.Prompt}}
	return &converted, nil
}
