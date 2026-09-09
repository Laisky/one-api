package model

// Retention throughput metric tests (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 3 / W3.3 and
// section 8.3).
//
// The distinction these tests exist to protect is completed vs canceled: a
// shutdown that keeps cutting sweeps short must not look like a gateway whose
// retention is keeping up.

import (
	"context"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/metrics"
)

// retentionSweepSample is one recorded sweep.
type retentionSweepSample struct {
	target     string
	result     string
	rows       float64
	durationMs float64
}

// retentionSweepSpy captures retention sweep metrics.
//
// It is not safe for concurrent use: every test here runs its sweep on the test
// goroutine.
type retentionSweepSpy struct {
	*metrics.NoOpRecorder

	sweeps []retentionSweepSample
}

// RecordRetentionSweep implements metrics.RetentionRecorder.
//
// Parameters:
//   - target: the swept table.
//   - result: one of the metrics.RetentionResult* constants.
//   - rows: rows removed.
//   - durationMs: how long the sweep took.
//
// Return values: none.
func (s *retentionSweepSpy) RecordRetentionSweep(target, result string, rows, durationMs float64) {
	s.sweeps = append(s.sweeps, retentionSweepSample{
		target: target, result: result, rows: rows, durationMs: durationMs,
	})
}

// only returns the single recorded sweep, failing when there is not exactly one.
//
// Parameters:
//   - t: the test, used to fail fast.
//
// Return values:
//   - retentionSweepSample: the recorded sweep.
func (s *retentionSweepSpy) only(t *testing.T) retentionSweepSample {
	t.Helper()
	require.Len(t, s.sweeps, 1, "one sweep must produce exactly one sample")
	return s.sweeps[0]
}

// installRetentionSweepSpy swaps in a recorder capturing retention sweeps.
//
// Parameters:
//   - t: the test, used to register cleanup.
//
// Return values:
//   - *retentionSweepSpy: the installed recorder.
func installRetentionSweepSpy(t *testing.T) *retentionSweepSpy {
	t.Helper()
	spy := &retentionSweepSpy{NoOpRecorder: &metrics.NoOpRecorder{}}
	prev := metrics.Recorder()
	metrics.SetRecorder(spy)
	t.Cleanup(func() { metrics.SetRecorder(prev) })
	return spy
}

// TestRetentionSweepResultClassification pins the result vocabulary, which is
// the whole point of section 8.3's catch-up accounting: a sweep stopped by
// shutdown left work behind and must never be reported as completed.
func TestRetentionSweepResultClassification(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"drained", nil, metrics.RetentionResultCompleted},
		{"shutdown", errors.Wrap(context.Canceled, "sweep expired traces"),
			metrics.RetentionResultCanceled},
		{"deadline", errors.Wrap(context.DeadlineExceeded, "sweep expired traces"),
			metrics.RetentionResultCanceled},
		{"database failure", errors.New("locate expired rows in traces"),
			metrics.RetentionResultFailed},
		{"backlog left behind", errors.New("retention sweep of traces incomplete"),
			metrics.RetentionResultFailed},
	} {
		require.Equal(t, tc.want, retentionSweepResult(tc.err), tc.name)
	}
}

// TestRetentionSweepMetricRejectsUnknownTargets verifies the target label cannot
// be widened past the SQL allow-list, so no caller can turn it into unbounded
// cardinality.
func TestRetentionSweepMetricRejectsUnknownTargets(t *testing.T) {
	spy := installRetentionSweepSpy(t)

	recordRetentionSweep("traces", ChunkedDeleteStats{Deleted: 3}, nil, time.Millisecond)
	recordRetentionSweep("/v1/chat/completions", ChunkedDeleteStats{Deleted: 3}, nil, time.Millisecond)
	recordRetentionSweep("", ChunkedDeleteStats{}, nil, time.Millisecond)

	require.Len(t, spy.sweeps, 1, "only allow-listed targets may be labelled")
	require.Equal(t, "traces", spy.sweeps[0].target)
	require.Equal(t, metrics.RetentionResultFailed,
		retentionSweepResult(errors.New("boom")),
		"a failed sweep still reports a bounded result label")
}

