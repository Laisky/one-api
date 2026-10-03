package deepseekcompat

import (
	"net"
	"net/url"
	"strings"

	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
)

// IsDeepSeekModel reports whether modelName belongs to the DeepSeek model family.
//
// Parameters:
//   - modelName: the raw or mapped model identifier.
//
// Returns:
//   - bool: true when the normalized model name has the DeepSeek prefix.
func IsDeepSeekModel(modelName string) bool {
	normalized := strings.TrimSpace(strings.ToLower(modelName))
	if normalized == "" {
		return false
	}
	return strings.HasPrefix(normalized, "deepseek")
}

// UsesDeepSeekAPIContract reports whether meta points at DeepSeek's own API
// contract rather than a third-party host of DeepSeek weights.
//
// Parameters:
//   - metaInfo: request metadata containing channel type and base URL.
//
// Returns:
//   - bool: true for dedicated DeepSeek channels or hosts under deepseek.com.
func UsesDeepSeekAPIContract(metaInfo *meta.Meta) bool {
	if metaInfo == nil {
		return false
	}
	if metaInfo.ChannelType == channeltype.DeepSeek {
		return true
	}
	return HostUsesDeepSeekAPIContract(metaInfo.BaseURL)
}

// HostUsesDeepSeekAPIContract reports whether rawURL has a deepseek.com host.
//
// Parameters:
//   - rawURL: the upstream base URL, with or without a scheme.
//
// Returns:
//   - bool: true for deepseek.com or any subdomain of deepseek.com.
func HostUsesDeepSeekAPIContract(rawURL string) bool {
	host := normalizedHost(rawURL)
	return host == "deepseek.com" || strings.HasSuffix(host, ".deepseek.com")
}

// normalizedHost extracts a lowercase hostname from rawURL.
//
// Parameters:
//   - rawURL: the input URL or host-like string.
//
// Returns:
//   - string: the normalized hostname without a port, or an empty string.
func normalizedHost(rawURL string) string {
	value := strings.TrimSpace(rawURL)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err == nil && parsed.Host != "" {
		return lowerHostname(parsed.Host)
	}
	if !strings.Contains(value, "://") {
		parsed, err = url.Parse("https://" + value)
		if err == nil && parsed.Host != "" {
			return lowerHostname(parsed.Host)
		}
	}
	return ""
}

// lowerHostname lowercases host and removes any port.
//
// Parameters:
//   - host: a host or host:port value.
//
// Returns:
//   - string: a lowercase hostname without port.
func lowerHostname(host string) string {
	hostname, _, err := net.SplitHostPort(host)
	if err != nil {
		hostname = host
	}
	return strings.ToLower(strings.Trim(hostname, "[] \t\r\n."))
}
