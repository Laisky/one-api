package typesafe

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/logger"
	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestCatalog checks official IDs, input pricing and independent metadata copies.
func TestCatalog(t *testing.T) {
	a := &Adaptor{}
	require.Equal(t, []string{"jev-1.13.0", "jev-latest", "jev-preview"}, a.GetModelList())
	for name, config := range a.GetDefaultModelPricing() {
		require.Equal(t, 0.042*billingratio.MilliTokensUsd, config.Ratio)
		require.Equal(t, config.Ratio, a.GetModelRatio(name))
		require.Zero(t, a.GetCompletionRatio(name))
		require.Zero(t, config.CompletionRatio)
		require.EqualValues(t, 64000, config.ContextLength)
		require.Equal(t, []string{"text"}, config.InputModalities)
		require.Nil(t, config.PerCall)
		require.Empty(t, config.SupportedFeatures)
		config.InputModalities[0] = "image"
		require.Equal(t, []string{"text"}, a.GetDefaultModelPricing()[name].InputModalities)
	}
}

// TestURLsAndHeaders prevents cleartext credentials and normalizes version suffixes.
func TestURLsAndHeaders(t *testing.T) {
	a := &Adaptor{}
	for _, base := range []string{"", DefaultBaseURL, DefaultBaseURL + "/", DefaultBaseURL + "/v1/"} {
		url, err := a.GetRequestURL(&meta.Meta{BaseURL: base, Mode: relaymode.SystemOne})
		require.NoError(t, err)
		require.Equal(t, DefaultBaseURL+"/v1/systemone", url)
	}
	for _, base := range []string{"http://api.typesafe.ai", "https://user:pass@api.typesafe.ai", "https://api.typesafe.ai?key=secret", "https://api.typesafe.ai/#secret", "//api.typesafe.ai"} {
		_, err := a.GetRequestURL(&meta.Meta{BaseURL: base, Mode: relaymode.SystemOne})
		require.Error(t, err)
	}
	_, err := a.GetRequestURL(&meta.Meta{Mode: relaymode.ChatCompletions})
	require.Error(t, err)
	request := httptest.NewRequest(http.MethodPost, "http://api.typesafe.ai/v1/systemone", nil)
	require.Error(t, a.SetupRequestHeader(nil, request, &meta.Meta{APIKey: "secret"}))
	require.Empty(t, request.Header.Get("Authorization"))
	request = httptest.NewRequest(http.MethodPost, DefaultBaseURL+"/v1/systemone", nil)
	require.NoError(t, a.SetupRequestHeader(nil, request, &meta.Meta{APIKey: "provider-key"}))
	require.Equal(t, "Bearer provider-key", request.Header.Get("Authorization"))
	require.Equal(t, "application/json", request.Header.Get("Content-Type"))
	_, err = a.ConvertRequest(nil, relaymode.ChatCompletions, &model.GeneralOpenAIRequest{})
	require.Error(t, err)
	_, err = a.ConvertClaudeRequest(nil, &model.ClaudeRequest{})
	require.Error(t, err)
	_, err = a.ConvertImageRequest(nil, &model.ImageRequest{})
	require.Error(t, err)
}

// TestRedirectsAreNotReplayed exercises the actual shared HTTP transport.
func TestRedirectsAreNotReplayed(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Location", "/v1/systemone")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	previous := client.HTTPClient
	client.HTTPClient = server.Client()
	t.Cleanup(func() { client.HTTPClient = previous })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/systemone", nil)
	gmw.SetLogger(c, logger.Logger)
	_, err := (&Adaptor{}).DoRequest(c, &meta.Meta{BaseURL: server.URL, Mode: relaymode.SystemOne, APIKey: "key"}, strings.NewReader(`{}`))
	require.Error(t, err)
	require.EqualValues(t, 1, calls.Load())
	require.Nil(t, client.HTTPClient.CheckRedirect, "provider policy must not mutate the shared client")
}
