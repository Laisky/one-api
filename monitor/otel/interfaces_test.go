package otel

import "github.com/Laisky/one-api/common/metrics"

// Compile-time proof that OtelRecorder satisfies the base recorder and every
// optional extension interface it is expected to serve.
//
// See monitor/prometheus/interfaces_test.go for why these assertions exist: a
// missing optional-extension method is silently a no-op at runtime, and an
// OTLP-only deployment is precisely the one with no second metrics path to
// reveal the gap.
var (
	_ metrics.MetricsRecorder       = (*OtelRecorder)(nil)
	_ metrics.TracePipelineRecorder = (*OtelRecorder)(nil)
	_ metrics.TraceActiveRecorder   = (*OtelRecorder)(nil)
	_ metrics.LogPipelineRecorder   = (*OtelRecorder)(nil)
)
