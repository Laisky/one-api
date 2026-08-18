package format

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var benchmarkDetectFormatPlainText = []byte(`{
	"model":"gpt-4.1",
	"messages":[
		{"role":"system","content":"You are helpful"},
		{"role":"user","content":"Explain Kubernetes operators"},
		{"role":"assistant","content":"Sure."},
		{"role":"user","content":"Continue with examples"}
	]
}`)

var benchmarkDetectFormatLargeSystem = []byte(`{
	"model":"claude-sonnet-4",
	"system":[{"type":"text","text":"` + strings.Repeat("A", 4096) + `"}],
	"messages":[
		{"role":"user","content":"Explain Kubernetes operators"},
		{"role":"assistant","content":"Sure."},
		{"role":"user","content":"Continue with examples"}
	]
}`)

func TestDetectFormatScalarContentBehavior(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
		want APIFormat
	}{
		{name: "string content", body: `{"messages":[{"role":"user","content":"hello"}]}`, want: Unknown},
		{name: "null content", body: `{"messages":[{"role":"assistant","content":null}]}`, want: Unknown},
		{name: "object content", body: `{"messages":[{"role":"user","content":{"text":"hello"}}]}`, want: Unknown},
		{name: "claude array content", body: `{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"1"}]}]}`, want: ClaudeMessages},
		{name: "large unused fields stay ambiguous", body: string(benchmarkDetectFormatLargeSystem), want: Unknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DetectFormat([]byte(tt.body))
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func BenchmarkDetectFormatPlainText(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_, _ = DetectFormat(benchmarkDetectFormatPlainText)
	}
}

func BenchmarkDetectFormatLargeSystem(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_, _ = DetectFormat(benchmarkDetectFormatLargeSystem)
	}
}

// TestBoltFormatBenchmark is temporary instrumentation used to capture the
// baseline and optimized results in the same PR CI environment.
func TestBoltFormatBenchmark(t *testing.T) {
	for _, tc := range []struct {
		name string
		fn   func(*testing.B)
	}{
		{name: "plain_text", fn: BenchmarkDetectFormatPlainText},
		{name: "large_system", fn: BenchmarkDetectFormatLargeSystem},
	} {
		result := testing.Benchmark(tc.fn)
		t.Logf("BOLT_BENCH format/%s: %d ns/op, %d B/op, %d allocs/op", tc.name, result.NsPerOp(), result.AllocedBytesPerOp(), result.AllocsPerOp())
	}
}
