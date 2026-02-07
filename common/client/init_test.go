package client

import (
	"net/http"
	"github.com/songquanpeng/one-api/common/config"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInit(t *testing.T) {
	// Test that Init() creates properly configured HTTP clients
	Init()

	// Verify UserContentRequestHTTPClient is created
	require.NotNil(t, UserContentRequestHTTPClient)
	require.NotNil(t, UserContentRequestHTTPClient.Transport)

	// Verify it has a timeout set
	require.Greater(t, UserContentRequestHTTPClient.Timeout.Seconds(), 0.0)

	// Verify HTTP/2 is disabled (TLSNextProto should be empty map)
	if transport, ok := UserContentRequestHTTPClient.Transport.(*http.Transport); ok {
		require.NotNil(t, transport.TLSNextProto)
		require.Empty(t, transport.TLSNextProto)
	}

	// Verify other clients are created
	require.NotNil(t, HTTPClient)
	require.NotNil(t, ImpatientHTTPClient)
}

func TestUserContentRequestHTTPClient_SSRF(t *testing.T) {
	// 1. Test that UserContentRequestHTTPClient blocks internal IPs by default
	config.BlockInternalUserContentRequests = true
	Init()

	// Try to fetch from literal internal IP
	_, err := UserContentRequestHTTPClient.Get("http://127.0.0.1:12345")
	require.Error(t, err)
	require.Contains(t, err.Error(), "SSRF protection")

	// Try to fetch from hostname resolving to internal IP (localhost)
	_, err = UserContentRequestHTTPClient.Get("http://localhost:12345")
	require.Error(t, err)
	require.Contains(t, err.Error(), "SSRF protection")

	// 2. Test that protection can be disabled
	config.BlockInternalUserContentRequests = false
	Init()
	_, err = UserContentRequestHTTPClient.Get("http://127.0.0.1:12345")
	// Should fail with connection refused or timeout, not SSRF protection error
	require.Error(t, err)
	require.NotContains(t, err.Error(), "SSRF protection")

	// Cleanup
	config.BlockInternalUserContentRequests = true
	Init()
}

func TestUserContentRequestHTTPClient_ProxyExemption(t *testing.T) {
	// Test that connections to a configured internal proxy are allowed
	config.UserContentRequestProxy = "http://127.0.0.1:8080"
	config.BlockInternalUserContentRequests = true
	Init()

	// Try to fetch through the proxy
	// The dialer will try to connect to 127.0.0.1:8080.
	// Our Control should allow it because it matches the proxy.
	_, err := UserContentRequestHTTPClient.Get("http://example.com")

	// We expect a connection error (since no proxy is running),
	// but it should NOT be an SSRF protection error.
	require.Error(t, err)
	require.NotContains(t, err.Error(), "SSRF protection")

	// Cleanup
	config.UserContentRequestProxy = ""
	Init()
}
