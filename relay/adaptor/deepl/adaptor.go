package deepl

import (
	"fmt"
	"io"
	"net/http"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

type Adaptor struct {
	meta       *meta.Meta
	promptText string
	adaptor.DefaultPricingMethods
}

func (a *Adaptor) Init(meta *meta.Meta) {
	a.meta = meta
}

func (a *Adaptor) GetRequestURL(meta *meta.Meta) (string, error) {
	return fmt.Sprintf("%s/v2/translate", meta.BaseURL), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) error {
	adaptor.SetupCommonRequestHeader(c, req, meta)
	req.Header.Set("Authorization", "DeepL-Auth-Key "+meta.APIKey)
	return nil
}

func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	convertedRequest, text := ConvertRequest(*request)
	a.promptText = text
	return convertedRequest, nil
}

func (a *Adaptor) ConvertImageRequest(_ *gin.Context, request *model.ImageRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return request, nil
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error) {
	// DeepL is a translation service, not a chat completion service
	// Claude Messages API is not applicable for translation
	return nil, errors.New("Claude Messages API not supported by DeepL translation service")
}

func (a *Adaptor) DoRequest(c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	return adaptor.DoRequestHelper(a, c, meta, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	if meta.IsStream {
		err = StreamHandler(c, resp, meta.ActualModelName)
	} else {
		err = Handler(c, resp, meta.ActualModelName)
	}
	promptTokens := len(a.promptText)
	usage = &model.Usage{
		PromptTokens: promptTokens,
		TotalTokens:  promptTokens,
	}
	return
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return "deepl"
}

// GetDefaultModelPricing returns DeepL's per-source-character pricing.
//
// Return values:
//   - map[string]adaptor.ModelConfig: the audited pricing keyed by model id.
func (a *Adaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	return ModelRatios
}

// GetModelRatio returns the per-source-character ratio for modelName.
//
// Parameters:
//   - modelName: the requested model id.
//
// Return values:
//   - float64: quota per source character.
func (a *Adaptor) GetModelRatio(modelName string) float64 {
	if price, exists := ModelRatios[modelName]; exists {
		return price.Ratio
	}
	return a.DefaultPricingMethods.GetModelRatio(modelName)
}

// GetCompletionRatio returns the output multiplier for modelName. DeepL bills
// source characters only, so this is never applied to real usage.
//
// Parameters:
//   - modelName: the requested model id.
//
// Return values:
//   - float64: output-to-input price multiplier.
func (a *Adaptor) GetCompletionRatio(modelName string) float64 {
	if price, exists := ModelRatios[modelName]; exists {
		return price.CompletionRatio
	}
	return a.DefaultPricingMethods.GetCompletionRatio(modelName)
}

// DefaultToolingConfig returns DeepL tooling defaults (translation API has no separate tool metering).
func (a *Adaptor) DefaultToolingConfig() adaptor.ChannelToolConfig {
	return DeepLToolingDefaults
}
