package otel

// Operational request and retention metrics (proposal
// 20260905_observability-data-tiering.md, Phase 3 / W3.3). The instruments are
// created lazily on first use so the OtelRecorder struct and constructor in
// recorder.go stay untouched.
//
// These series are SAMPLED-INDEPENDENT by construction: their call sites record
// before the trace sampling decision, so TRACE_SAMPLE_RATE cannot turn the
// operational view into a 5% view. They are best-effort telemetry and never
// billing truth; quota and usage remain the authoritative financial record.

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// requestDurationBucketsMs are the total-request-lifetime buckets, in
// milliseconds.
//
// The boundaries are the same ones monitor/prometheus/recorder_operational.go
// uses, so the two exporters remain comparable in a mixed deployment: the low
// end (1ms-50ms) resolves cheap control-plane and cache-hit responses, the
// middle mirrors the relay latency buckets, and the 120s/300s tail keeps a long
// streaming completion distinguishable from a request that hung until its
// deadline. The OTel SDK's default boundaries top out at 10s, which would
// collapse every streaming request into one overflow bucket.
var requestDurationBucketsMs = []float64{
	1, 5, 10, 25, 50, 100, 250, 500,
	1000, 2500, 5000, 10000, 30000, 60000, 120000, 300000,
}

// timeToFirstTokenBucketsMs are the time-to-first-token buckets, in
// milliseconds.
//
// TTFT measures prefill only and is an order of magnitude smaller than the
// total lifetime, so reusing the duration boundaries above would pile almost
// every observation into two buckets. Resolution is concentrated between 100ms
// and 3s, where user-visible responsiveness and the usual alert thresholds
// live, with a 60s ceiling for a stalled upstream that eventually answers.
var timeToFirstTokenBucketsMs = []float64{
	5, 10, 25, 50, 100, 200, 400, 800, 1500, 3000, 6000, 12000, 30000, 60000,
}

// retentionSweepDurationBucketsMs are the retention-sweep duration buckets, in
// milliseconds.
//
// A sweep with nothing eligible returns in milliseconds, while a first sweep
// over a long-unpruned table runs for tens of minutes; the boundaries therefore
// span five orders of magnitude. Section 8.3 judges retention on whether a
// sweep keeps pace with newly eligible rows, so the tail past 15 minutes is the
// part that matters.
var retentionSweepDurationBucketsMs = []float64{
	10, 50, 100, 500, 1000, 5000, 15000, 30000, 60000, 300000, 900000, 1800000, 3600000,
}

var (
	operationalOnce            sync.Once
	requestOutcomesCounter     metric.Int64Counter
	requestDurationHist        metric.Float64Histogram
	requestTimeToFirstTokenHst metric.Float64Histogram
	retentionSweepsCounter     metric.Int64Counter
	retentionSweepRowsCounter  metric.Float64Counter
	retentionSweepDurationHist metric.Float64Histogram
)

