package pricing

import (
	"io"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

type cwMockAdaptor struct {
	m map[string]adaptor.ModelConfig
}

func (a *cwMockAdaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig { return a.m }
func (a *cwMockAdaptor) GetModelRatio(modelName string) float64                 { return a.m[modelName].Ratio }
func (a *cwMockAdaptor) GetCompletionRatio(modelName string) float64 {
	return a.m[modelName].CompletionRatio
}

// Unused methods to satisfy interface
func (a *cwMockAdaptor) Init(meta *meta.Meta)                          {}
func (a *cwMockAdaptor) GetRequestURL(meta *meta.Meta) (string, error) { return "", nil }
func (a *cwMockAdaptor) SetupRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) error {
	return nil
}
func (a *cwMockAdaptor) ConvertRequest(c *gin.Context, relayMode int, request *relaymodel.GeneralOpenAIRequest) (any, error) {
	return nil, nil
}
func (a *cwMockAdaptor) ConvertImageRequest(c *gin.Context, request *relaymodel.ImageRequest) (any, error) {
	return nil, nil
}
func (a *cwMockAdaptor) ConvertClaudeRequest(c *gin.Context, request *relaymodel.ClaudeRequest) (any, error) {
	return nil, nil
}
func (a *cwMockAdaptor) DoRequest(c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	return nil, nil
}
func (a *cwMockAdaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (*relaymodel.Usage, *relaymodel.ErrorWithStatusCode) {
	return nil, nil
}
func (a *cwMockAdaptor) GetModelList() []string { return nil }
func (a *cwMockAdaptor) GetChannelName() string { return "mock" }

func TestResolveEffectivePricing_CacheWriteRatios(t *testing.T) {
	a := &cwMockAdaptor{m: map[string]adaptor.ModelConfig{
		"m": {Ratio: 1.0, CompletionRatio: 2.0, CachedInputRatio: 0.1, CacheWrite5mRatio: 1.25, CacheWrite1hRatio: 2.0},
	}}

	eff := ResolveEffectivePricing("m", 100, a)

	if eff.InputRatio != 1.0 || eff.OutputRatio != 2.0 {
		t.Fatalf("unexpected in/out: %v %v", eff.InputRatio, eff.OutputRatio)
	}
	if eff.CachedInputRatio != 0.1 {
		t.Fatalf("unexpected cached in: %v", eff.CachedInputRatio)
	}
	if eff.CacheWrite5mRatio != 1.25 || eff.CacheWrite1hRatio != 2.0 {
		t.Fatalf("unexpected cache write ratios: %v %v", eff.CacheWrite5mRatio, eff.CacheWrite1hRatio)
	}
}
