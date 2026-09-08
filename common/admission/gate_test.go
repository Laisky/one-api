package admission

// Tests for the concurrency gate (W2.4 step 5).

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestGateAdmitsUpToCapacity verifies the gate lets exactly capacity callers in
// and refuses the next one within its wait budget.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestGateAdmitsUpToCapacity(t *testing.T) {
	gate := NewGate("test", 2, 20*time.Millisecond)

	releaseA, err := gate.Acquire(context.Background())
	require.NoError(t, err)
	releaseB, err := gate.Acquire(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, gate.InFlight())

	start := time.Now()
	release, err := gate.Acquire(context.Background())
	require.ErrorIs(t, err, ErrBusy)
	require.Nil(t, release)
	require.GreaterOrEqual(t, time.Since(start), 20*time.Millisecond,
		"a refused caller must have waited its budget, not failed instantly")

	releaseA()
	releaseC, err := gate.Acquire(context.Background())
	require.NoError(t, err, "a freed slot must be reusable")
	releaseB()
	releaseC()
	require.Zero(t, gate.InFlight())
}

// TestGateReleaseIsIdempotent verifies a double release cannot free a slot the
// caller does not hold.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestGateReleaseIsIdempotent(t *testing.T) {
	gate := NewGate("test", 1, time.Millisecond)
	release, err := gate.Acquire(context.Background())
	require.NoError(t, err)

	release()
	release()
	require.Zero(t, gate.InFlight())

	again, err := gate.Acquire(context.Background())
	require.NoError(t, err)
	again()
}

// TestGateHonorsContext verifies a cancelled caller is refused immediately.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestGateHonorsContext(t *testing.T) {
	gate := NewGate("test", 1, time.Hour)
	held, err := gate.Acquire(context.Background())
	require.NoError(t, err)
	defer held()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	release, err := gate.Acquire(ctx)
	require.Error(t, err)
	require.Nil(t, release)
	require.Less(t, time.Since(start), time.Second, "a cancelled caller must not wait the full budget")
}

// TestGateIsSafeUnderConcurrency verifies the gate never admits more than its
// capacity when many callers contend.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestGateIsSafeUnderConcurrency(t *testing.T) {
	const capacity = 3
	gate := NewGate("test", capacity, 200*time.Millisecond)

	var (
		mu      sync.Mutex
		current int
		peak    int
		wg      sync.WaitGroup
	)

	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := gate.Acquire(context.Background())
			if err != nil {
				return
			}
			defer release()

			mu.Lock()
			current++
			if current > peak {
				peak = current
			}
			mu.Unlock()

			time.Sleep(time.Millisecond)

			mu.Lock()
			current--
			mu.Unlock()
		}()
	}
	wg.Wait()

	require.LessOrEqual(t, peak, capacity, "the gate must never admit beyond its capacity")
	require.Zero(t, gate.InFlight())
}
