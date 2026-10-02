// Package typesafe implements the native TypeSafe System One evaluation API.
package typesafe

import (
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor"
	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// DefaultBaseURL is TypeSafe's documented HTTPS inference service.
const DefaultBaseURL = "https://api.typesafe.ai"

// Adaptor implements native evaluation; it deliberately rejects generative APIs.
type Adaptor struct{ adaptor.DefaultPricingMethods }

var _ adaptor.Adaptor = (*Adaptor)(nil)
var _ adaptor.RedirectPolicyAdaptor = (*Adaptor)(nil)

// modelPricing records the official catalog checked on 2026-09-18.
// Sources: https://docs.typesafe.ai/models and https://docs.typesafe.ai/api.
// $42/B input tokens = $0.042/M input tokens; output tokens are free.
var modelPricing = func() map[string]adaptor.ModelConfig {
	models := make(map[string]adaptor.ModelConfig, 3)
	for _, name := range []string{"jev-1.13.0", "jev-latest", "jev-preview"} {
		models[name] = adaptor.ModelConfig{
			Ratio:            0.042 * billingratio.MilliTokensUsd,
			CompletionRatio:  0,
			ContextLength:    64000,
			InputModalities:  []string{"text"},
			OutputModalities: []string{"text"},
			Description: "TypeSafe System One typed evaluation (noul/choice/score), not chat. " +
				"64k state+all questions; 32k state+longest question. Input-token pricing; output free. " +
				"Aliases currently resolve to jev-1.13.0 and may change.",
		}
	}
	return models
}()

// Init leaves request-specific state in the supplied relay metadata.
func (a *Adaptor) Init(_ *meta.Meta) {}

// GetChannelName returns the provider identifier used in logs and catalogs.
func (a *Adaptor) GetChannelName() string { return "typesafe" }

// GetModelList returns a stable list of documented versioned IDs and aliases.
func (a *Adaptor) GetModelList() []string {
	names := adaptor.GetModelListFromPricing(modelPricing)
	sort.Strings(names)
	return names
}

// GetDefaultModelPricing returns independent copies of the researched catalog.
func (a *Adaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	result := make(map[string]adaptor.ModelConfig, len(modelPricing))
	for name, config := range modelPricing {
		result[name] = config.Clone()
	}
	return result
}

// GetModelRatio returns the default input price for a documented model.
func (a *Adaptor) GetModelRatio(name string) float64 {
	if config, ok := modelPricing[name]; ok {
		return config.Ratio
	}
	return a.DefaultPricingMethods.GetModelRatio(name)
}

// GetCompletionRatio keeps output tokens free for the input-priced native API.
func (a *Adaptor) GetCompletionRatio(_ string) float64 { return 0 }

// GetRequestURL normalizes an optional /v1 suffix and validates endpoint overrides.
func (a *Adaptor) GetRequestURL(m *meta.Meta) (string, error) {
	if m == nil || m.Mode != relaymode.SystemOne {
		return "", errors.New("TypeSafe supports POST /v1/systemone only")
	}
	base := strings.TrimRight(m.BaseURL, "/")
	if base == "" {
		base = DefaultBaseURL
	}
	if err := validateURL(base); err != nil {
		return "", err
	}
	if override := m.UpstreamEndpointURLOverride(); override != "" {
		if err := validateURL(override); err != nil {
			return "", err
		}
	}
	return strings.TrimSuffix(base, "/v1") + "/v1/systemone", nil
}

// validateURL prevents credentials from being sent over HTTP or embedded in URLs.
func validateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" ||
		u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return errors.New("TypeSafe upstream URL must use HTTPS without userinfo, query or fragment")
	}
	return nil
}

// SetupRequestHeader validates the effective URL before attaching the channel key.
// Downstream authorization and arbitrary X-* headers are not forwarded.
func (a *Adaptor) SetupRequestHeader(_ *gin.Context, request *http.Request, m *meta.Meta) error {
	if request == nil || request.URL == nil || m == nil {
		return errors.New("missing TypeSafe request metadata")
	}
	if err := validateURL(request.URL.String()); err != nil {
		return err
	}
	if request.Header == nil {
		request.Header = make(http.Header)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+m.APIKey)
	return nil
}

// CheckRedirect prevents credential forwarding and implicit replay of paid work.
func (a *Adaptor) CheckRedirect(_ *http.Request, _ []*http.Request) error {
	return errors.New("automatic redirects are disabled for TypeSafe evaluations")
}

// ConvertRequest rejects chat, embedding and other non-System-One request shapes.
func (a *Adaptor) ConvertRequest(_ *gin.Context, _ int, _ *model.GeneralOpenAIRequest) (any, error) {
	return nil, errors.New("TypeSafe is not a chat model; use the native /v1/systemone endpoint")
}

// ConvertImageRequest rejects unsupported image generation requests.
func (a *Adaptor) ConvertImageRequest(_ *gin.Context, _ *model.ImageRequest) (any, error) {
	return nil, errors.New("TypeSafe does not support image generation")
}

// ConvertClaudeRequest rejects lossy conversion of conversations to evaluations.
func (a *Adaptor) ConvertClaudeRequest(_ *gin.Context, _ *model.ClaudeRequest) (any, error) {
	return nil, errors.New("TypeSafe requires native state and questions, not Claude Messages")
}

// DoRequest sends a validated native payload through the gateway's shared transport.
func (a *Adaptor) DoRequest(c *gin.Context, m *meta.Meta, body io.Reader) (*http.Response, error) {
	return adaptor.DoRequestHelper(a, c, m, body)
}

// DoResponse implements the common adaptor interface for native response consumers.
// The billed native controller uses ReadResponse to settle before committing output.
func (a *Adaptor) DoResponse(c *gin.Context, response *http.Response, m *meta.Meta) (*model.Usage, *model.ErrorWithStatusCode) {
	result, usage, apiErr := a.ReadResponse(c, response, m)
	if result != nil {
		result.Write(c)
	}
	return usage, apiErr
}
