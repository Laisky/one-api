package mcp

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGuardMCPRedirectTargetBlocksPublicToPrivateHop pins the SSRF guard on the
// MCP redirect path.
//
// The same-origin pin in httpClient only engages for credentialed clients, so an
// MCP server registered with auth_type=none — or anyone controlling its DNS —
// could answer a tool call with a redirect to link-local or private address space
// and have the gateway fetch it and return the body to the caller. The upstream
// operator is not inside the admin's trust boundary, so "the admin configured
// this URL" does not cover the redirect target.
func TestGuardMCPRedirectTargetBlocksPublicToPrivateHop(t *testing.T) {
	for _, tc := range []struct {
		name      string
		initial   string
		target    string
		wantError bool
	}{
		{
			name:      "public endpoint redirecting to the cloud metadata service",
			initial:   "https://mcp.example.com/mcp",
			target:    "http://169.254.169.254/latest/meta-data/iam/security-credentials/",
			wantError: true,
		},
		{
			name:      "public endpoint redirecting to a private LAN address",
			initial:   "https://mcp.example.com/mcp",
			target:    "http://10.0.0.5:8080/mcp",
			wantError: true,
		},
		{
			name:      "public endpoint redirecting to loopback",
			initial:   "https://mcp.example.com/mcp",
			target:    "http://127.0.0.1:9000/mcp",
			wantError: true,
		},
		{
			name:      "public endpoint redirecting to another public host stays allowed",
			initial:   "https://mcp.example.com/mcp",
			target:    "https://mcp-eu.example.com/mcp",
			wantError: false,
		},
		{
			name:      "an endpoint the operator pointed at loopback may redirect within it",
			initial:   "http://127.0.0.1:8080/mcp",
			target:    "http://127.0.0.1:9000/mcp",
			wantError: false,
		},
		{
			name:      "an endpoint the operator pointed at a LAN address may redirect within it",
			initial:   "http://192.168.1.10/mcp",
			target:    "http://192.168.1.11/mcp",
			wantError: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			initial, err := url.Parse(tc.initial)
			require.NoError(t, err)
			target, err := url.Parse(tc.target)
			require.NoError(t, err)

			err = guardMCPRedirectTarget(initial, target)
			if tc.wantError {
				require.Error(t, err, "redirect into private address space must be refused")
				return
			}
			require.NoError(t, err)
		})
	}
}
