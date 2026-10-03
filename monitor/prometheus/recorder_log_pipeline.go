package prometheus

// Application-log containment metrics (proposal
// 20260905_observability-data-tiering.md, Phase 0 / W0.4). They are an ordinary
// part of the PrometheusRecorder implementation and live in their own file only
// to keep recorder.go small.

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// logSuppressedLinesTotal counts application log lines the disk guard
// discarded. The reason label is drawn from the compile-time registry in
// common/metrics/log_pipeline.go; no message, path, or error text may reach it.
var logSuppressedLinesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "oneapi_log_suppressed_lines_total",
	Help: "Application log lines discarded by the disk guard, by reason (never content)",
}, []string{"reason"})

// logSuppressedBytesTotal counts the bytes those suppressed lines would have
// written. Lines alone understate the problem when the storm is a few enormous
// stack traces rather than many small entries.
var logSuppressedBytesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "oneapi_log_suppressed_bytes_total",
	Help: "Bytes of application log output discarded by the disk guard, by reason",
}, []string{"reason"})

// logDiskPressureActive reports whether the emergency logging policy is
// engaged. A sustained 1 means the process is deliberately losing log output to
// stay alive, which is an alertable degraded state rather than a healthy one.
var logDiskPressureActive = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "oneapi_log_disk_pressure_active",
	Help: "1 when application logging is operating under the disk-pressure emergency policy",
})

// RecordLogSuppression tallies suppressed application log lines and bytes.
//
// Parameters:
//   - reason: a compile-time constant from common/metrics/log_pipeline.go.
//   - lines: how many log lines were discarded.
//   - bytes: how many bytes those lines would have written.
//
// Return values: none.
func (p *PrometheusRecorder) RecordLogSuppression(reason string, lines int, bytes int64) {
	if lines <= 0 {
		return
	}
	labels := labelValues(reason)
	logSuppressedLinesTotal.WithLabelValues(labels...).Add(float64(lines))
	if bytes > 0 {
		logSuppressedBytesTotal.WithLabelValues(labels...).Add(float64(bytes))
	}
}

// UpdateLogDiskPressure publishes whether the emergency logging policy is
// engaged.
//
// Parameters:
//   - active: 1 when engaged, 0 otherwise.
//
// Return values: none.
func (p *PrometheusRecorder) UpdateLogDiskPressure(active float64) {
	logDiskPressureActive.Set(active)
}
