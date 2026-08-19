package adaptor

import (
	"regexp"
	"strings"
)

var versionSuffixRe = regexp.MustCompile(`/v\d+[a-zA-Z0-9]*$`)

// NormalizeBaseURL trims whitespace and trailing slashes from baseURL.
// It returns the canonical base URL used for upstream request construction.
func NormalizeBaseURL(baseURL string) string {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/")
}

// NormalizeRequestPath trims whitespace from requestURL and ensures a leading slash when non-empty.
// It returns the canonical path segment used for upstream request construction.
func NormalizeRequestPath(requestURL string) string {
	path := strings.TrimSpace(requestURL)
	if path != "" && !strings.HasPrefix(path, "/") {
		return "/" + path
	}
	return path
}

// HasVersionSuffix reports whether baseURL ends with a version-like suffix such as /v1, /v4, or /v1beta.
// It returns true only when the final path segment matches that version contract.
func HasVersionSuffix(baseURL string) bool {
	return versionSuffixRe.MatchString(NormalizeBaseURL(baseURL))
}

// StripOpenAIV1Prefix removes a leading /v1 segment from path when it is a complete path segment.
// The exactV1Result parameter controls the return value when path is exactly /v1.
func StripOpenAIV1Prefix(path string, exactV1Result string) string {
	normalizedPath := NormalizeRequestPath(path)
	if normalizedPath == "/v1" {
		return exactV1Result
	}
	if strings.HasPrefix(normalizedPath, "/v1/") {
		return normalizedPath[len("/v1"):]
	}
	return normalizedPath
}

// JoinBaseURLAndPath concatenates baseURL and requestURL after canonical normalization.
// It returns the normalized base URL unchanged when requestURL is empty.
func JoinBaseURLAndPath(baseURL string, requestURL string) string {
	trimmedBase := NormalizeBaseURL(baseURL)
	path := NormalizeRequestPath(requestURL)
	if path == "" {
		return trimmedBase
	}
	return trimmedBase + path
}
