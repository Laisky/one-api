package prometheus

// Application-log export metrics (proposal
// 20260905_observability-data-tiering.md, Phase 3 / W3.2). They are an ordinary
// part of the PrometheusRecorder implementation and live in their own file only
// to keep recorder.go small.
//
// The OTLP application-log bridge is best-effort: it discards records rather
// than blocking a request goroutine. These series are the only place that loss
// becomes visible, because the discarded records are precisely the ones that
// never reach the log stream an operator would otherwise read.

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// appLogExportRecordsTotal counts application log records by export outcome.
// The outcome label is drawn from the compile-time registry in
// common/metrics/log_export.go; no log message, logger name, request id, or
// error text may ever reach it.
var appLogExportRecordsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "oneapi_app_log_export_records_total",
	Help: "Application log records by OTLP export outcome (never content)",
}, []string{"outcome"})

// appLogExportQueueRecords reports how many log records currently sit in the
// export pipeline. A depth that tracks its limit means the collector cannot
// keep pace and records are about to be discarded.
var appLogExportQueueRecords = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "oneapi_app_log_export_queue_records",
	Help: "Application log records currently resident in the OTLP export pipeline",
})

// appLogExportQueueRecordLimit reports the configured record ceiling the depth
// above is measured against. Publishing the bound as its own series keeps the
// saturation ratio computable without hard-coding configuration into a
// dashboard query.
var appLogExportQueueRecordLimit = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "oneapi_app_log_export_queue_record_limit",
	Help: "Configured record ceiling of the OTLP application-log export queue",
})

// appLogExportQueueBytes reports the estimated bytes currently resident in the
// export pipeline. Records alone understate the pressure when the storm is a
// few enormous stack traces rather than many small entries.
var appLogExportQueueBytes = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "oneapi_app_log_export_queue_bytes",
	Help: "Estimated bytes currently resident in the OTLP application-log export queue",
})

// appLogExportQueueByteLimit reports the configured byte ceiling.
var appLogExportQueueByteLimit = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "oneapi_app_log_export_queue_byte_limit",
	Help: "Configured byte ceiling of the OTLP application-log export queue",
})

// RecordAppLogExportRecords tallies count application log records under the
// given export outcome.
//
// Parameters:
//   - outcome: a compile-time constant from common/metrics/log_export.go.
//   - count: how many log records the outcome applies to; a non-positive count
//     is ignored so an empty batch cannot create a series.
//
// Return values: none.
func (p *PrometheusRecorder) RecordAppLogExportRecords(outcome string, count int) {
	if count <= 0 {
		return
	}
	appLogExportRecordsTotal.WithLabelValues(labelValues(outcome)...).Add(float64(count))
}

// UpdateAppLogExportQueue publishes the export queue's occupancy and the bounds
// it is measured against.
//
// Parameters:
//   - records: log records currently resident in the export pipeline.
//   - recordLimit: the configured record ceiling.
//   - bytes: estimated bytes currently resident in the export pipeline.
//   - byteLimit: the configured byte ceiling.
//
// Return values: none.
func (p *PrometheusRecorder) UpdateAppLogExportQueue(records, recordLimit, bytes, byteLimit float64) {
	appLogExportQueueRecords.Set(records)
	appLogExportQueueRecordLimit.Set(recordLimit)
	appLogExportQueueBytes.Set(bytes)
	appLogExportQueueByteLimit.Set(byteLimit)
}
