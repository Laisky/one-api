package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMCPServerNormalizeAndValidateCredentialTransport verifies remote credentials require HTTPS while loopback development endpoints remain available.
//
// Parameters:
//   - t: The test owns table-driven server configurations and policy assertions.
//
// Return values: none; failures are reported through t.
func TestMCPServerNormalizeAndValidateCredentialTransport(t *testing.T) {
	testCases := []struct {
		name      string
		server    MCPServer
		wantError string
	}{
		{
			name: "remote bearer credentials require HTTPS",
			server: MCPServer{
				Name:     "remote-bearer",
				BaseURL:  "http://mcp.example.com/mcp",
				AuthType: MCPAuthTypeBearer,
				APIKey:   "secret",
			},
			wantError: "must use https",
		},
		{
			name: "remote sensitive header requires HTTPS",
			server: MCPServer{
				Name:    "remote-header",
				BaseURL: "http://mcp.example.com/mcp",
				Headers: JSONStringMap{"Authorization": "Bearer secret"},
			},
			wantError: "must use https",
		},
		{
			name: "remote custom authentication headers require HTTPS",
			server: MCPServer{
				Name:     "remote-custom",
				BaseURL:  "http://mcp.example.com/mcp",
				AuthType: MCPAuthTypeCustomHeaders,
				Headers:  JSONStringMap{"X-Tenant-Identity": "secret"},
			},
			wantError: "must use https",
		},
		{
			name: "remote URL user information requires HTTPS",
			server: MCPServer{
				Name:    "remote-userinfo",
				BaseURL: "http://user:secret@mcp.example.com/mcp",
			},
			wantError: "must use https",
		},
		{
			name: "credentialed HTTPS endpoint is accepted",
			server: MCPServer{
				Name:     "secure",
				BaseURL:  "https://mcp.example.com/mcp",
				AuthType: MCPAuthTypeAPIKey,
				APIKey:   "secret",
			},
		},
		{
			name: "credentialed IPv4 loopback HTTP endpoint is accepted",
			server: MCPServer{
				Name:     "loopback-v4",
				BaseURL:  "http://127.0.0.1:8080/mcp",
				AuthType: MCPAuthTypeAPIKey,
				APIKey:   "secret",
			},
		},
		{
			name: "credentialed IPv6 loopback HTTP endpoint is accepted",
			server: MCPServer{
				Name:     "loopback-v6",
				BaseURL:  "http://[::1]:8080/mcp",
				AuthType: MCPAuthTypeAPIKey,
				APIKey:   "secret",
			},
		},
		{
			name: "unauthenticated remote HTTP endpoint remains compatible",
			server: MCPServer{
				Name:    "public-http",
				BaseURL: "http://mcp.example.com/mcp",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.server.AutoSyncIntervalMinutes = 60
			err := testCase.server.NormalizeAndValidate()
			if testCase.wantError == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, testCase.wantError)
		})
	}
}
