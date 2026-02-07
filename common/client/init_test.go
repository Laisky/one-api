package client

import (
	"net/http"
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
	// Test that UserContentRequestHTTPClient blocks internal IPs
	Init()

	// Try to fetch from localhost (which is an internal IP)
	// We use a random port that is likely not listening to avoid connection refused
	// but the DialControl should block it before it even tries to connect.
	_, err := UserContentRequestHTTPClient.Get("http://127.0.0.1:12345")
	require.Error(t, err)
	require.Contains(t, err.Error(), "SSRF protection")
	require.Contains(t, err.Error(), "blocked")
}
