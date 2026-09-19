package model

import (
	"context"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// waitMinimum waits for the real workload's qualification signal while the
// pinned artifact remains alive. Parameters: ctx bounds the wait. Returns: nil
// on the operation floor, or an error on early termination or cancellation.
// Mutable counters are read only after finished publishes their final values.
func (load *compactPrevLegacyLoad) waitMinimum(ctx context.Context) error {
	select {
	case <-load.minimumReached:
		return nil
	case <-load.finished:
		if load.failure != nil {
			return errors.Wrap(load.failure, "legacy workload ended before qualification")
		}
		return errors.Errorf("legacy workload stopped after %d operations, need %d",
			load.operations, compactPrevRequiredOperations)
	case <-ctx.Done():
		return errors.Wrap(ctx.Err(), "wait for legacy workload qualification")
	}
}

// TestCompactPrevLegacyLoadWaitMinimum verifies the bounded, fail-closed
// qualification barrier independently from database throughput. Parameters:
// t owns the test. Returns: none. The real binary/DB test retains its 1,000 floor.
func TestCompactPrevLegacyLoadWaitMinimum(t *testing.T) {
	t.Parallel()
	t.Run("does not infer completion from startup", func(t *testing.T) {
		load := &compactPrevLegacyLoad{minimumReached: make(chan struct{}), finished: make(chan struct{})}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		require.ErrorIs(t, load.waitMinimum(ctx), context.Canceled)
	})
	t.Run("explicit workload completion releases the barrier", func(t *testing.T) {
		load := &compactPrevLegacyLoad{minimumReached: make(chan struct{}), finished: make(chan struct{})}
		close(load.minimumReached)
		require.NoError(t, load.waitMinimum(context.Background()))
	})
	t.Run("premature stop cannot qualify", func(t *testing.T) {
		load := &compactPrevLegacyLoad{minimumReached: make(chan struct{}), finished: make(chan struct{}), operations: 984}
		close(load.finished)
		require.ErrorContains(t, load.waitMinimum(context.Background()), "984 operations, need 1000")
	})
	t.Run("workload failure remains visible", func(t *testing.T) {
		failure := errors.New("legacy write failed")
		load := &compactPrevLegacyLoad{minimumReached: make(chan struct{}), finished: make(chan struct{}), failure: failure}
		close(load.finished)
		require.ErrorIs(t, load.waitMinimum(context.Background()), failure)
	})
}
