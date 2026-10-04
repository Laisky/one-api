package proxy

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor"
	channelhelper "github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

var _ adaptor.Adaptor = new(Adaptor)

const channelName = "proxy"

type Adaptor struct {
	adaptor.DefaultPricingMethods
}

// Init prepares the proxy adaptor with channel metadata. No-op for proxy.
func (a *Adaptor) Init(meta *meta.Meta) {
}

// ConvertRequest forwards an OpenAI-style request for native upstream handling.
func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("proxy adaptor received nil request")
	}
	return request, nil
}

// DoResponse preserves response bytes while returning observed usage to the
// caller's settlement policy. Explicit unmetered proxy routes remain unmetered.
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, m *meta.Meta) (*model.Usage, *model.ErrorWithStatusCode) {
	// The Responses chat-completions fallback installs a rewrite bridge. Keep
	// its shared handler: raw copying here would break /v1/responses streams.
	if openai_compatible.StreamRewriterFromContext(c) != nil {
		promptTokens, modelName := proxyUsageMetadata(m)
		streamErr, streamUsage := openai_compatible.StreamHandler(c, resp, promptTokens, modelName)
		return streamUsage, streamErr
	}
	return forwardProxyResponse(c, resp, m)
}

// GetModelList returns nil because proxy models are configured per channel.
func (a *Adaptor) GetModelList() (models []string) {
	return nil
}

// GetChannelName returns the identifier for the proxy channel.
func (a *Adaptor) GetChannelName() string {
	return channelName
}

// GetRequestURL removes the static proxy prefix from the upstream request URL.
func (a *Adaptor) GetRequestURL(meta *meta.Meta) (string, error) {
	prefix := fmt.Sprintf("/v1/oneapi/proxy/%d", meta.ChannelId)
	return meta.BaseURL + strings.TrimPrefix(meta.RequestURLPath, prefix), nil
}

// SetupRequestHeader copies only forwardable caller headers before installing
// the selected channel credential. Administrator custom headers are applied by
// DoRequestHelper after this function returns.
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) error {
	req.Header = adaptor.ForwardableRequestHeaders(req.Header)
	for name, values := range adaptor.ForwardableRequestHeaders(c.Request.Header) {
		req.Header[name] = values
	}
	req.Header.Set("Authorization", meta.APIKey)
	return nil
}

// ConvertImageRequest reports that image conversion is not implemented.
func (a *Adaptor) ConvertImageRequest(_ *gin.Context, request *model.ImageRequest) (any, error) {
	return nil, errors.Errorf("not implement")
}

// ConvertClaudeRequest returns the original Claude request for pass-through.
func (a *Adaptor) ConvertClaudeRequest(_ *gin.Context, request *model.ClaudeRequest) (any, error) {
	return request, nil
}

// DoRequest forwards the request using the shared upstream transport helper.
func (a *Adaptor) DoRequest(c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	return channelhelper.DoRequestHelper(a, c, meta, requestBody)
}
