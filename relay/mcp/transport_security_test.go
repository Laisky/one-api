package mcp

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
)

// TestMCPClientRejectsCredentialedRemotePlaintextHTTP verifies the runtime transport blocks secrets before any remote plaintext network request begins.
//
// Parameters:
//   - t: The test owns client configurations and transport-policy assertions.
//
// Return values: none; failures are reported through t.
func TestMCPClientRejectsCredentialedRemotePlaintextHTTP(t *testing.T) {
	testCases := []struct {
		name    string
		server  model.MCPServer
		headers map[string]string
	}{
		{
			name:   "API key header",
			server: model.MCPServer{BaseURL: "http://example.invalid/mcp"},
			headers: map[string]string{
				"X-API-Key": "secret",
			},
		},
		{
			name:   "authorization header",
			server: model.MCPServer{BaseURL: "http://example.invalid/mcp"},
			headers: map[string]string{
				"Authorization": "Bearer secret",
			},
		},
		{
			name:   "custom authentication header",
			server: model.MCPServer{BaseURL: "http://example.invalid/mcp"},
			headers: map[string]string{
				"X-Tenant-Identity": "secret",
			},
		},
		{
			name:   "URL user information",
			server: model.MCPServer{BaseURL: "http://user:secret@example.invalid/mcp"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			client := NewStreamableHTTPClient(&testCase.server, testCase.headers, time.Second)
			_, err := client.DiscoverLatest(context.Background())
			require.ErrorContains(t, err, "require HTTPS")
		})
	}
}

// TestValidateMCPOutboundTransportPreservesExplicitCompatibility verifies plaintext remains available only when no credentials leave the host or the endpoint is loopback.
//
// Parameters:
//   - t: The test owns request fixtures and transport-policy assertions.
//
// Return values: none; failures are reported through t.
func TestValidateMCPOutboundTransportPreservesExplicitCompatibility(t *testing.T) {
	remoteRequest, err := http.NewRequest(http.MethodPost, "http://mcp.example.com/mcp", nil)
	require.NoError(t, err)
	require.NoError(t, validateMCPOutboundTransport(remoteRequest, false))

	loopbackRequest, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:8080/mcp", nil)
	require.NoError(t, err)
	require.NoError(t, validateMCPOutboundTransport(loopbackRequest, true))

	secureRequest, err := http.NewRequest(http.MethodPost, "https://mcp.example.com/mcp", nil)
	require.NoError(t, err)
	require.NoError(t, validateMCPOutboundTransport(secureRequest, true))
}