// TestRetentionSweepMetricReportsCompletedThroughput drives the real trace
// retention sweep and asserts the emitted throughput describes it.
func TestRetentionSweepMetricReportsCompletedThroughput(t *testing.T) {
	setupTestDatabase(t)
	cleanupTraces(t, "test-retention-metric-%")
	t.Cleanup(func() { cleanupTraces(t, "test-retention-metric-%") })

	expired := time.Now().UTC().Add(-72 * time.Hour).UnixMilli()
	seedRetentionTraces(t, "test-retention-metric-", 5, expired)

	spy := installRetentionSweepSpy(t)
	stats, err := CleanExpiredTracesStats(context.Background(), 1)
	require.NoError(t, err)
	require.GreaterOrEqual(t, stats.Deleted, int64(5))

	sample := spy.only(t)
	require.Equal(t, "traces", sample.target)
	require.Equal(t, metrics.RetentionResultCompleted, sample.result)
	require.Equal(t, float64(stats.Deleted), sample.rows,
		"the reported rows must be the rows the sweep actually removed")
	require.GreaterOrEqual(t, sample.durationMs, float64(0))
}

// TestRetentionSweepMetricReportsCanceledOnShutdown is the distinction section
// 8.3 depends on. A cancelled sweep stops at a chunk boundary with work still
// eligible; reporting that as completed would make a permanently behind gateway
// look healthy, because every sweep would "finish".
func TestRetentionSweepMetricReportsCanceledOnShutdown(t *testing.T) {
	setupTestDatabase(t)
	cleanupTraces(t, "test-retention-cancel-%")
	t.Cleanup(func() { cleanupTraces(t, "test-retention-cancel-%") })

	expired := time.Now().UTC().Add(-72 * time.Hour).UnixMilli()
	seedRetentionTraces(t, "test-retention-cancel-", 5, expired)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	spy := installRetentionSweepSpy(t)
	_, err := CleanExpiredTracesStats(ctx, 1)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)

	sample := spy.only(t)
	require.Equal(t, "traces", sample.target)
	require.Equal(t, metrics.RetentionResultCanceled, sample.result,
		"a sweep stopped by shutdown left work behind and is not completed")

	require.Equal(t, int64(5), countRetentionTraces(t, "test-retention-cancel-%"),
		"the cancelled sweep must have removed nothing, so work genuinely remained")
}

// TestRetentionSweepMetricCoversAsyncTaskBindings verifies the two-pass sweep
// emits ONE sample for the table rather than one per pass, which would otherwise
// double this target's sweep rate against every other one.
func TestRetentionSweepMetricCoversAsyncTaskBindings(t *testing.T) {
	setupTestDatabase(t)

	spy := installRetentionSweepSpy(t)
	_, err := CleanExpiredAsyncTaskBindingsStats(context.Background(), 1)
	require.NoError(t, err)

	sample := spy.only(t)
	require.Equal(t, "async_task_bindings", sample.target)
	require.Equal(t, metrics.RetentionResultCompleted, sample.result)
}

// TestRetentionSweepMetricCoversOperatorLogPurge verifies the operator-triggered
// log purge reports through the same series as the periodic sweeps.
func TestRetentionSweepMetricCoversOperatorLogPurge(t *testing.T) {
	setupTestDatabase(t)

	spy := installRetentionSweepSpy(t)
	_, err := DeleteOldLogContext(context.Background(), time.Now().Add(-720*time.Hour).Unix())
	require.NoError(t, err)

	sample := spy.only(t)
	require.Equal(t, "logs", sample.target)
	require.Equal(t, metrics.RetentionResultCompleted, sample.result)
}
