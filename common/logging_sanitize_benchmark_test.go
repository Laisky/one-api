package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizeURLForLoggingFastPathBehavior(t *testing.T) {
	t.Parallel()
	tests := []string{
		"",
		"/v1/chat/completions",
		"/v1/chat/completions#fragment",
		"https://example.com/v1/responses",
		"https://example.com/v1/messages?stream=true&foo=bar",
	}

	for _, rawURL := range tests {
		t.Run(rawURL, func(t *testing.T) {
			require.Equal(t, rawURL, SanitizeURLForLogging(rawURL))
		})
	}
}

func BenchmarkSanitizeURLForLoggingNoQuery(b *testing.B) {
	const rawURL = "/v1/chat/completions"
	b.ReportAllocs()
	for b.Loop() {
		_ = SanitizeURLForLogging(rawURL)
	}
}

func BenchmarkSanitizeURLForLoggingPublicQuery(b *testing.B) {
	const rawURL = "/v1/chat/completions?stream=true&foo=bar"
	b.ReportAllocs()
	for b.Loop() {
		_ = SanitizeURLForLogging(rawURL)
	}
}

// TestBoltURLBenchmark is temporary instrumentation used to capture the
// baseline and optimized results in the same PR CI environment.
func TestBoltURLBenchmark(t *testing.T) {
	for _, tc := range []struct {
		name string
		fn   func(*testing.B)
	}{
		{name: "no_query", fn: BenchmarkSanitizeURLForLoggingNoQuery},
		{name: "public_query", fn: BenchmarkSanitizeURLForLoggingPublicQuery},
	} {
		result := testing.Benchmark(tc.fn)
		t.Logf("BOLT_BENCH logging/%s: %d ns/op, %d B/op, %d allocs/op", tc.name, result.NsPerOp(), result.AllocedBytesPerOp(), result.AllocsPerOp())
	}
}
