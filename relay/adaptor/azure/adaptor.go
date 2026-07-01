// Package azure implements the adaptor for the Azure AI Foundry channel.
//
// A single Azure AI Foundry resource exposes two distinct upstream surfaces:
//   - Azure OpenAI models via /openai/deployments/{deployment}/... (OpenAI wire
//     format, `api-key` header) — handled by the embedded openai.Adaptor.
//   - Anthropic Claude models via the native Anthropic Messages API at
//     /anthropic/v1/messages (`x-api-key` + `anthropic-version` headers) — there
//     is NO OpenAI-compatible route for Claude on Foundry.
//
// This adaptor therefore dispatches by model family: Claude models
// (meta.AzureTargetsAnthropic) are delegated to an anthropic.Adaptor, and
// everything else falls through to the embedded openai.Adaptor's Azure handling.
// Because it owns its own apitype, both request routing (relay.GetAdaptor) and
// pricing (resolvePricingAdaptor) resolve here, so Claude-on-Azure bills at
// Anthropic rates while GPT-on-Azure keeps its existing behavior unchanged.
//
// Reference:
// https://learn.microsoft.com/en-us/azure/foundry/foundry-models/how-to/use-foundry-models-claude
package azure

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

// Adaptor serves the Azure AI Foundry channel. It embeds openai.Adaptor for the
// Azure OpenAI surface and holds an anthropic.Adaptor for the native Anthropic
// Messages surface used by Claude models.
type Adaptor struct {
	openai.Adaptor
	claude anthropic.Adaptor
}

func (a *Adaptor) Init(m *meta.Meta) {
	a.Adaptor.Init(m)
	a.claude.Init(m)
}

// GetRequestURL builds the Azure OpenAI URL for OpenAI models, or the native
// Anthropic Messages URL for Claude models. The Azure resource base URL is shared
// (e.g. https://<resource>.services.ai.azure.com); the Claude surface lives under
// the "/anthropic" path, derived here since the base URL is reused for both.
func (a *Adaptor) GetRequestURL(m *meta.Meta) (string, error) {
	if m.AzureTargetsAnthropic() {
		return fmt.Sprintf("%s/anthropic/v1/messages", strings.TrimRight(m.BaseURL, "/")), nil
	}
	return a.Adaptor.GetRequestURL(m)
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Request, m *meta.Meta) error {
	if m.AzureTargetsAnthropic() {
		return a.claude.SetupRequestHeader(c, req, m)
	}
	return a.Adaptor.SetupRequestHeader(c, req, m)
}

func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	if meta.GetByContext(c).AzureTargetsAnthropic() {
		return a.claude.ConvertRequest(c, relayMode, request)
	}
	return a.Adaptor.ConvertRequest(c, relayMode, request)
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error) {
	if meta.GetByContext(c).AzureTargetsAnthropic() {
		return a.claude.ConvertClaudeRequest(c, request)
	}
	return a.Adaptor.ConvertClaudeRequest(c, request)
}

func (a *Adaptor) DoRequest(c *gin.Context, m *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	// Route through DoRequestHelper with this adaptor so GetRequestURL and
	// SetupRequestHeader dispatch by model family (openai vs anthropic surface).
	return adaptor.DoRequestHelper(a, c, m, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, m *meta.Meta) (*model.Usage, *model.ErrorWithStatusCode) {
	if m.AzureTargetsAnthropic() {
		return a.claude.DoResponse(c, resp, m)
	}
	return a.Adaptor.DoResponse(c, resp, m)
}

// GetModelList returns the Azure AI Foundry catalog this adaptor supports: the
// Azure OpenAI models plus the Foundry Claude models (see constants.go).
func (a *Adaptor) GetModelList() []string {
	return ModelList
}

// GetDefaultModelPricing merges OpenAI and Anthropic pricing so the channel
// pricing UI and global pricing fallback see both families' models.
func (a *Adaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	merged := make(map[string]adaptor.ModelConfig)
	for k, v := range a.Adaptor.GetDefaultModelPricing() {
		merged[k] = v
	}
	for k, v := range a.claude.GetDefaultModelPricing() {
		merged[k] = v
	}
	return merged
}

func (a *Adaptor) GetModelRatio(modelName string) float64 {
	if meta.IsClaudeModelName(modelName) {
		return a.claude.GetModelRatio(modelName)
	}
	return a.Adaptor.GetModelRatio(modelName)
}

func (a *Adaptor) GetCompletionRatio(modelName string) float64 {
	if meta.IsClaudeModelName(modelName) {
		return a.claude.GetCompletionRatio(modelName)
	}
	return a.Adaptor.GetCompletionRatio(modelName)
}

// DefaultToolingConfigForModel resolves built-in tool defaults per model family so
// Claude built-in tools (e.g. web search) bill at Anthropic rates and OpenAI models
// keep OpenAI tool defaults. Implementing ToolingDefaultsForModelProvider makes the
// tooling-policy builder prefer this over the model-agnostic DefaultToolingConfig.
func (a *Adaptor) DefaultToolingConfigForModel(modelName string) adaptor.ChannelToolConfig {
	if meta.IsClaudeModelName(modelName) {
		return a.claude.DefaultToolingConfig()
	}
	return a.Adaptor.DefaultToolingConfig()
}
