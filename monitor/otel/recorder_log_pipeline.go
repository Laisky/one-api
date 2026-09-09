package otel

// Application-log containment metrics (proposal
// 20260905_observability-data-tiering.md, Phase 0 / W0.4). The instruments are
// created lazily on first use so the OtelRecorder struct and constructor in
// recorder.go stay untouched.
//
// These matter most in exactly the deployment that has no Prometheus scrape:
// when the disk guard starts discarding application log output, the log itself
// can no longer be trusted to report the loss, so the counters have to leave
// the process by another route.

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

var (
	logPipelineOnce          sync.Once
	logSuppressedLinesCount  metric.Int64Counter
	logSuppressedBytesCount  metric.Int64Counter
	logDiskPressureGaugeInst metric.Int64Gauge
)

// initLogPipelineInstruments creates the log-containment instruments once.
//
// Parameters: none.
//
// Return values: none; instruments that fail to build stay nil and their
// recording methods become no-ops.
func initLogPipelineInstruments() {
	logPipelineOnce.Do(func() {
		meter := otel.Meter("one-api")
		if c, err := meter.Int64Counter(
			"oneapi_log_suppressed_lines_total",
			metric.WithDescription("Application log lines discarded by the disk guard, by reason (never content)"),
		); err == nil {
			logSuppressedLinesCount = c
		}
		if c, err := meter.Int64Counter(
			"oneapi_log_suppressed_bytes_total",
			metric.WithDescription("Bytes of application log output discarded by the disk guard, by reason"),
		); err == nil {
			logSuppressedBytesCount = c
		}
		if g, err := meter.Int64Gauge(
			"oneapi_log_disk_pressure_active",
			metric.WithDescription("1 when application logging is operating under the disk-pressure emergency policy"),
		); err == nil {
			logDiskPressureGaugeInst = g
		}
	})
}

// RecordLogSuppression tallies suppressed application log lines and bytes.
//
// Parameters:
//   - reason: a compile-time constant from common/metrics/log_pipeline.go; it
//     becomes an attribute and must never carry a message or path.
//   - lines: how many log lines were discarded.
//   - bytes: how many bytes those lines would have written.
//
// Return values: none.
func (r *OtelRecorder) RecordLogSuppression(reason string, lines int, bytes int64) {
	if lines <= 0 {
		return
	}
	initLogPipelineInstruments()
	attrs := metric.WithAttributes(strAttr("reason", reason))
	if logSuppressedLinesCount != nil {
		logSuppressedLinesCount.Add(context.Background(), int64(lines), attrs)
	}
	if bytes > 0 && logSuppressedBytesCount != nil {
		logSuppressedBytesCount.Add(context.Background(), bytes, attrs)
	}
}

// UpdateLogDiskPressure publishes whether the emergency logging policy is
// engaged.
//
// Parameters:
//   - active: 1 when engaged, 0 otherwise.
//
// Return values: none.
func (r *OtelRecorder) UpdateLogDiskPressure(active float64) {
	initLogPipelineInstruments()
	if logDiskPressureGaugeInst != nil {
		logDiskPressureGaugeInst.Record(context.Background(), int64(active))
	}
}
