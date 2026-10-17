package anthropic

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/common/toolnamesafe"
	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

type Adaptor struct {
}

// Init stores the request metadata used by this Anthropic adaptor.
func (a *Adaptor) Init(meta *meta.Meta) {

}

// GetRequestURL returns the Anthropic endpoint for the supplied metadata.
// https://docs.anthropic.com/claude/reference/messages_post
// anthopic migrate to Message API
func (a *Adaptor) GetRequestURL(meta *meta.Meta) (string, error) {
	return fmt.Sprintf("%s/v1/messages", meta.BaseURL), nil
}

// SetupRequestHeader sets provider credentials and admitted protocol headers on the upstream request.
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) error {
	adaptor.SetupCommonRequestHeader(c, req, meta)
	req.Header.Set("x-api-key", meta.APIKey)

	var callerHeaders http.Header
	if c != nil && c.Request != nil {
		callerHeaders = copyClaudeProtocolHeaders(req.Header, c.Request.Header)
	}
	inboundVersion := callerHeaders.Get("anthropic-version")
	anthropicVersion := strings.TrimSpace(inboundVersion)
	if anthropicVersion == "" {
		anthropicVersion = AnthropicVersionDefault
	}
	req.Header.Set("anthropic-version", anthropicVersion)

	// Start with any inbound beta headers to preserve explicit caller configuration.
	// Caller betas are not an admission path: every paid or provider-executed
	// capability they unlock (server tools, mcp_servers, containers) must also be
	// declared in the body, which the Claude Messages tool policy gates before
	// reservation and dispatch.
	var betaHeaders []string
	for _, value := range callerHeaders.Values("anthropic-beta") {
		betaHeaders = append(betaHeaders, strings.Split(value, ",")...)
	}
	betaHeaders = append(betaHeaders, AnthropicBetaMessages)

	// https://docs.anthropic.com/en/docs/about-claude/models/overview
	// claude-3-7-sonnet can support 128k context
	if strings.HasPrefix(meta.ActualModelName, "claude-3-7-sonnet") {
		betaHeaders = append(betaHeaders, "output-128k-2025-02-19")
	}
	if strings.HasPrefix(meta.ActualModelName, "claude-4-sonnet") {
		betaHeaders = append(betaHeaders, "context-1m-2025-08-07")
	}

	if strings.HasPrefix(meta.ActualModelName, "claude-4") {
		betaHeaders = append(betaHeaders, "interleaved-thinking-2025-05-14")
	}

	if c != nil {
		if enabled, ok := c.Get(ctxkey.ClaudeToolSearchEnabled); ok {
			if enabledBool, ok := enabled.(bool); ok && enabledBool {
				betaHeaders = append(betaHeaders, AnthropicBetaAdvancedToolUse)
			}
		}
		if enabled, ok := c.Get(ctxkey.ClaudeThinkingBindingControlsEnabled); ok {
			if enabledBool, ok := enabled.(bool); ok && enabledBool {
				betaHeaders = append(betaHeaders, AnthropicBetaThinkingBindingControls)
			}
		}
	}

	mergedBeta := mergeAnthropicBetaHeaders(betaHeaders)
	if len(mergedBeta) > 0 {
		req.Header.Set("anthropic-beta", strings.Join(mergedBeta, ","))
	}

	return nil
}

// ConvertRequest converts the supplied request to the Anthropic format for its relay mode.
func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}

	c.Set(ctxkey.ClaudeModel, request.Model)
	c.Set(ctxkey.ClaudeThinkingBindingControlsEnabled, hasClaudeThinkingBindingControls(request.Thinking))
	// Anthropic's `tools[].name` validator enforces `^[a-zA-Z0-9_-]{1,64}$`; sanitize
	// the OpenAI-shaped payload before conversion so the resulting Anthropic tool
	// definitions carry compliant names. Restoration happens in the response path
	// (StreamHandler / Handler) using the rename map stashed on c.
	toolnamesafe.SanitizeRequestToolNames(c, request)
	return ConvertRequest(c, *request)
}

// ConvertImageRequest returns the unsupported Anthropic image-request error.
func (a *Adaptor) ConvertImageRequest(_ *gin.Context, request *model.ImageRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return request, nil
}

