package copilot

import (
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

const (
	defaultEditorVersion       = "vscode/1.85.1"
	defaultEditorPluginVersion = "copilot/1.0.0"
	defaultIntegrationID       = "vscode-chat"
	defaultUserAgent           = "GithubCopilot/1.0.0"
)

// Adaptor implements GitHub Copilot's OpenAI-compatible surface.
//
// Copilot requires exchanging a GitHub access token (stored in the channel key)
// for a short-lived Copilot API token, then sending requests to
// https://api.githubcopilot.com.
//
// Supported endpoints:
// - Chat Completions: /chat/completions (mapped from /v1/chat/completions)
// - Embeddings: /embeddings (mapped from /v1/embeddings)
// - Response API: /v1/responses (passed through as /v1/responses)
//
// Note: Copilot enforces additional editor headers (editor-version, etc). This
// adaptor provides defaults and allows overriding them via request headers.
type Adaptor struct {
	adaptor.DefaultPricingMethods
}

// Init prepares the adaptor for a request.
func (a *Adaptor) Init(meta *meta.Meta) {
	// no-op
}

// GetRequestURL returns the upstream URL for the given request.
func (a *Adaptor) GetRequestURL(meta *meta.Meta) (string, error) {
	if meta == nil {
		return "", errors.New("meta is nil")
	}
	base := strings.TrimRight(strings.TrimSpace(meta.BaseURL), "/")
	if base == "" {
		return "", errors.New("base URL is empty")
	}
	path := strings.TrimSpace(meta.RequestURLPath)
	if path == "" {
		path = "/"
	}

	// Split query so we can normalize only the path.
	query := ""
	if idx := strings.Index(path, "?"); idx >= 0 {
		query = path[idx:]
		path = path[:idx]
	}

	normalizedPath := normalizeCopilotRequestPath(path)
	full := base + normalizedPath + query
	if _, err := url.Parse(full); err != nil {
		return "", errors.Wrap(err, "invalid upstream url")
	}
	return full, nil
}

// SetupRequestHeader injects Copilot-specific auth and required headers.
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) error {
	adaptor.SetupCommonRequestHeader(c, req, meta)

	// Copilot upstreams can be picky about compressed responses; rely on Go defaults.
	req.Header.Del("Accept-Encoding")

	if meta == nil {
		return errors.New("meta is nil")
	}

	apiToken, err := GetCopilotAPIToken(gmw.Ctx(c), meta.ChannelId, meta.APIKey)
	if err != nil {
		return errors.Wrap(err, "get copilot api token")
	}
	req.Header.Set("Authorization", "Bearer "+apiToken)

	// Copilot requires editor-identifying headers.
	ensureHeader(req, c, "editor-version", defaultEditorVersion)
	ensureHeader(req, c, "editor-plugin-version", defaultEditorPluginVersion)
	ensureHeader(req, c, "Copilot-Integration-Id", defaultIntegrationID)
	ensureHeader(req, c, "User-Agent", defaultUserAgent)

	return nil
}

// ConvertRequest passes through OpenAI-format requests unchanged.
func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	_ = c
	_ = relayMode
	return request, nil
}

// ConvertImageRequest is not supported by this adaptor.
func (a *Adaptor) ConvertImageRequest(_ *gin.Context, _ *model.ImageRequest) (any, error) {
	return nil, errors.New("copilot adaptor does not support image requests")
}

// ConvertClaudeRequest is not supported by this adaptor.
func (a *Adaptor) ConvertClaudeRequest(_ *gin.Context, _ *model.ClaudeRequest) (any, error) {
	return nil, errors.New("copilot adaptor does not support claude messages requests")
}

// DoRequest forwards the request to the Copilot upstream.
func (a *Adaptor) DoRequest(c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	return adaptor.DoRequestHelper(a, c, meta, requestBody)
}

// DoResponse streams or forwards the response and returns usage when available.
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (*model.Usage, *model.ErrorWithStatusCode) {
	if meta == nil {
		return nil, openai.ErrorWrapper(errors.New("meta is nil"), "invalid_meta", http.StatusInternalServerError)
	}

	if meta.IsStream {
		var (
			usage        *model.Usage
			errResp      *model.ErrorWithStatusCode
			responseText string
		)

		switch meta.Mode {
		case relaymode.ResponseAPI:
			errResp, responseText, usage = openai.ResponseAPIDirectStreamHandler(c, resp, meta.Mode)
		default:
			errResp, responseText, usage = openai.StreamHandler(c, resp, meta.Mode)
		}

		if usage == nil || usage.TotalTokens == 0 {
			usage = openai.ResponseText2Usage(responseText, meta.ActualModelName, meta.PromptTokens)
		}
		if usage != nil && usage.TotalTokens != 0 && usage.PromptTokens == 0 {
			usage.PromptTokens = meta.PromptTokens
			usage.CompletionTokens = usage.TotalTokens - meta.PromptTokens
		}
		return usage, errResp
	}

	// Non-streaming
	switch meta.Mode {
	case relaymode.ResponseAPI:
		errResp, usage := openai.ResponseAPIDirectHandler(c, resp, meta.PromptTokens, meta.ActualModelName)
		return usage, errResp
	case relaymode.Embeddings:
		errResp, usage := openai.EmbeddingHandler(c, resp, meta.PromptTokens, meta.ActualModelName)
		return usage, errResp
	default:
		errResp, usage := openai.Handler(c, resp, meta.PromptTokens, meta.ActualModelName)
		return usage, errResp
	}
}

// GetModelList returns the known upstream model list if available.
func (a *Adaptor) GetModelList() []string {
	return nil
}

// GetChannelName returns the provider name for metrics/logging.
func (a *Adaptor) GetChannelName() string {
	return "copilot"
}

// ensureHeader copies the header from the incoming request into the upstream
// request if present; otherwise it sets a default.
func ensureHeader(req *http.Request, c *gin.Context, key string, defaultValue string) {
	if req.Header.Get(key) != "" {
		return
	}

	// Prefer exact header name.
	incoming := ""
	if c != nil && c.Request != nil {
		incoming = c.Request.Header.Get(key)
		if incoming == "" {
			incoming = c.Request.Header.Get("X-" + key)
		}
	}
	if incoming != "" {
		req.Header.Set(key, incoming)
		return
	}
	if defaultValue != "" {
		req.Header.Set(key, defaultValue)
	}
}

// normalizeCopilotRequestPath maps One-API /v1 endpoints to Copilot's upstream paths.
func normalizeCopilotRequestPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// GitHub Copilot uses non-/v1 paths for chat completions and embeddings.
	if strings.HasPrefix(path, "/v1/chat/") {
		return strings.TrimPrefix(path, "/v1")
	}
	if strings.HasPrefix(path, "/v1/embeddings") {
		return strings.TrimPrefix(path, "/v1")
	}
	if strings.HasPrefix(path, "/v1/models") {
		return strings.TrimPrefix(path, "/v1")
	}

	// Response API is typically available at /v1/responses.
	return path
}
