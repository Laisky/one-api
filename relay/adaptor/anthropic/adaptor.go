package anthropic

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

type Adaptor struct {
}

func (a *Adaptor) Init(meta *meta.Meta) {

}

// https://docs.anthropic.com/claude/reference/messages_post
// anthopic migrate to Message API
func (a *Adaptor) GetRequestURL(meta *meta.Meta) (string, error) {
	// Handle different relay modes for Anthropic
	switch meta.Mode {
	case relaymode.ClaudeMessages:
		return fmt.Sprintf("%s/v1/messages", meta.BaseURL), nil
	default:
		return fmt.Sprintf("%s/v1/messages", meta.BaseURL), nil
	}
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) error {
	adaptor.SetupCommonRequestHeader(c, req, meta)
	req.Header.Set("x-api-key", meta.APIKey)

	var inboundVersion string
	if c != nil && c.Request != nil {
		inboundVersion = c.Request.Header.Get("anthropic-version")
	}
	anthropicVersion := strings.TrimSpace(inboundVersion)
	if anthropicVersion == "" {
		anthropicVersion = AnthropicVersionDefault
	}
	req.Header.Set("anthropic-version", anthropicVersion)

	// Start with any inbound beta headers to preserve explicit caller configuration.
	var betaHeaders []string
	if c != nil && c.Request != nil {
		betaHeaders = append(betaHeaders, strings.Split(c.Request.Header.Get("anthropic-beta"), ",")...)
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
	}

	mergedBeta := mergeAnthropicBetaHeaders(betaHeaders)
	if len(mergedBeta) > 0 {
		req.Header.Set("anthropic-beta", strings.Join(mergedBeta, ","))
	}

	return nil
}

func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}

	c.Set(ctxkey.ClaudeModel, request.Model)
	return ConvertRequest(c, *request)
}

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

func (a *Adaptor) DoRequest(c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	return adaptor.DoRequestHelper(a, c, meta, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	if meta.IsStream {
		err, usage = StreamHandler(c, resp)
	} else {
		err, usage = Handler(c, resp, meta.PromptTokens, meta.ActualModelName)
	}
	return
}

func (a *Adaptor) GetModelList() []string {
	return adaptor.GetModelListFromPricing(ModelRatios)
}

func (a *Adaptor) GetChannelName() string {
	return "anthropic"
}

// DefaultToolingConfig returns Anthropic's default built-in tooling policy
// including per-model pricing overrides for upstream web search.
func (a *Adaptor) DefaultToolingConfig() adaptor.ChannelToolConfig {
	return AnthropicToolingDefaults
}

// Pricing methods - Anthropic adapter manages its own model pricing
func (a *Adaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	return ModelRatios
}

func (a *Adaptor) GetModelRatio(modelName string) float64 {
	pricing := a.GetDefaultModelPricing()
	if price, exists := pricing[modelName]; exists {
		return price.Ratio
	}
	// Fallback to global pricing for unknown models
	return 3 * 0.000001 // Default Anthropic pricing
}

func (a *Adaptor) GetCompletionRatio(modelName string) float64 {
	pricing := a.GetDefaultModelPricing()
	if price, exists := pricing[modelName]; exists {
		return price.CompletionRatio
	}
	// Default completion ratio for Anthropic
	return 5.0
}
