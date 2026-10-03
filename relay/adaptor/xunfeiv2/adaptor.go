package xunfeiv2

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

type Adaptor struct {
	adaptor.DefaultPricingMethods
}

func (a *Adaptor) Init(meta *meta.Meta) {}

func (a *Adaptor) GetRequestURL(meta *meta.Meta) (string, error) {
	return "", nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) error {
	return nil
}

func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	return nil, nil
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, request *model.ImageRequest) (any, error) {
	return nil, nil
}

// ConvertClaudeRequest converts Claude Messages API request to OpenAI format for xunfeiv2
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error) {
	return openai_compatible.ConvertClaudeRequest(c, request)
}

func (a *Adaptor) DoRequest(c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	return nil, nil
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	return nil, nil
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return "xunfeiv2"
}

// GetDefaultModelPricing returns the audited XunfeiV2 pricing table.
//
// It must stay the same map ModelList is derived from: this method previously
// returned a hand-written map keyed by marketing names ("spark-pro", "spark-max")
// while the channel advertises the upstream domain ids ("generalv3", "max-32k").
// The two key sets did not intersect, so every real request missed and fell back
// to the 2.5 USD/1M default in DefaultPricingMethods.
//
// Return values:
//   - map[string]adaptor.ModelConfig: pricing keyed by the ids this channel serves.
func (a *Adaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	return ModelRatios
}

func (a *Adaptor) GetModelRatio(modelName string) float64 {
	pricing := a.GetDefaultModelPricing()
	if price, exists := pricing[modelName]; exists {
		return price.Ratio
	}
	// Use default fallback from DefaultPricingMethods
	return a.DefaultPricingMethods.GetModelRatio(modelName)
}

func (a *Adaptor) GetCompletionRatio(modelName string) float64 {
	pricing := a.GetDefaultModelPricing()
	if price, exists := pricing[modelName]; exists {
		return price.CompletionRatio
	}
	// Use default fallback from DefaultPricingMethods
	return a.DefaultPricingMethods.GetCompletionRatio(modelName)
}

// DefaultToolingConfig returns Xunfei V2 tooling defaults (no published tool billing as of 2025-11-12).
func (a *Adaptor) DefaultToolingConfig() adaptor.ChannelToolConfig {
	return XunfeiV2ToolingDefaults
}
