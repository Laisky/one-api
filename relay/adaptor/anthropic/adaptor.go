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
	anthropicVersion := c.Request.Header.Get("anthropic-version")
	if anthropicVersion == "" {
		anthropicVersion = "2023-06-01"
	}
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("anthropic-beta", "messages-2023-12-15")

	// set beta headers for specific models
	var betaHeaders []string

	// https://x.com/alexalbert__/status/1812921642143900036
	// claude-3-5-sonnet can support 8k context
	if strings.HasPrefix(meta.ActualModelName, "claude-3-7-sonnet") {
		betaHeaders = append(betaHeaders, "output-128k-2024-11-06")
	}

	if strings.HasPrefix(meta.ActualModelName, "claude-4") {
		betaHeaders = append(betaHeaders, "interleaved-thinking-2025-05-14")
	}

	if len(betaHeaders) > 0 {
		req.Header.Set("anthropic-beta", strings.Join(betaHeaders, ","))
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

	// Still parse the request for billing purposes, but we won't use the converted result
	// The original request body will be forwarded directly for better compatibility
	_, err := ConvertClaudeRequest(c, *request)
	if err != nil {
		return nil, err
	}

	// Return the original request - this won't be marshaled since we use direct pass-through
	return request, nil
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
