package adaptor

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestSecurityCommonHeadersNeverCopyGatewayCredentials checks the real shared
// copy boundary and verifies that administrator-owned provider keys still work.
func TestSecurityCommonHeadersNeverCopyGatewayCredentials(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Request.Header.Set("X-Api-Key", "gateway-secret-fixture")
	c.Request.Header.Set("Authorization", "Bearer gateway-secret-fixture")
	c.Request.Header.Set("Api-Key", "gateway-secret-fixture")
	c.Request.Header.Set("Cookie", "session=gateway-secret-fixture")
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Add("X-Business-Header", "one")
	c.Request.Header.Add("X-Business-Header", "two")
	original := c.Request.Header.Clone()
	request, err := http.NewRequest(http.MethodPost, "http://provider.invalid/fixture", nil)
	require.NoError(t, err)
	m := &meta.Meta{IsStream: true, APIKey: "provider-secret-fixture"}
	SetupCommonRequestHeader(c, request, m)
	for _, values := range request.Header {
		for _, value := range values {
			require.NotContains(t, value, "gateway-secret-fixture")
		}
	}
	require.Equal(t, []string{"one", "two"}, request.Header.Values("X-Business-Header"))
	require.Equal(t, "text/event-stream", request.Header.Get("Accept"))
	require.Equal(t, "application/json", request.Header.Get("Content-Type"))
	require.Equal(t, original, c.Request.Header)
	m.Config = model.ChannelConfig{CustomHeaders: map[string]string{
		"X-Api-Key": "{{key}}", "Authorization": "Bearer {{key}}", "Api-Key": "{{key}}",
	}}
	require.NoError(t, applyChannelCustomHeaders(request, m))
	require.Equal(t, "provider-secret-fixture", request.Header.Get("X-Api-Key"))
	require.Equal(t, "provider-secret-fixture", request.Header.Get("Api-Key"))
	require.Equal(t, "Bearer provider-secret-fixture", request.Header.Get("Authorization"))
}

// TestSecurityForwardableHeadersPreserveOnlyEndToEndValues verifies repeated
// business headers, case-insensitive connection nominations and safe protocols.
func TestSecurityForwardableHeadersPreserveOnlyEndToEndValues(t *testing.T) {
	input := http.Header{
		"cOnNeCtIoN":    {"keep-alive, X-Hop-Fixture", "X-Second-Hop"},
		"X-Hop-Fixture": {"hop-one"}, "x-second-hop": {"hop-two"},
		"Proxy-Authorization": {"private-fixture"},
		"Keep-Alive":          {"timeout=5"}, "Transfer-Encoding": {"chunked"},
		"x-business":             {"one", "two"},
		"sec-websocket-protocol": {"realtime, OPENAI-INSECURE-API-KEY.private-fixture", "openai-beta.realtime-v1"},
	}
	original := input.Clone()
	output := ForwardableRequestHeaders(input)
	require.Equal(t, http.Header{
		"X-Business":             {"one", "two"},
		"Sec-Websocket-Protocol": {"realtime, openai-beta.realtime-v1"},
	}, output)
	require.Equal(t, original, input)
	output["X-Business"][0] = "changed"
	require.Equal(t, original, input, "returned values must not alias caller slices")
	require.Empty(t, ForwardableRequestHeaders(http.Header{
		"Sec-Websocket-Protocol": {"openai-insecure-api-key.private-fixture"},
	}))
}
