package aiproxy

import (
	"fmt"
	"io"
	"net/http"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

type Adaptor struct {
	meta *meta.Meta
	adaptor.DefaultPricingMethods
}

func (a *Adaptor) Init(meta *meta.Meta) {
	a.meta = meta
}

func (a *Adaptor) GetRequestURL(meta *meta.Meta) (string, error) {
	return fmt.Sprintf("%s/api/library/ask", meta.BaseURL), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) error {
	adaptor.SetupCommonRequestHeader(c, req, meta)
	req.Header.Set("Authorization", "Bearer "+meta.APIKey)
	return nil
}

func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	aiProxyLibraryRequest := ConvertRequest(*request)
	aiProxyLibraryRequest.LibraryId = a.meta.Config.LibraryID
	return aiProxyLibraryRequest, nil
}

func (a *Adaptor) ConvertImageRequest(_ *gin.Context, request *model.ImageRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return request, nil
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error) {
	// Use the shared OpenAI-compatible Claude Messages conversion
	return openai_compatible.ConvertClaudeRequest(c, request)
}

func (a *Adaptor) DoRequest(c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	return adaptor.DoRequestHelper(a, c, meta, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	if meta.IsStream {
		err, usage = StreamHandler(c, resp)
	} else {
		err, usage = Handler(c, resp)
	}
	return
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return "aiproxy"
}

// GetDefaultModelPricing returns the pricing table this channel advertises.
//
// constants.go sets ModelRatios = openai.ModelRatios because AIProxy resells the
// OpenAI catalog. Without this override the embedded DefaultPricingMethods answered
// a flat 2.5 USD/1M with completion ratio 1 for every model, so cheap models were
// over-charged and expensive completions under-charged.
//
// Return values:
//   - map[string]adaptor.ModelConfig: the audited pricing keyed by model id.
func (a *Adaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	return ModelRatios
}

// GetModelRatio returns the input ratio for modelName, falling back to the
// framework default only for models this channel does not publish.
//
// Parameters:
//   - modelName: the requested model id.
//
// Return values:
//   - float64: quota per input token.
func (a *Adaptor) GetModelRatio(modelName string) float64 {
	if price, exists := ModelRatios[modelName]; exists {
		return price.Ratio
	}
	return a.DefaultPricingMethods.GetModelRatio(modelName)
}

// GetCompletionRatio returns the output multiplier for modelName, falling back to
// the framework default only for models this channel does not publish.
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

// DefaultToolingConfig mirrors OpenAI's defaults because AIProxy relays OpenAI tool calls.
func (a *Adaptor) DefaultToolingConfig() adaptor.ChannelToolConfig {
	return AIProxyToolingDefaults
}
