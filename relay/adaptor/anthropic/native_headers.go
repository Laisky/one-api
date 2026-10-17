package anthropic

import (
	"net/http"
	"strings"

	"github.com/Laisky/one-api/relay/adaptor"
)

// copyClaudeProtocolHeaders preserves open-ended Anthropic protocol fields after
// filtering gateway credentials and hop-by-hop headers. Parameters: destination
// receives independent header values; source is caller-owned. Returns: the
// sanitized caller headers for version and beta normalization.
func copyClaudeProtocolHeaders(destination, source http.Header) http.Header {
	headers := adaptor.ForwardableRequestHeaders(source)
	for key, values := range headers {
		if strings.HasPrefix(strings.ToLower(key), "anthropic-") {
			destination[key] = append([]string(nil), values...)
		}
	}
	return headers
}

// CopyClaudeResponseHeaders forwards correlation, retry and Anthropic rate-limit
// metadata, excluding cookies, credentials, arbitrary and hop-by-hop fields.
// Parameters: destination is the downstream header map; source is upstream.
// Returns: nothing; every selected value is independently copied.
func CopyClaudeResponseHeaders(destination, source http.Header) {
	for key, values := range adaptor.ForwardableRequestHeaders(source) {
		lower := strings.ToLower(key)
		if lower == "apim-request-id" || lower == "request-id" || lower == "x-request-id" || lower == "retry-after" || strings.HasPrefix(lower, "anthropic-ratelimit-") {
			destination[key] = append([]string(nil), values...)
		}
	}
}
