package controller

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
)

// Compile-time compatibility check for the exported automatic channel-test
// entry point. The context-aware lifecycle API must remain additive.
var _ func(int) = AutomaticallyTestChannels

// TestChannelSweepAdmissionIsAtomic verifies concurrent automatic or HTTP
// callers cannot both receive a successful start response before the worker has
// set its running flag.
func TestChannelSweepAdmissionIsAtomic(t *testing.T) {
	testAllChannelsLock.Lock()
	previous := testAllChannelsRunning
	testAllChannelsRunning = false
	testAllChannelsLock.Unlock()
	t.Cleanup(func() {
		testAllChannelsLock.Lock()
		testAllChannelsRunning = previous
		testAllChannelsLock.Unlock()
	})

	var admitted atomic.Int32
	var workers sync.WaitGroup
	for range 32 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if beginChannelSweep() {
				admitted.Add(1)
			}
		}()
	}
	workers.Wait()
	require.Equal(t, int32(1), admitted.Load())
	finishChannelSweep()
}

// TestOperatorChannelSweepIsJoinable verifies a request-triggered asynchronous
// sweep is registered with the same shutdown join used by periodic producers.
//
// Parameters:
//   - t: the running test.
//
// Return values: none.
func TestOperatorChannelSweepIsJoinable(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	previousHook := beforeChannelSweepRun
	beforeChannelSweepRun = func() error {
		close(started)
		<-release
		return context.Canceled
	}
	t.Cleanup(func() {
		beforeChannelSweepRun = previousHook
		select {
		case <-release:
		default:
			close(release)
		}
		finishChannelSweep()
	})

	ctx := gmw.SetLogger(context.Background(), logger.Logger)
	require.NoError(t, testChannels(ctx, false, "all"))
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("operator channel sweep did not start")
	}

	deadline, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, model.WaitForBackgroundWorkers(deadline), context.DeadlineExceeded,
		"shutdown must observe the request-triggered channel sweep")

	close(release)
	require.NoError(t, model.WaitForBackgroundWorkers(context.Background()))
}
