package prometheus

// Trace-pipeline metrics (proposal 20260905_observability-data-tiering.md,
// Phase 1 / W1.3). They are an ordinary part of the PrometheusRecorder
// implementation of metrics.MetricsRecorder and live in their own file only to
// keep recorder.go small.

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// traceRecordsTotal counts completed request traces by pipeline outcome. The
// outcome label is drawn from the compile-time registry in
// common/metrics/trace_pipeline.go; no trace id, URL, or error message may ever
// reach it.
var traceRecordsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "oneapi_trace_records_total",
	Help: "Total completed request traces by trace-pipeline outcome (never content)",
}, []string{"outcome"})

// traceQueueDepth reports the current occupancy of the asynchronous trace
// writer queue. A depth that tracks capacity means the database cannot keep
// pace with trace ingestion and records are about to be dropped.
var traceQueueDepth = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "oneapi_trace_queue_depth",
	Help: "Completed request traces currently buffered for asynchronous writing",
})

// traceQueueCapacity reports the configured trace writer queue capacity.
var traceQueueCapacity = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "oneapi_trace_queue_capacity",
	Help: "Configured capacity of the asynchronous trace writer queue",
})

// traceActiveRecorders reports how many in-flight requests currently hold a
// trace recorder. The completed-record queue gauge above says nothing about
// this: long-lived streaming requests accumulate on the active side, which is
// where trace memory actually grows at high concurrency.
var traceActiveRecorders = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "oneapi_trace_active_recorders",
	Help: "In-flight requests currently holding a trace recorder",
})

// traceActiveRecordersLimit reports the configured admission limit, or 0 when
// admission is unlimited.
var traceActiveRecordersLimit = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "oneapi_trace_active_recorders_limit",
	Help: "Configured TRACE_MAX_ACTIVE_RECORDERS admission limit (0 means unlimited)",
})

// UpdateTraceActiveRecorders publishes in-flight trace recorder occupancy.
//
// Parameters:
//   - active: number of requests currently holding a recorder.
//   - limit: configured admission limit; 0 means unlimited.
//
// Return values: none.
func (p *PrometheusRecorder) UpdateTraceActiveRecorders(active, limit float64) {
	traceActiveRecorders.Set(active)
	traceActiveRecordersLimit.Set(limit)
}

// RecordTraceRecord records the outcome of count completed request traces.
//
// Parameters:
//   - outcome: a compile-time constant from common/metrics/trace_pipeline.go.
//   - count: how many trace records the outcome applies to.
//
// Return values: none.
func (p *PrometheusRecorder) RecordTraceRecord(outcome string, count int) {
	if count <= 0 {
		return
	}
	traceRecordsTotal.WithLabelValues(labelValues(outcome)...).Add(float64(count))
}

// UpdateTraceQueueDepth publishes the current trace writer queue occupancy.
//
// Parameters:
//   - depth: number of completed traces currently buffered.
//   - capacity: configured queue capacity.
//
// Return values: none.
func (p *PrometheusRecorder) UpdateTraceQueueDepth(depth, capacity float64) {
	traceQueueDepth.Set(depth)
	traceQueueCapacity.Set(capacity)
}
