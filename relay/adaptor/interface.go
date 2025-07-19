package adaptor

import (
	"io"
	"net/http"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
)

// ModelConfig represents pricing and configuration information for a model
// This structure consolidates both pricing (Ratio, CompletionRatio) and
// configuration (MaxTokens, etc.) to eliminate the need for separate ModelConfig
type ModelConfig struct {
	Ratio float64 `json:"ratio"`
	// CompletionRatio represents the output rate / input rate
	CompletionRatio float64 `json:"completion_ratio,omitempty"`
	// MaxTokens represents the maximum token limit for this model on this channel
	// 0 means no limit (infinity)
	MaxTokens int32 `json:"max_tokens,omitempty"`
}

type Adaptor interface {
	Init(meta *meta.Meta)
	GetRequestURL(meta *meta.Meta) (string, error)
	SetupRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) error
	ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error)
	ConvertImageRequest(c *gin.Context, request *model.ImageRequest) (any, error)
	ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error)
	DoRequest(c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error)
	DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode)
	GetModelList() []string
	GetChannelName() string

	// Pricing methods - each adapter manages its own model pricing
	GetDefaultModelPricing() map[string]ModelConfig
	GetModelRatio(modelName string) float64
	GetCompletionRatio(modelName string) float64
}

// DefaultPricingMethods provides default implementations for adapters without specific pricing
type DefaultPricingMethods struct{}

func (d *DefaultPricingMethods) GetDefaultModelPricing() map[string]ModelConfig {
	return make(map[string]ModelConfig) // Empty pricing map
}

func (d *DefaultPricingMethods) GetModelRatio(modelName string) float64 {
	// Fallback to a reasonable default
	return 2.5 * 0.000001 // 2.5 USD per million tokens
}

func (d *DefaultPricingMethods) GetCompletionRatio(modelName string) float64 {
	return 1.0 // Default completion ratio
}

func (d *DefaultPricingMethods) ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error) {
	// Default implementation: not supported
	return nil, errors.New("Claude Messages API not supported by this adaptor")
}

// GetModelListFromPricing derives model list from pricing map keys
// This eliminates the need for separate ModelList variables
func GetModelListFromPricing(pricing map[string]ModelConfig) []string {
	models := make([]string, 0, len(pricing))
	for model := range pricing {
		models = append(models, model)
	}
	return models
}
