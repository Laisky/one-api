// Package zai implements the Z.AI international GLM channel (https://api.z.ai).
//
// Z.AI and open.bigmodel.cn are two brands of the same company serving the same
// GLM wire protocol at the same paths (/api/paas/v4/...) with near-identical
// model ids, so this adaptor embeds zhipu.Adaptor and inherits its entire
// request/response pipeline: the v3/v4 version split, ConvertRequest, the OCR
// (layout_parsing) routing, the image and async-video handlers, and the Claude
// Messages conversion.
//
// Only three things genuinely differ, and each one is overridden below:
//
//  1. Auth. Z.AI takes a plain `Authorization: Bearer <key>`. BigModel's adaptor
//     HS256 JWT-signs a dotted {id}.{secret} key, and zhipu.GetToken returns ""
//     for any key without exactly one dot -- which is the usual shape of a Z.AI
//     key. Inheriting it would send an empty Authorization header and surface as
//     an unexplained upstream 401.
//  2. Pricing. Z.AI publishes flat USD prices with no input-length or
//     output-length tiers; BigModel publishes tiered CNY prices for the very same
//     model ids. See constants.go.
//  3. Catalog. Z.AI serves no embeddings, no rerank, no text-to-speech and no
//     realtime surface.
//
// IMPORTANT -- Go embedding has no virtual dispatch. A promoted zhipu method
// binds its receiver to the *inner* zhipu.Adaptor, so an override that is not
// also reachable from the inner call chain is silently discarded at runtime.
// Two inner call chains force extra overrides here:
//
//   - zhipu.DoRequest calls adaptor.DoRequestHelper(a, ...) with the inner
//     receiver, and DoRequestHelper is what invokes GetRequestURL,
//     SetupRequestHeader and GetChannelName. Without the DoRequest override
//     below, the Bearer auth fix never runs. Same reason as azure.Adaptor.
//   - zhipu.GetModelRatio / GetCompletionRatio call a.GetDefaultModelPricing()
//     on the inner receiver, so overriding GetDefaultModelPricing alone still
//     bills Z.AI traffic at BigModel's CNY rates.
//
// If you ever override GetRequestURL or ConvertRequest, delegate to
// a.Adaptor.<method>(...): both call SetVersionByModeName, and skipping it
// leaves APIVersion empty so DoResponse falls into the v3 legacy branch.
package zai

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/zhipu"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/meta"
)

// Adaptor serves the Z.AI channel by reusing the whole Zhipu GLM pipeline and
// replacing only auth, pricing, and the advertised catalog.
type Adaptor struct {
	zhipu.Adaptor
}

var (
	_ adaptor.Adaptor                 = (*Adaptor)(nil)
	_ adaptor.OCRAdaptor              = (*Adaptor)(nil)
	_ adaptor.ToolingDefaultsProvider = (*Adaptor)(nil)
)

// SetupRequestHeader authenticates with a plain bearer token.
//
// This deliberately does NOT call zhipu.GetToken: that helper JWT-signs a dotted
// {id}.{secret} BigModel key and returns "" for anything else, which would send
// an empty Authorization header to Z.AI.
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Request, m *meta.Meta) error {
	adaptor.SetupCommonRequestHeader(c, req, m)
	req.Header.Set("Authorization", "Bearer "+m.APIKey)
	// Z.AI localizes upstream error text; en-US keeps relayed error messages English.
	req.Header.Set("Accept-Language", "en-US,en")
	return nil
}

// DoRequest re-dispatches through DoRequestHelper on the OUTER receiver so the
// SetupRequestHeader and GetChannelName overrides above actually run. Without
// this, zhipu's promoted DoRequest would bind to the embedded adaptor and send
// JWT-signed requests. Mirrors azure.Adaptor.DoRequest.
func (a *Adaptor) DoRequest(c *gin.Context, m *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	return adaptor.DoRequestHelper(a, c, m, requestBody)
}

// GetChannelName must be a constant: controller/model.go builds a fresh,
// non-Init'd adaptor when aggregating /v1/models.
func (a *Adaptor) GetChannelName() string {
	return "zai"
}

// GetModelList advertises only what Z.AI actually serves, which is a strict
// subset of BigModel's catalog plus a few Z.AI-only ids.
func (a *Adaptor) GetModelList() []string {
	return adaptor.GetModelListFromPricing(ModelRatios)
}

// GetDefaultModelPricing returns Z.AI's flat USD price table.
func (a *Adaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	return ModelRatios
}

// GetModelRatio must be overridden alongside GetDefaultModelPricing: zhipu's
// promoted implementation resolves pricing through the inner receiver and would
// return BigModel's CNY ratio for the same model id.
func (a *Adaptor) GetModelRatio(modelName string) float64 {
	if price, ok := ModelRatios[modelName]; ok {
		return price.Ratio
	}
	// Z.AI bills in USD; do not inherit zhipu's RMB-denominated default.
	return 2.5 * ratio.MilliTokensUsd
}

// GetCompletionRatio mirrors GetModelRatio; see the note there.
func (a *Adaptor) GetCompletionRatio(modelName string) float64 {
	if price, ok := ModelRatios[modelName]; ok {
		return price.CompletionRatio
	}
	return 1.0
}

// DefaultToolingConfig returns Z.AI's built-in tool pricing. BigModel's tiered
// search_std/search_pro/search_pro_sogou/search_pro_quark engines do not exist
// on Z.AI, which publishes a single flat $0.01-per-use web search.
func (a *Adaptor) DefaultToolingConfig() adaptor.ChannelToolConfig {
	return ZaiToolingDefaults
}
