package prometheus

// Operational request and retention metrics (proposal
// 20260905_observability-data-tiering.md, Phase 3 / W3.3). They are an ordinary
// part of the PrometheusRecorder implementation and live in their own file only
// to keep recorder.go small.
//
// These series are SAMPLED-INDEPENDENT by construction: their call sites record
// before the trace sampling decision, so TRACE_SAMPLE_RATE cannot turn the
// operational view into a 5% view. They are best-effort telemetry and never
// billing truth; quota and usage remain the authoritative financial record.

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// requestDurationBucketsMs are the total-request-lifetime buckets, in
// milliseconds.
//
// The range is deliberately wider than relayRequestDuration's: this histogram
// covers every request, not just relays. The low end (1ms-50ms) resolves cheap
// control-plane and cache-hit responses that would otherwise all fall into one
// bucket; the middle mirrors the relay buckets so the two can be read together;
// and the 120s/300s tail covers long streaming completions, which routinely run
// for minutes and must stay distinguishable from a request that hung until its
// deadline. Sixteen buckets across at most seven bounded outcome values is a
// worst case of 112 series, which is well inside the cardinality budget.
var requestDurationBucketsMs = []float64{
	1, 5, 10, 25, 50, 100, 250, 500,
	1000, 2500, 5000, 10000, 30000, 60000, 120000, 300000,
}

// timeToFirstTokenBucketsMs are the time-to-first-token buckets, in
// milliseconds.
//
// TTFT measures prefill only and is an order of magnitude smaller than the
// total lifetime, so reusing the duration buckets above would pile almost every
// observation into two of them. The resolution is concentrated between 100ms
// and 3s, where user-visible responsiveness and the usual alert thresholds
// live, with a 60s ceiling for a stalled upstream that eventually answers.
var timeToFirstTokenBucketsMs = []float64{
	5, 10, 25, 50, 100, 200, 400, 800, 1500, 3000, 6000, 12000, 30000, 60000,
}

// retentionSweepDurationBucketsMs are the retention-sweep duration buckets, in
// milliseconds.
//
// A sweep with nothing eligible returns in milliseconds, while a first sweep
// over a long-unpruned table runs for tens of minutes; the buckets therefore
// span five orders of magnitude, matching uuidBackfillCycleDuration's shape.
// Section 8.3 judges retention on whether a sweep keeps pace with newly
// eligible rows, so the tail past 15 minutes is the part that matters.
var retentionSweepDurationBucketsMs = []float64{
	10, 50, 100, 500, 1000, 5000, 15000, 30000, 60000, 300000, 900000, 1800000, 3600000,
}

// requestOutcomesTotal counts finished requests by operational outcome. The
// outcome label is drawn from the compile-time registry in
// common/metrics/operational.go; no path, model, user id, channel id, token, or
// error message may ever reach it.
var requestOutcomesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "oneapi_request_outcomes_total",
	Help: "Finished requests by operational outcome, independent of trace sampling (never content)",
}, []string{"outcome"})

// requestDurationMs observes total request lifetime in milliseconds, split by
// the same bounded outcome vocabulary so a latency regression can be attributed
// to successes rather than being hidden by a burst of fast client errors.
var requestDurationMs = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "oneapi_request_duration_ms",
	Help:    "Total request lifetime in milliseconds by operational outcome",
	Buckets: requestDurationBucketsMs,
}, []string{"outcome"})

// requestTimeToFirstTokenMs observes the delay before a request produced its
// first client byte.
//
// A streaming relay's total lifetime is dominated by the tail of the response,
// so requestDurationMs alone cannot answer "how quickly did the caller start
// seeing output" -- the question an operator actually asks during an incident.
var requestTimeToFirstTokenMs = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "oneapi_request_time_to_first_token_ms",
	Help:    "Milliseconds from request receipt to first client byte, by operational outcome",
	Buckets: timeToFirstTokenBucketsMs,
}, []string{"outcome"})

// retentionSweepsTotal counts finished retention sweeps. Both labels are
// bounded: target comes from the closed set of retention targets and result
// from the compile-time registry in common/metrics/operational.go.
var retentionSweepsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "oneapi_retention_sweeps_total",
	Help: "Finished retention sweeps by target and result",
}, []string{"target", "result"})

// retentionSweepRowsTotal counts the rows or files those sweeps removed.
// Sweeps alone cannot answer whether retention keeps pace with ingestion; rows
// removed per second is the throughput section 8.3 makes part of capacity
// acceptance.
var retentionSweepRowsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "oneapi_retention_sweep_rows_total",
	Help: "Rows or files removed by retention sweeps, by target and result",
}, []string{"target", "result"})

// retentionSweepDurationMs observes how long each retention sweep took.
var retentionSweepDurationMs = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "oneapi_retention_sweep_duration_ms",
	Help:    "Retention sweep duration in milliseconds by target and result",
	Buckets: retentionSweepDurationBucketsMs,
}, []string{"target", "result"})

// RecordRequestOutcome tallies one finished request and its total lifetime.
//
// Parameters:
//   - outcome: a compile-time constant from common/metrics/operational.go.
//   - durationMs: the request's total lifetime in milliseconds; a negative
//     value is ignored so a clock adjustment cannot corrupt the histogram sum.
//
// Return values: none.
func (p *PrometheusRecorder) RecordRequestOutcome(outcome string, durationMs float64) {
	labels := labelValues(outcome)
	requestOutcomesTotal.WithLabelValues(labels...).Inc()
	if durationMs >= 0 {
		requestDurationMs.WithLabelValues(labels...).Observe(durationMs)
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
func (p *PrometheusRecorder) RecordTimeToFirstToken(outcome string, ttftMs float64) {
	if ttftMs < 0 {
		return
	}
	requestTimeToFirstTokenMs.WithLabelValues(labelValues(outcome)...).Observe(ttftMs)
}

// RecordRetentionSweep tallies one finished retention sweep.
//
// Parameters:
//   - target: the swept table or file set, from the closed set of retention
//     targets. It is sanitized like every other label value because a
//     WithLabelValues panic here would take down the sweeper goroutine.
//   - result: a compile-time constant from common/metrics/operational.go.
//   - rows: how many rows or files the sweep removed; a negative value is
//     ignored because a counter cannot decrease.
//   - durationMs: how long the sweep took, in milliseconds; negative is
//     ignored.
//
// Return values: none.
func (p *PrometheusRecorder) RecordRetentionSweep(target, result string, rows float64, durationMs float64) {
	labels := labelValues(target, result)
	retentionSweepsTotal.WithLabelValues(labels...).Inc()
	if rows > 0 {
		retentionSweepRowsTotal.WithLabelValues(labels...).Add(rows)
	}
	if durationMs >= 0 {
		retentionSweepDurationMs.WithLabelValues(labels...).Observe(durationMs)
	}
}
