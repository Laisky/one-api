package otel

// Application-log export metrics (proposal
// 20260905_observability-data-tiering.md, Phase 3 / W3.2). The instruments are
// created lazily on first use so the OtelRecorder struct and constructor in
// recorder.go stay untouched.
//
// This is the deployment where the accounting matters most: when the OTLP
// bridge is the only place application logs go, a record it discards leaves no
// trace anywhere else, so the counters have to leave the process through the
// metrics pipeline instead.

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

var (
	logExportOnce               sync.Once
	appLogExportRecordsCount    metric.Int64Counter
	appLogExportQueueRecordsG   metric.Int64Gauge
	appLogExportQueueRecordLimG metric.Int64Gauge
	appLogExportQueueBytesG     metric.Int64Gauge
	appLogExportQueueByteLimitG metric.Int64Gauge
)

// initLogExportInstruments creates the application-log export instruments once.
//
// Parameters: none.
//
// Return values: none; instruments that fail to build stay nil and their
// recording methods become no-ops.
func initLogExportInstruments() {
	logExportOnce.Do(func() {
		meter := otel.Meter("one-api")
		if c, err := meter.Int64Counter(
			"oneapi_app_log_export_records_total",
			metric.WithDescription("Application log records by OTLP export outcome (never content)"),
		); err == nil {
			appLogExportRecordsCount = c
		}
		if g, err := meter.Int64Gauge(
			"oneapi_app_log_export_queue_records",
			metric.WithDescription("Application log records currently resident in the OTLP export pipeline"),
		); err == nil {
			appLogExportQueueRecordsG = g
		}
		if g, err := meter.Int64Gauge(
			"oneapi_app_log_export_queue_record_limit",
			metric.WithDescription("Configured record ceiling of the OTLP application-log export queue"),
		); err == nil {
			appLogExportQueueRecordLimG = g
		}
		if g, err := meter.Int64Gauge(
			"oneapi_app_log_export_queue_bytes",
			metric.WithUnit("By"),
			metric.WithDescription("Estimated bytes currently resident in the OTLP application-log export queue"),
		); err == nil {
			appLogExportQueueBytesG = g
		}
		if g, err := meter.Int64Gauge(
			"oneapi_app_log_export_queue_byte_limit",
			metric.WithUnit("By"),
			metric.WithDescription("Configured byte ceiling of the OTLP application-log export queue"),
		); err == nil {
			appLogExportQueueByteLimitG = g
		}
	})
}

// RecordAppLogExportRecords tallies count application log records under the
// given export outcome.
//
// Parameters:
//   - outcome: a compile-time constant from common/metrics/log_export.go; it
//     becomes an attribute and must never carry a log message or logger name.
//   - count: how many log records the outcome applies to; a non-positive count
//     is ignored.
//
// Return values: none.
func (r *OtelRecorder) RecordAppLogExportRecords(outcome string, count int) {
	if count <= 0 {
		return
	}
	initLogExportInstruments()
	if appLogExportRecordsCount == nil {
		return
	}
	appLogExportRecordsCount.Add(context.Background(), int64(count), metric.WithAttributes(
		strAttr("outcome", outcome),
	))
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
func (r *OtelRecorder) UpdateAppLogExportQueue(records, recordLimit, bytes, byteLimit float64) {
	initLogExportInstruments()
	ctx := context.Background()
	if appLogExportQueueRecordsG != nil {
		appLogExportQueueRecordsG.Record(ctx, int64(records))
	}
	if appLogExportQueueRecordLimG != nil {
		appLogExportQueueRecordLimG.Record(ctx, int64(recordLimit))
	}
	if appLogExportQueueBytesG != nil {
		appLogExportQueueBytesG.Record(ctx, int64(bytes))
	}
	if appLogExportQueueByteLimitG != nil {
		appLogExportQueueByteLimitG.Record(ctx, int64(byteLimit))
	}
}
