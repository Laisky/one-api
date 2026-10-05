package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/meta"
)

// TestSecurityProxyCredentialsNeverReachUpstream exercises the actual header
// boundary and HTTP transport against an isolated provider recorder. No real
// credentials or external services are used.
func TestSecurityProxyCredentialsNeverReachUpstream(t *testing.T) {
	const gatewayKey = "sk-gateway-security-fixture"
	const upstreamKey = "Bearer upstream-security-fixture"
	cases := []struct {
		name     string
		headers  http.Header
		protocol string
	}{
		{"authorization", http.Header{"Authorization": {"Bearer " + gatewayKey}}, ""},
		{"anthropic", http.Header{"X-Api-Key": {gatewayKey}}, ""},
		{"azure", http.Header{"Api-Key": {gatewayKey}}, ""},
		{"cookie", http.Header{"Cookie": {"session=" + gatewayKey}}, ""},
		{"websocket", http.Header{"Sec-Websocket-Protocol": {"realtime, openai-insecure-api-key." + gatewayKey + ", openai-beta.realtime-v1"}}, "realtime, openai-beta.realtime-v1"},
		{"mixed-and-repeated", http.Header{
			"Authorization":       {"Bearer " + gatewayKey},
			"X-Api-Key":           {gatewayKey, gatewayKey + "-second"},
			"Api-Key":             {gatewayKey},
			"Cookie":              {"session=" + gatewayKey},
			"Proxy-Authorization": {"Bearer " + gatewayKey},
		}, ""},
		{"noncanonical-header-map", http.Header{"x-api-key": {gatewayKey}, "aPi-KeY": {gatewayKey}, "cookie": {"session=" + gatewayKey}}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			received := make(chan http.Header, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received <- r.Header.Clone()
				w.WriteHeader(http.StatusNoContent)
			}))
			defer upstream.Close()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/oneapi/proxy/1/fixture", strings.NewReader(`{}`))
			c.Request.Header = tc.headers.Clone()
			c.Request.Header.Set("Content-Type", "application/json")
			c.Request.Header.Set("X-Request-Id", "business-fixture")
			original := c.Request.Header.Clone()
			req, err := http.NewRequest(http.MethodPost, upstream.URL, strings.NewReader(`{}`))
			require.NoError(t, err)
			err = (&Adaptor{}).SetupRequestHeader(c, req, &meta.Meta{APIKey: upstreamKey})
			require.NoError(t, err)
			resp, err := upstream.Client().Do(req)
			require.NoError(t, err)
			_, err = io.Copy(io.Discard, resp.Body)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			headers := <-received
			for name, values := range headers {
				for _, value := range values {
					require.NotContains(t, value, gatewayKey, "gateway credential escaped in %s", name)
				}
			}
			require.Equal(t, upstreamKey, headers.Get("Authorization"))
			require.Equal(t, "application/json", headers.Get("Content-Type"))
			require.Equal(t, "business-fixture", headers.Get("X-Request-Id"))
			require.Equal(t, tc.protocol, headers.Get("Sec-WebSocket-Protocol"))
			require.Equal(t, original, c.Request.Header, "forwarding must not mutate inbound authentication state")
		})
	}
}
