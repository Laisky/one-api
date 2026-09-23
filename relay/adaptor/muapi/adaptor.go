// Package muapi contains the MuAPI video-generation adaptor.
package muapi

import (
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// Adaptor implements the MuAPI submit-and-poll video protocol behind the
// gateway's OpenAI-compatible /v1/videos surface.
type Adaptor struct {
	adaptor.DefaultPricingMethods
}

var _ adaptor.Adaptor = (*Adaptor)(nil)
var _ adaptor.VideoRequestPreparer = (*Adaptor)(nil)
var _ adaptor.VideoPricingEstimator = (*Adaptor)(nil)

// Init initializes the MuAPI adaptor for a request. MuAPI has no per-request
// adaptor state, so this method intentionally performs no work.
func (a *Adaptor) Init(_ *meta.Meta) {}

// GetRequestURL maps the gateway video lifecycle to MuAPI's model submit and
// prediction-result endpoints. Return values are the upstream URL or an error
// for unsupported operations.
func (a *Adaptor) GetRequestURL(metaInfo *meta.Meta) (string, error) {
	if metaInfo == nil {
		return "", errors.New("MuAPI request metadata is nil")
	}
	if metaInfo.Mode != relaymode.Videos {
		return "", errors.Errorf("MuAPI supports video generation only, got relay mode %d", metaInfo.Mode)
	}

	requestPath := stripQuery(metaInfo.RequestURLPath)
	if requestPath == "/v1/videos" || requestPath == "/v1/videos/generations" {
		modelName := strings.TrimSpace(metaInfo.ActualModelName)
		if !validMuAPIModelName(modelName) {
			return "", errors.Errorf("invalid MuAPI model slug %q", modelName)
		}
		return muAPICoreBaseURL(metaInfo.BaseURL) + "/" + modelName, nil
	}
	if strings.HasPrefix(requestPath, "/v1/videos/") {
		taskID := strings.TrimPrefix(requestPath, "/v1/videos/")
		if validMuAPITaskID(taskID) {
			return muAPICoreBaseURL(metaInfo.BaseURL) + "/predictions/" + taskID + "/result", nil
		}
	}
	return "", errors.New("MuAPI supports video creation and task polling only")
}

// SetupRequestHeader applies MuAPI's x-api-key authentication for video
// requests and copies the gateway's shared request headers.
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Request, metaInfo *meta.Meta) error {
	adaptor.SetupCommonRequestHeader(c, req, metaInfo)
	req.Header.Set("x-api-key", metaInfo.APIKey)
	req.Header.Del("Authorization")
	return nil
}

// ConvertRequest rejects chat-style requests because the MuAPI adaptor is
// intentionally scoped to the provider's unified media job protocol.
func (a *Adaptor) ConvertRequest(_ *gin.Context, relayMode int, _ *model.GeneralOpenAIRequest) (any, error) {
	return nil, errors.Errorf("MuAPI does not expose relay mode %d through this adaptor", relayMode)
}

// ConvertImageRequest rejects image requests because MuAPI's OpenAI-compatible
// image surface is configured through the generic OpenAI-compatible channel;
// this adaptor owns only the native multi-model video job lifecycle.
func (a *Adaptor) ConvertImageRequest(_ *gin.Context, _ *model.ImageRequest) (any, error) {
	return nil, errors.New("MuAPI image generation uses the OpenAI-compatible channel; native MuAPI video is supported here")
}

// DoRequest validates the MuAPI video operation and dispatches it through the
// shared REST helper. It returns the upstream response or a wrapped error.
func (a *Adaptor) DoRequest(c *gin.Context, metaInfo *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	if metaInfo == nil || metaInfo.Mode != relaymode.Videos {
		return nil, errors.New("MuAPI adaptor received a non-video request")
	}
	if err := validateMuAPIVideoOperation(c); err != nil {
		return nil, err
	}
	if c != nil && c.Request != nil && c.Request.Method == http.MethodPost {
		var err error
		requestBody, err = prepareMuAPIVideoBody(c, requestBody)
		if err != nil {
			return nil, err
		}
	}
	return adaptor.DoRequestHelper(a, c, metaInfo, requestBody)
}

// DoResponse validates and forwards MuAPI's native asynchronous response while
// persisting accepted task ids for authenticated follow-up polling.
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, metaInfo *meta.Meta) (*model.Usage, *model.ErrorWithStatusCode) {
	if metaInfo == nil || metaInfo.Mode != relaymode.Videos {
		return nil, openai.ErrorWrapper(errors.New("MuAPI adaptor received a non-video response"), "invalid_api_type", http.StatusBadRequest)
	}
	return a.handleVideoResponse(c, resp)
}

// GetModelList returns representative discovery models while allowing any
// provider model slug to be configured from MuAPI's live catalog. The returned
// slice is for catalog discovery only and is not a routing allowlist.
func (a *Adaptor) GetModelList() []string {
	return append([]string(nil), discoveryModels...)
}

// GetChannelName returns the stable provider identifier used in logs and model
// listings.
func (a *Adaptor) GetChannelName() string {
	return "muapi"
}

// GetDefaultModelPricing returns no frozen tariff table. MuAPI pricing is
// model- and request-dependent; video requests use EstimateVideoPricing before
// quota admission, and channel overrides remain available for operators.
func (a *Adaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{}
}

var discoveryModels = []string{
	"veo3-fast",
	"kling-v2.6-pro",
	"seedance-2.5-text-to-video",
	"wan2.6-t2v",
	"runway-gen-4",
}
