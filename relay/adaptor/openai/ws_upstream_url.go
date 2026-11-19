package openai

import (
	"net/url"
	"regexp"

	"github.com/Laisky/errors/v2"
)

// ErrInvalidProviderWebSocketURL reports a provider WebSocket handshake URL
// that violates ValidateProviderWebSocketURL. It never carries the URL, which
// may embed configuration secrets.
var ErrInvalidProviderWebSocketURL = errors.New("invalid provider WebSocket URL")

// providerWebSocketURLPattern is the complete grammar of a provider WebSocket
// handshake URL: a ws/wss scheme; a DNS name, IPv4 address or bracketed IPv6
// literal with an optional numeric port; and an optional percent-encoded path
// and query. It admits no userinfo, fragment, whitespace or control characters,
// so a dial target is exactly "scheme://host[:port][/path][?query]".
var providerWebSocketURLPattern = regexp.MustCompile(
	`^wss?://` +
		`(?:[A-Za-z0-9_%-]+(?:\.[A-Za-z0-9_%-]+)*\.?|\[[0-9A-Fa-f:.]+(?:%25[A-Za-z0-9._~-]+)?\])` +
		`(?::[0-9]{1,5})?` +
		`(?:/[A-Za-z0-9._~!$&'()*+,;=:@%/-]*)?` +
		`(?:\?[A-Za-z0-9._~!$&'()*+,;=:@%/?-]*)?$`)

// ValidateProviderWebSocketURL is the shared dial-site policy for provider
// WebSocket handshakes (OpenAI Realtime, OpenAI Responses WebSocket mode and
// Gemini/Vertex Live). The host and path come only from administrator channel
// configuration, and every builder rebuilds the query from gateway decisions;
// this check runs on the final string immediately before dialing, so a URL
// that is not a plain ws/wss target is never dialed.
//
// Parameters:
//   - candidate: the fully built provider handshake URL.
//
// Returns:
//   - string: candidate unchanged when it satisfies the policy.
//   - error: ErrInvalidProviderWebSocketURL (wrapped) when candidate does not
//     parse, uses a scheme other than ws/wss, carries userinfo, an opaque part
//     or a fragment, or falls outside providerWebSocketURLPattern.
func ValidateProviderWebSocketURL(candidate string) (string, error) {
	parsed, err := url.Parse(candidate)
	if err != nil {
		// url.Parse errors quote the input, which may embed secrets; the parse
		// failure is fully handled by reporting the policy violation instead.
		return "", errors.Wrap(ErrInvalidProviderWebSocketURL, "URL does not parse")
	}
	if (parsed.Scheme != "ws" && parsed.Scheme != "wss") || parsed.Host == "" ||
		parsed.User != nil || parsed.Opaque != "" || parsed.Fragment != "" {
		return "", errors.Wrap(ErrInvalidProviderWebSocketURL, "URL is not a credential-free ws/wss target")
	}
	if !providerWebSocketURLPattern.MatchString(candidate) {
		return "", errors.Wrap(ErrInvalidProviderWebSocketURL, "URL contains characters outside the provider URL grammar")
	}
	return candidate, nil
}
