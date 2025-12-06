package xunfeiv2

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/adaptor/openai_compatible"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
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

// GetDefaultModelPricing returns the pricing information for XunfeiV2 models
// Based on Xunfei pricing: https://www.xfyun.cn/doc/spark/Web.html#_1-%E6%8E%A5%E5%8F%A3%E8%AF%B4%E6%98%8E
func (a *Adaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	const MilliTokensRmb = 3.5 // 0.000007 * 500000 = 3.5 quota per milli-token

	return map[string]adaptor.ModelConfig{
		// XunfeiV2 Models - Based on https://www.xfyun.cn/doc/spark/Web.html#_1-%E6%8E%A5%E5%8F%A3%E8%AF%B4%E6%98%8E
		"spark-lite":      {Ratio: 0.0 * MilliTokensRmb, CompletionRatio: 1},   // Free tier
		"spark-pro":       {Ratio: 0.003 * MilliTokensRmb, CompletionRatio: 1}, // CNY 0.003 / 1k tokens
		"spark-pro-128k":  {Ratio: 0.005 * MilliTokensRmb, CompletionRatio: 1}, // CNY 0.005 / 1k tokens
		"spark-max":       {Ratio: 0.005 * MilliTokensRmb, CompletionRatio: 1}, // CNY 0.005 / 1k tokens
		"spark-max-32k":   {Ratio: 0.008 * MilliTokensRmb, CompletionRatio: 1}, // CNY 0.008 / 1k tokens
		"spark-4.0-ultra": {Ratio: 0.005 * MilliTokensRmb, CompletionRatio: 1}, // CNY 0.005 / 1k tokens
	}
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
