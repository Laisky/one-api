package prometheus

import "github.com/Laisky/one-api/common/metrics"

// Compile-time proof that PrometheusRecorder satisfies the base recorder and
// every optional extension interface it is expected to serve.
//
// The helper functions in common/metrics reach a recorder through a type
// assertion, so an extension method that is renamed, given a wrong signature,
// or simply never added does not fail the build -- it silently turns every
// sample routed through that helper into a no-op, and the affected metric reads
// zero forever while the subsystem works correctly. That failure is invisible
// in tests and in production alike, so it has to be caught at compile time.
var (
	_ metrics.MetricsRecorder       = (*PrometheusRecorder)(nil)
	_ metrics.TracePipelineRecorder = (*PrometheusRecorder)(nil)
	_ metrics.TraceActiveRecorder   = (*PrometheusRecorder)(nil)
	_ metrics.LogPipelineRecorder   = (*PrometheusRecorder)(nil)
)
