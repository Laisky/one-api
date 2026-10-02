package model

import (
	"net/url"
	"strings"
)

// SanitizeLogUpstreamEndpoint removes URL components that may contain provider credentials from an upstream endpoint before it is stored in user-visible log metadata. It accepts the raw adaptor-resolved endpoint and returns a copy without user info, query parameters, or fragments.
func SanitizeLogUpstreamEndpoint(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return ""
	}

	parsed, err := url.Parse(endpoint)
	if err != nil {
		return sanitizeRawLogUpstreamEndpoint(endpoint)
	}

	parsed.User = nil
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	parsed.RawFragment = ""
	return parsed.String()
}

// sanitizeRawLogUpstreamEndpoint strips credential-bearing URL suffixes from endpoints that cannot be parsed as standard URLs. It accepts a raw endpoint string and returns a conservative fallback without query strings, fragments, or user info.
func sanitizeRawLogUpstreamEndpoint(endpoint string) string {
	if cutAt := strings.IndexAny(endpoint, "?#"); cutAt >= 0 {
		endpoint = endpoint[:cutAt]
	}

	schemeSep := strings.Index(endpoint, "://")
	if schemeSep < 0 {
		return endpoint
	}

	authorityStart := schemeSep + len("://")
	remainder := endpoint[authorityStart:]
	authorityEnd := strings.IndexByte(remainder, '/')
	if authorityEnd < 0 {
		authorityEnd = len(remainder)
	}
	authority := remainder[:authorityEnd]
	if at := strings.LastIndexByte(authority, '@'); at >= 0 {
		endpoint = endpoint[:authorityStart] + authority[at+1:] + remainder[authorityEnd:]
	}

	return endpoint
}
