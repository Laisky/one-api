package gemini

import (
	"net/url"
	"strings"
)

// IsGoogleGenerativeAPIHost reports whether host serves Google's Gemini Developer API or Vertex AI.
// Parameters: host is a hostname without port, in any letter case, with or without a trailing dot.
// Returns true for generativelanguage.googleapis.com, aiplatform.googleapis.com and regional
// *-aiplatform.googleapis.com hosts.
func IsGoogleGenerativeAPIHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	return host == "generativelanguage.googleapis.com" || host == "aiplatform.googleapis.com" || strings.HasSuffix(host, "-aiplatform.googleapis.com")
}

// IsGoogleGenerativeAPIBaseURL reports whether a channel base URL targets a Google generative API host,
// such as a generic OpenAI-compatible channel pointed at Google's OpenAI-compatible endpoint.
// Parameters: baseURL is the channel's configured base URL. Returns false for unparsable URLs.
func IsGoogleGenerativeAPIBaseURL(baseURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return false
	}
	return IsGoogleGenerativeAPIHost(parsed.Hostname())
}
