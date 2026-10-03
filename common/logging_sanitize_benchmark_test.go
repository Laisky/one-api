package common

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSanitizeURLForLoggingFastPathBehavior verifies the optimized sanitizer returns exactly the legacy output.
func TestSanitizeURLForLoggingFastPathBehavior(t *testing.T) {
	t.Parallel()
	tests := []string{
		"",
		"/v1/chat/completions",
		"/v1/chat/completions#fragment",
		"https://example.com/v1/responses",
		"%zz",
		"https://example.com/v1/messages?stream=true&foo=bar",
	}

	for _, rawURL := range tests {
		t.Run(rawURL, func(t *testing.T) {
			require.Equal(t, sanitizeURLForLoggingLegacy(rawURL), SanitizeURLForLogging(rawURL))
		})
	}
}

// sanitizeURLForLoggingLegacy preserves the pre-optimization implementation for side-by-side behavior and benchmarks.
func sanitizeURLForLoggingLegacy(rawURL string) string {
	if rawURL == "" {
		return rawURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	query := parsed.Query()
	changed := false
	for key := range query {
		if isSensitiveURLQueryKey(key) {
			query[key] = []string{"[redacted]"}
			changed = true
		}
	}
	if !changed {
		return rawURL
	}

	parsed.RawQuery = query.Encode()
	return parsed.String()
}

// BenchmarkSanitizeURLForLoggingNoQuery compares legacy and optimized handling of the common query-free request URL.
func BenchmarkSanitizeURLForLoggingNoQuery(b *testing.B) {
	const rawURL = "/v1/chat/completions"
	b.Run("legacy", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = sanitizeURLForLoggingLegacy(rawURL)
		}
	})
	b.Run("optimized", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = SanitizeURLForLogging(rawURL)
		}
	})
}

// BenchmarkSanitizeURLForLoggingSensitiveQuery verifies the redaction path keeps comparable cost and allocations.
func BenchmarkSanitizeURLForLoggingSensitiveQuery(b *testing.B) {
	const rawURL = "/api/user/register?turnstile=secret-token&page=1&api_key=secret-key"
	b.Run("legacy", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = sanitizeURLForLoggingLegacy(rawURL)
		}
	})
	b.Run("optimized", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = SanitizeURLForLogging(rawURL)
		}
	})
}
