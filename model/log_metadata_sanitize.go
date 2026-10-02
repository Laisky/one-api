package model

import (
	"maps"
	"net/url"
	"strings"
)

// SanitizeLogUpstreamEndpoint removes credentials from an upstream diagnostic
// URL without modifying the actual request. Parameters: endpoint is untrusted
// URL text. Returns: a query-, fragment-, and userinfo-free HTTP(S)/WS(S) URL or
// root-relative path; malformed and opaque inputs fail closed to an empty value.
func SanitizeLogUpstreamEndpoint(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return ""
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Opaque != "" {
		return ""
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "ws", "wss":
		if parsed.Host == "" {
			return ""
		}
	case "":
		if parsed.Host == "" && !strings.HasPrefix(parsed.Path, "/") {
			return ""
		}
	default:
		return ""
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	parsed.RawFragment = ""
	return parsed.String()
}

// SanitizeLogMetadata sanitizes upstream URL metadata for persistence and direct
// JSON serialization. Parameters: metadata is caller-owned. Returns: the original
// map when no endpoint exists, otherwise a shallow copy with a sanitized endpoint.
// Non-string endpoints are removed rather than serializing arbitrary objects.
func SanitizeLogMetadata(metadata LogMetadata) LogMetadata {
	endpoint, exists := metadata[LogMetadataKeyUpstreamEndpoint]
	if !exists {
		return metadata
	}
	clean := maps.Clone(metadata)
	delete(clean, LogMetadataKeyUpstreamEndpoint)
	if raw, ok := endpoint.(string); ok {
		if endpoint := SanitizeLogUpstreamEndpoint(raw); endpoint != "" {
			clean[LogMetadataKeyUpstreamEndpoint] = endpoint
		}
	}
	return clean
}

// publicLogMetadata omits operator-owned upstream URLs from every external log
// DTO, including historical records. Parameters: metadata is caller-owned.
// Returns: the original map if no endpoint exists, otherwise a shallow copy.
// Keeping this at the DTO boundary also protects cursor, token, and export APIs.
func publicLogMetadata(metadata LogMetadata) map[string]any {
	if _, exists := metadata[LogMetadataKeyUpstreamEndpoint]; !exists {
		return metadata
	}
	clean := maps.Clone(metadata)
	delete(clean, LogMetadataKeyUpstreamEndpoint)
	return clean
}
