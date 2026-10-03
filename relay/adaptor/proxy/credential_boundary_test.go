package proxy

import (
	"github.com/Laisky/one-api/relay/meta"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSecurityProxyCredentialBoundary verifies only configured provider credentials cross the upstream boundary.
func TestSecurityProxyCredentialBoundary(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/", nil)
	for _, h := range []string{"Authorization", "Api-Key", "X-Api-Key", "Cookie", "Proxy-Authorization", "Sec-WebSocket-Protocol"} {
		c.Request.Header.Set(h, "local-fixture-secret")
	}
	c.Request.Header.Set("X-Request-Id", "fixture-request")
	r, _ := http.NewRequest("POST", "http://fixture.invalid", nil)
	require.NoError(t, (&Adaptor{}).SetupRequestHeader(c, r, &meta.Meta{APIKey: "provider-fixture"}))
	require.Equal(t, "provider-fixture", r.Header.Get("Authorization"))
	for _, h := range []string{"Api-Key", "X-Api-Key", "Cookie", "Proxy-Authorization", "Sec-WebSocket-Protocol"} {
		require.Empty(t, r.Header.Get(h), h)
	}
	require.Equal(t, "fixture-request", r.Header.Get("X-Request-Id"))
}
