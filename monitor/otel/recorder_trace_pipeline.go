package otel

// Trace-pipeline metrics (proposal 20260905_observability-data-tiering.md,
// Phase 1 / W1.3). The instruments are created lazily on first use so the
// OtelRecorder struct and constructor in recorder.go stay untouched.

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

var (
	tracePipelineOnce      sync.Once
	traceRecordsCounter    metric.Int64Counter
	traceQueueDepthGauge   metric.Int64Gauge
	traceQueueCapacityGaug metric.Int64Gauge
)

// initTracePipelineInstruments creates the trace-pipeline instruments once.
//
// Parameters: none.
//
// Return values: none; instruments that fail to build stay nil and their
// recording methods become no-ops.
func initTracePipelineInstruments() {
	tracePipelineOnce.Do(func() {
		meter := otel.Meter("one-api")
		if c, err := meter.Int64Counter(
			"oneapi_trace_records_total",
			metric.WithDescription("Total completed request traces by trace-pipeline outcome (never content)"),
		); err == nil {
			traceRecordsCounter = c
		}
		if g, err := meter.Int64Gauge(
			"oneapi_trace_queue_depth",
			metric.WithDescription("Completed request traces currently buffered for asynchronous writing"),
		); err == nil {
			traceQueueDepthGauge = g
		}
		if g, err := meter.Int64Gauge(
			"oneapi_trace_queue_capacity",
			metric.WithDescription("Configured capacity of the asynchronous trace writer queue"),
		); err == nil {
			traceQueueCapacityGaug = g
		}
	})
}

// RecordTraceRecord records the outcome of count completed request traces.
//
// Parameters:
//   - outcome: a compile-time constant from common/metrics/trace_pipeline.go;
//     it becomes an attribute and must never carry a trace id or error message.
//   - count: how many trace records the outcome applies to.
//
// Return values: none.
func (r *OtelRecorder) RecordTraceRecord(outcome string, count int) {
	if count <= 0 {
		return
	}
	initTracePipelineInstruments()
	if traceRecordsCounter == nil {
		return
	}
	traceRecordsCounter.Add(context.Background(), int64(count), metric.WithAttributes(
		strAttr("outcome", outcome),
	))
}

// UpdateTraceQueueDepth publishes the current trace writer queue occupancy.
//
// Parameters:
//   - depth: number of completed traces currently buffered.
//   - capacity: configured queue capacity.
//
// Return values: none.
func (r *OtelRecorder) UpdateTraceQueueDepth(depth, capacity float64) {
	initTracePipelineInstruments()
	if traceQueueDepthGauge != nil {
		traceQueueDepthGauge.Record(context.Background(), int64(depth))
	}
	if traceQueueCapacityGaug != nil {
		traceQueueCapacityGaug.Record(context.Background(), int64(capacity))
	}
}
