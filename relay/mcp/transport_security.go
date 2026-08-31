package mcp

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/Laisky/errors/v2"
)

const maxMCPRedirects = 10

// httpClient returns an HTTP client that enforces MCP redirect transport boundaries.
//
// Parameters: none.
//
// Return values:
//   - *http.Client: The client uses the configured timeout and rejects unsafe redirects.
func (c *StreamableHTTPClient) httpClient() *http.Client {
	client := &http.Client{Timeout: c.Timeout}
	sensitive := hasSensitiveMCPClientState(c)
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
		return nil
	}
	return client
}

// hasSensitiveMCPClientState reports whether an MCP client sends credentials with requests.
//
// Parameters:
//   - client: The client supplies configured headers and URL user information.
//
// Return values:
//   - bool: True is returned when redirects could expose credentials.
func hasSensitiveMCPClientState(client *StreamableHTTPClient) bool {
	if client == nil {
		return false
	}
	for key, value := range client.headerSnapshot() {
		if strings.TrimSpace(value) != "" && isSensitiveKey(strings.ToLower(strings.TrimSpace(key))) {
			return true
		}
	}
	parsed, err := url.Parse(client.BaseURL)
	return err == nil && parsed.User != nil
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