// ConvertClaudeRequest implements direct pass-through for Claude Messages API requests.
// Instead of converting the request format, this method:
// 1. Parses the request for billing/token counting purposes
// 2. Sets flags to use the original request body directly for upstream calls
// 3. Ensures maximum compatibility with Anthropic's API by avoiding conversion artifacts
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}

	c.Set(ctxkey.ClaudeModel, request.Model)
	// Mark this as a native Claude Messages request (no conversion needed)
	c.Set(ctxkey.ClaudeMessagesNative, true)
	// Set flag to use direct pass-through instead of conversion
	c.Set(ctxkey.ClaudeDirectPassthrough, true)
	c.Set(ctxkey.ClaudeToolSearchEnabled, hasClaudeToolSearchTools(request))
	c.Set(ctxkey.ClaudeThinkingBindingControlsEnabled, hasClaudeThinkingBindingControls(request.Thinking))

	// Still parse the request for billing purposes, but we won't use the converted result
	// The original request body will be forwarded directly for better compatibility
	_, err := ConvertClaudeRequest(c, *request)
	if err != nil {
		return nil, errors.Wrap(err, "convert Claude request for billing")
	}

	// Return the original request - this won't be marshaled since we use direct pass-through
	return request, nil
}

// hasClaudeToolSearchTools reports whether the Claude request declares Anthropic
// Tool Search built-in tools.
func hasClaudeToolSearchTools(request *model.ClaudeRequest) bool {
	if request == nil {
		return false
	}
	for _, tool := range request.Tools {
		typeName := strings.ToLower(strings.TrimSpace(tool.Type))
		if typeName == ToolTypeToolSearchRegex ||
			typeName == ToolTypeToolSearchBM25 ||
			strings.HasPrefix(typeName, ToolTypeToolSearchRegexPrefix) ||
			strings.HasPrefix(typeName, ToolTypeToolSearchBM25Prefix) {
			return true
		}
	}
	return false
}

// mergeAnthropicBetaHeaders trims, deduplicates, and preserves insertion order
// for Anthropic beta header tokens.
func mergeAnthropicBetaHeaders(headers []string) []string {
	if len(headers) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(headers))
	merged := make([]string, 0, len(headers))
	for _, raw := range headers {
		for _, token := range strings.Split(raw, ",") {
			normalized := strings.ToLower(strings.TrimSpace(token))
			if normalized == "" {
				continue
			}
			if _, ok := seen[normalized]; ok {
				continue
			}
			seen[normalized] = struct{}{}
			merged = append(merged, normalized)
		}
	}
	return merged
}

// DoRequest dispatches the prepared Anthropic request and returns the upstream response or an error.
func (a *Adaptor) DoRequest(c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	prepared, err := PrepareRequestBody(c, meta.ActualModelName, requestBody)
	if err != nil {
		return nil, err
	}
	return adaptor.DoRequestHelper(a, c, meta, prepared)
}

// DoResponse converts the Anthropic response and returns its usage or relay error.
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	if meta.IsStream {
		err, usage = StreamHandler(c, resp)
	} else {
		err, usage = Handler(c, resp, meta.PromptTokens, meta.ActualModelName)
	}
	return
}

// GetModelList returns the Anthropic adaptor model identifiers.
func (a *Adaptor) GetModelList() []string {
	return adaptor.GetModelListFromPricing(ModelRatios)
}

// GetChannelName returns the Anthropic channel display name.
func (a *Adaptor) GetChannelName() string {
	return "anthropic"
}

// DefaultToolingConfig returns Anthropic's default built-in tooling policy
// including per-model pricing overrides for upstream web search.
func (a *Adaptor) DefaultToolingConfig() adaptor.ChannelToolConfig {
	return AnthropicToolingDefaults
}

// GetDefaultModelPricing returns the default Anthropic model pricing table.
// Pricing methods - Anthropic adapter manages its own model pricing
func (a *Adaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	return ModelRatios
}

// GetModelRatio returns the default price ratio for the supplied model.
func (a *Adaptor) GetModelRatio(modelName string) float64 {
	pricing := a.GetDefaultModelPricing()
	if price, exists := pricing[modelName]; exists {
		return price.Ratio
	}
	// Fallback to global pricing for unknown models
	return 3 * billingratio.MilliTokensUsd // Default Anthropic pricing in internal quota units
}

// GetCompletionRatio returns the output-token price ratio for the supplied model.
func (a *Adaptor) GetCompletionRatio(modelName string) float64 {
	pricing := a.GetDefaultModelPricing()
	if price, exists := pricing[modelName]; exists {
		return price.CompletionRatio
	}
	// Default completion ratio for Anthropic
	return 5.0
}
