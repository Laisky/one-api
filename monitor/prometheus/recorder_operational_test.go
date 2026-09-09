package prometheus

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

// histogramCount reads the observation count of a histogram child.
//
// Parameters:
//   - t: the running test.
//   - o: a histogram child obtained from a HistogramVec.
//
// Return values:
//   - uint64: how many observations the child has recorded so far.
func histogramCount(t *testing.T, o prometheus.Observer) uint64 {
	t.Helper()
	m, ok := o.(prometheus.Metric)
	require.True(t, ok, "histogram child must expose prometheus.Metric")
	var pb dto.Metric
	require.NoError(t, m.Write(&pb))
	return pb.GetHistogram().GetSampleCount()
}

// TestRecordRequestOutcomeCountsAndObserves proves the outcome counter and the
// duration histogram move together. The counter is the operational view's
// denominator, so a duration observation that did not also increment it would
// silently understate the request rate.
func TestRecordRequestOutcomeCountsAndObserves(t *testing.T) {
	rec := &PrometheusRecorder{}
	const outcome = "success"

	beforeCount := counterValue(t, requestOutcomesTotal.WithLabelValues(outcome))
	beforeObs := histogramCount(t, requestDurationMs.WithLabelValues(outcome))

	rec.RecordRequestOutcome(outcome, 42.5)

	require.Equal(t, beforeCount+1, counterValue(t, requestOutcomesTotal.WithLabelValues(outcome)))
	require.Equal(t, beforeObs+1, histogramCount(t, requestDurationMs.WithLabelValues(outcome)))
}

// TestRecordRequestOutcomeIgnoresNegativeDuration pins that a clock adjustment
// cannot corrupt the histogram sum while the request itself still counts: a
// negative sample would make the average latency meaningless forever, because
// a histogram's _sum is cumulative and cannot be repaired.
func TestRecordRequestOutcomeIgnoresNegativeDuration(t *testing.T) {
	rec := &PrometheusRecorder{}
	const outcome = "timeout"

	beforeCount := counterValue(t, requestOutcomesTotal.WithLabelValues(outcome))
	beforeObs := histogramCount(t, requestDurationMs.WithLabelValues(outcome))

	rec.RecordRequestOutcome(outcome, -1)

	require.Equal(t, beforeCount+1, counterValue(t, requestOutcomesTotal.WithLabelValues(outcome)),
		"the request must still be counted")
	require.Equal(t, beforeObs, histogramCount(t, requestDurationMs.WithLabelValues(outcome)),
		"a negative duration must not be observed")
}

// TestRecordTimeToFirstTokenIgnoresNegative pins the same guard on TTFT.
func TestRecordTimeToFirstTokenIgnoresNegative(t *testing.T) {
	rec := &PrometheusRecorder{}
	const outcome = "upstream_error"

	before := histogramCount(t, requestTimeToFirstTokenMs.WithLabelValues(outcome))
	rec.RecordTimeToFirstToken(outcome, -0.5)
	require.Equal(t, before, histogramCount(t, requestTimeToFirstTokenMs.WithLabelValues(outcome)))

	rec.RecordTimeToFirstToken(outcome, 12)
	require.Equal(t, before+1, histogramCount(t, requestTimeToFirstTokenMs.WithLabelValues(outcome)))
}

// TestRecordRetentionSweepCountsSweepEvenWithoutRows proves a sweep that
// deleted nothing is still counted. Section 8.3 judges retention on throughput,
// and "the sweeper ran and found nothing eligible" must stay distinguishable
// from "the sweeper never ran", which a rows-only counter cannot express.
func TestRecordRetentionSweepCountsSweepEvenWithoutRows(t *testing.T) {
	rec := &PrometheusRecorder{}
	const target, result = "logs", "completed"

	beforeSweeps := counterValue(t, retentionSweepsTotal.WithLabelValues(target, result))
	beforeRows := counterValue(t, retentionSweepRowsTotal.WithLabelValues(target, result))
	beforeObs := histogramCount(t, retentionSweepDurationMs.WithLabelValues(target, result))

	rec.RecordRetentionSweep(target, result, 0, 7)

	require.Equal(t, beforeSweeps+1, counterValue(t, retentionSweepsTotal.WithLabelValues(target, result)))
	require.Equal(t, beforeRows, counterValue(t, retentionSweepRowsTotal.WithLabelValues(target, result)),
		"an empty sweep must not add rows")
	require.Equal(t, beforeObs+1, histogramCount(t, retentionSweepDurationMs.WithLabelValues(target, result)))

	rec.RecordRetentionSweep(target, result, 250, 9)
	require.Equal(t, beforeRows+250, counterValue(t, retentionSweepRowsTotal.WithLabelValues(target, result)))
}

// TestRecordRetentionSweepSanitizesTarget verifies the target label goes
// through labelValues. The retention targets are a closed set today, but a
// WithLabelValues panic inside the sweeper goroutine would kill retention
// entirely, so the sanitization must not depend on that staying true.
func TestRecordRetentionSweepSanitizesTarget(t *testing.T) {
	rec := &PrometheusRecorder{}
	before := counterValue(t, retentionSweepsTotal.WithLabelValues("logs-�", "failed"))
	rec.RecordRetentionSweep("logs-\xc0", "failed", 0, 1)
	require.Equal(t, before+1, counterValue(t, retentionSweepsTotal.WithLabelValues("logs-�", "failed")))
}

// TestRecordAppLogExportRecordsIgnoresEmptyBatch pins that an empty batch does
// not create a series. A zero-count call would otherwise materialize an outcome
// series that never grows, which reads on a dashboard as a real, permanently
// stalled outcome.
func TestRecordAppLogExportRecordsIgnoresEmptyBatch(t *testing.T) {
	rec := &PrometheusRecorder{}
	const outcome = "exported"

	before := counterValue(t, appLogExportRecordsTotal.WithLabelValues(outcome))
	rec.RecordAppLogExportRecords(outcome, 0)
	rec.RecordAppLogExportRecords(outcome, -3)
	require.Equal(t, before, counterValue(t, appLogExportRecordsTotal.WithLabelValues(outcome)))

	rec.RecordAppLogExportRecords(outcome, 4)
	require.Equal(t, before+4, counterValue(t, appLogExportRecordsTotal.WithLabelValues(outcome)))
}

// TestUpdateAppLogExportQueuePublishesBoundsWithOccupancy proves the limits are
// published alongside the depth. Without the bound series a dashboard cannot
// compute saturation without hard-coding configuration, which is exactly how a
// re-tuned queue silently stops alerting.
func TestUpdateAppLogExportQueuePublishesBoundsWithOccupancy(t *testing.T) {
	rec := &PrometheusRecorder{}
	rec.UpdateAppLogExportQueue(12, 2048, 3400, 4194304)

	require.Equal(t, 12.0, gaugeValue(t, appLogExportQueueRecords))
	require.Equal(t, 2048.0, gaugeValue(t, appLogExportQueueRecordLimit))
	require.Equal(t, 3400.0, gaugeValue(t, appLogExportQueueBytes))
	require.Equal(t, 4194304.0, gaugeValue(t, appLogExportQueueByteLimit))
}

// gaugeValue reads the current value of a gauge.
//
// Parameters:
//   - t: the running test.
//   - g: the gauge to read.
//
// Return values:
//   - float64: the gauge's current value.
func gaugeValue(t *testing.T, g prometheus.Gauge) float64 {
	t.Helper()
	var pb dto.Metric
	require.NoError(t, g.Write(&pb))
	return pb.GetGauge().GetValue()
}
