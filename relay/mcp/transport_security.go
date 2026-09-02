package mcp

import (
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/common/network"
)

const maxMCPRedirects = 10

// mcpSecurityRoundTripper rejects credential-bearing requests that would leave the process over remote plaintext HTTP.
type mcpSecurityRoundTripper struct {
	base         http.RoundTripper
	credentialed bool
}

// RoundTrip validates the outbound request transport before delegating to the configured HTTP transport.
//
// Parameters:
//   - request: The outbound MCP HTTP request is checked before network I/O begins.
//
// Return values:
//   - *http.Response: The delegated transport response is returned for an allowed request.
//   - error: A transport-policy or delegated round-trip error is returned on failure.
func (transport mcpSecurityRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	if err := validateMCPOutboundTransport(request, transport.credentialed); err != nil {
		return nil, err
	}
	base := transport.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(request)
}

// httpClient returns an HTTP client that enforces MCP request and redirect transport boundaries.
//
// Parameters: none.
//
// Return values:
//   - *http.Client: The client uses the configured timeout and rejects unsafe plaintext or redirected credential delivery.
func (c *StreamableHTTPClient) httpClient() *http.Client {
	sensitive := hasSensitiveMCPClientState(c)
	client := &http.Client{
		Timeout: c.Timeout,
		Transport: mcpSecurityRoundTripper{
			base:         http.DefaultTransport,
			credentialed: sensitive,
		},
	}
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= maxMCPRedirects {
			return errors.WithStack(errors.New("stopped after 10 MCP redirects"))
		}
		if request == nil || request.URL == nil || len(via) == 0 || via[0] == nil || via[0].URL == nil {
			return nil
		}
		initial := via[0].URL
		if strings.EqualFold(initial.Scheme, "https") && !strings.EqualFold(request.URL.Scheme, "https") {
			return errors.WithStack(errors.New("MCP redirect would downgrade HTTPS to plaintext HTTP"))
		}
		if sensitive && !sameMCPOrigin(initial, request.URL) {
			return errors.WithStack(errors.New("credentialed MCP redirect must preserve the endpoint origin"))
		}
		return guardMCPRedirectTarget(initial, request.URL)
	}
	return client
}

// guardMCPRedirectTarget refuses a redirect that walks from a public MCP endpoint
// into private address space.
//
// The same-origin pin above only applies to credentialed clients, so an MCP
// server configured with auth_type=none (or whoever controls its DNS) could
// answer a tool call with 302 Location: http://169.254.169.254/... and have the
// gateway fetch it and hand the body back to the caller — a straightforward SSRF,
// since the upstream operator is not inside the admin's trust boundary.
//
// An endpoint the operator deliberately pointed at localhost or a LAN address is
// left alone: redirects that stay inside address space the operator already chose
// are consistent with that choice.
//
// This checks the redirect target's host, so it does not defend against a DNS
// rebind between this check and the dial; it closes the direct redirect path,
// which is the one an upstream controls.
//
// Parameters:
//   - initial: the originally configured endpoint URL.
//   - target: the URL the server is redirecting to.
//
// Return values:
//   - error: non-nil when a public endpoint redirects into private address space.
func guardMCPRedirectTarget(initial, target *url.URL) error {
	if initial == nil || target == nil {
		return nil
	}
	if isInternalMCPHost(initial.Hostname()) {
		return nil
	}
	if isInternalMCPHost(target.Hostname()) {
		return errors.WithStack(errors.New("MCP redirect from a public endpoint into private address space is not allowed"))
	}
	return nil
}

// isInternalMCPHost reports whether host denotes a loopback, private, link-local
// or otherwise non-public address.
//
// Parameters:
//   - host: a hostname or IP literal.
//
// Return values:
//   - bool: true when the host is not publicly routable.
func isInternalMCPHost(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" {
		return false
	}
	if isLoopbackMCPHostname(host) {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return network.IsForbiddenIP(ip)
	}
	addresses, err := net.LookupIP(host)
	if err != nil {
		return false
	}
	for _, ip := range addresses {
		if network.IsForbiddenIP(ip) {
			return true
		}
	}
	return false
}

