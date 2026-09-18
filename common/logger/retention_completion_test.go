package logger

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestRetentionWorkerCompletionPublishesCountBeforeJoin verifies each reusable
// worker cohort publishes its final counter before waking joiners. The immediate
// snapshot, not an eventual assertion, protects the shutdown accounting contract.
func TestRetentionWorkerCompletionPublishesCountBeforeJoin(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, WaitForRetentionWorkers(ctx))
	for i := 0; i < 1000; i++ {
		addRetentionWorker()
		returned := make(chan struct{})
		go func() {
			retentionWorkerDone()
			close(returned)
		}()
		err := WaitForRetentionWorkers(ctx)
		activeAfterJoin := retentionWorkersActive.Load()
		// Always let the helper finish before another cohort or a failed assertion.
		<-returned
		require.NoError(t, err)
		require.Zero(t, activeAfterJoin, "cohort %d reported completion with active workers", i)
	}
}

// TestRetentionWorkerCanceledWaitAllowsNextCohort verifies a canceled wait leaves
// no asynchronous WaitGroup waiter behind to race the next worker registration.
// Live workers still report cancellation; completed work succeeds even when the
// caller's deadline has already expired.
func TestRetentionWorkerCanceledWaitAllowsNextCohort(t *testing.T) {
	joinCtx, joinCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer joinCancel()
	require.NoError(t, WaitForRetentionWorkers(joinCtx))
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for i := 0; i < 1000; i++ {
		addRetentionWorker()
		err := WaitForRetentionWorkers(canceled)
		retentionWorkerDone()
		require.ErrorIs(t, err, context.Canceled)
		require.NoError(t, WaitForRetentionWorkers(canceled))
		require.Zero(t, retentionWorkersActive.Load())
	}
}