// initOperationalInstruments creates the operational request and retention
// instruments once.
//
// Parameters: none.
//
// Return values: none; instruments that fail to build stay nil and their
// recording methods become no-ops.
func initOperationalInstruments() {
	operationalOnce.Do(func() {
		meter := otel.Meter("one-api")
		if c, err := meter.Int64Counter(
			"oneapi_request_outcomes_total",
			metric.WithDescription("Finished requests by operational outcome, independent of trace sampling (never content)"),
		); err == nil {
			requestOutcomesCounter = c
		}
		if h, err := meter.Float64Histogram(
			"oneapi_request_duration_ms",
			metric.WithUnit("ms"),
			metric.WithExplicitBucketBoundaries(requestDurationBucketsMs...),
			metric.WithDescription("Total request lifetime in milliseconds by operational outcome"),
		); err == nil {
			requestDurationHist = h
		}
		if h, err := meter.Float64Histogram(
			"oneapi_request_time_to_first_token_ms",
			metric.WithUnit("ms"),
			metric.WithExplicitBucketBoundaries(timeToFirstTokenBucketsMs...),
			metric.WithDescription("Milliseconds from request receipt to first client byte, by operational outcome"),
		); err == nil {
			requestTimeToFirstTokenHst = h
		}
		if c, err := meter.Int64Counter(
			"oneapi_retention_sweeps_total",
			metric.WithDescription("Finished retention sweeps by target and result"),
		); err == nil {
			retentionSweepsCounter = c
		}
		if c, err := meter.Float64Counter(
			"oneapi_retention_sweep_rows_total",
			metric.WithDescription("Rows or files removed by retention sweeps, by target and result"),
		); err == nil {
			retentionSweepRowsCounter = c
		}
		if h, err := meter.Float64Histogram(
			"oneapi_retention_sweep_duration_ms",
			metric.WithUnit("ms"),
			metric.WithExplicitBucketBoundaries(retentionSweepDurationBucketsMs...),
			metric.WithDescription("Retention sweep duration in milliseconds by target and result"),
		); err == nil {
			retentionSweepDurationHist = h
		}
	})
}

// RecordRequestOutcome tallies one finished request and its total lifetime.
//
// Parameters:
//   - outcome: a compile-time constant from common/metrics/operational.go; it
//     becomes an attribute and must never carry a path, model, or error string.
//   - durationMs: the request's total lifetime in milliseconds; a negative
//     value is ignored so a clock adjustment cannot corrupt the histogram sum.
//
// Return values: none.
func (r *OtelRecorder) RecordRequestOutcome(outcome string, durationMs float64) {
	initOperationalInstruments()
	ctx := context.Background()
	attrs := metric.WithAttributes(strAttr("outcome", outcome))
	if requestOutcomesCounter != nil {
		requestOutcomesCounter.Add(ctx, 1, attrs)
	}
	if durationMs >= 0 && requestDurationHist != nil {
		requestDurationHist.Record(ctx, durationMs, attrs)
	}
}

// RecordTimeToFirstToken observes the delay before a request produced its first
// client byte.
//
// Parameters:
//   - outcome: a compile-time constant from common/metrics/operational.go.
//   - ttftMs: milliseconds from request receipt to first client byte; a
//     negative value is ignored.
//
// Return values: none.
func (r *OtelRecorder) RecordTimeToFirstToken(outcome string, ttftMs float64) {
	if ttftMs < 0 {
		return
	}
	initOperationalInstruments()
	if requestTimeToFirstTokenHst == nil {
		return
	}
	requestTimeToFirstTokenHst.Record(context.Background(), ttftMs, metric.WithAttributes(
		strAttr("outcome", outcome),
	))
}

// RecordRetentionSweep tallies one finished retention sweep.
//
// Parameters:
//   - target: the swept table or file set, from the closed set of retention
//     targets. It is sanitized like every other string attribute because an
//     invalid UTF-8 attribute fails the whole OTLP export batch.
//   - result: a compile-time constant from common/metrics/operational.go.
//   - rows: how many rows or files the sweep removed; a non-positive value is
//     not added because a counter cannot decrease.
//   - durationMs: how long the sweep took, in milliseconds; negative is
//     ignored.
//
// Return values: none.
func (r *OtelRecorder) RecordRetentionSweep(target, result string, rows float64, durationMs float64) {
	initOperationalInstruments()
	ctx := context.Background()
	attrs := metric.WithAttributes(
		strAttr("target", target),
		strAttr("result", result),
	)
	if retentionSweepsCounter != nil {
		retentionSweepsCounter.Add(ctx, 1, attrs)
	}
	if rows > 0 && retentionSweepRowsCounter != nil {
		retentionSweepRowsCounter.Add(ctx, rows, attrs)
	}
	if durationMs >= 0 && retentionSweepDurationHist != nil {
		retentionSweepDurationHist.Record(ctx, durationMs, attrs)
	}
}