// validateMCPOutboundTransport enforces HTTPS for credentialed remote MCP requests while retaining loopback development compatibility.
//
// Parameters:
//   - request: The outbound HTTP request supplies the destination URL.
//   - credentialed: The flag indicates whether the client carries API keys, authorization values, cookies, URL user information, or similar secrets.
//
// Return values:
//   - error: A policy error is returned when a credentialed request targets remote plaintext HTTP; otherwise nil is returned.
func validateMCPOutboundTransport(request *http.Request, credentialed bool) error {
	if !credentialed {
		return nil
	}
	if request == nil || request.URL == nil {
		return errors.WithStack(errors.New("credentialed MCP request URL is missing"))
	}
	if strings.EqualFold(request.URL.Scheme, "https") {
		return nil
	}
	if strings.EqualFold(request.URL.Scheme, "http") && isLoopbackMCPHostname(request.URL.Hostname()) {
		return nil
	}
	return errors.WithStack(errors.New("credentialed MCP requests require HTTPS unless the endpoint is a loopback host"))
}

// hasSensitiveMCPClientState reports whether an MCP client sends credentials with requests.
//
// Parameters:
//   - client: The client supplies configured headers and URL user information.
//
// Return values:
//   - bool: True is returned when redirects or plaintext transport could expose credentials.
func hasSensitiveMCPClientState(client *StreamableHTTPClient) bool {
	if client == nil {
		return false
	}
	for key, value := range client.headerSnapshot() {
		if strings.TrimSpace(value) == "" {
			continue
		}
		normalizedKey := http.CanonicalHeaderKey(strings.TrimSpace(key))
		if isSensitiveKey(strings.ToLower(normalizedKey)) {
			return true
		}
		switch normalizedKey {
		case "Accept", "Accept-Encoding", "Content-Type", "User-Agent", http.CanonicalHeaderKey(ProtocolVersionHeader), http.CanonicalHeaderKey(SessionIDHeader):
			continue
		default:
			// Arbitrary configured headers can implement custom authentication even
			// when their names do not contain a conventional credential token.
			return true
		}
	}
	parsed, err := url.Parse(client.BaseURL)
	return err == nil && parsed.User != nil
}

// isLoopbackMCPHostname reports whether a hostname is restricted to the local machine.
//
// Parameters:
//   - hostname: The URL hostname is checked as localhost or a loopback IP address.
//
// Return values:
//   - bool: True is returned only for localhost and IP loopback addresses.
func isLoopbackMCPHostname(hostname string) bool {
	hostname = strings.TrimSpace(hostname)
	if strings.EqualFold(hostname, "localhost") {
		return true
	}
	address := net.ParseIP(hostname)
	return address != nil && address.IsLoopback()
}

// sameMCPOrigin reports whether two endpoint URLs share scheme, hostname, and effective port.
//
// Parameters:
//   - left: The original MCP endpoint supplies the expected origin.
//   - right: The redirect destination is compared with the original origin.
//
// Return values:
//   - bool: True is returned only when the complete origins match.
func sameMCPOrigin(left, right *url.URL) bool {
	if left == nil || right == nil {
		return false
	}
	return strings.EqualFold(left.Scheme, right.Scheme) &&
		strings.EqualFold(left.Hostname(), right.Hostname()) &&
		effectiveMCPPort(left) == effectiveMCPPort(right)
}

// effectiveMCPPort returns the explicit port or the scheme's standard port.
//
// Parameters:
//   - endpoint: The URL supplies an optional explicit port and scheme.
//
// Return values:
//   - string: The effective port is returned, or an empty string for an unknown scheme.
func effectiveMCPPort(endpoint *url.URL) string {
	if endpoint == nil {
		return ""
	}
	if port := endpoint.Port(); port != "" {
		return port
	}
	switch strings.ToLower(endpoint.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}
