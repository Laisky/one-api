package controller

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/graceful"
	"github.com/Laisky/one-api/relay/model"
)

// TestGoProcessChannelRelayError_SnapshotsErrAtSpawn reproduces the data race on
// *bizErr in controller/relay.go.
//
// The async processChannelRelayError goroutine used to dereference *bizErr inside
// the spawned closure (evaluated when the goroutine runs), while the request
// goroutine rewrites bizErr.Error.Message for the client response after the retry
// loop (the line previously annotated "// BUG: bizErr is in race condition").
// Logging must observe the ORIGINAL upstream error, not the client-decorated
// message, and reading/writing Message from two goroutines without synchronization
// is a data race.
//
// The test parks the spawned goroutine on a gate, mutates bizErr.Error.Message on
// the request goroutine (simulating the client-facing decoration), then releases
// the gate and asserts that the goroutine recorded the ORIGINAL message. This is a
// behavioral assertion on the snapshot value, which is deterministic.
//
// Against the buggy form (snapshot taken inside the closure, after the gate wait)
// the goroutine records the client-decorated message and the assertion FAILS. With
// the fix (snapshot taken synchronously before GoCritical) it records the original
// message and the assertion PASSES.
func TestGoProcessChannelRelayError_SnapshotsErrAtSpawn(t *testing.T) {
	const originalMessage = "original-upstream-error"
	const decoratedMessage = "client-decorated"

	gate := make(chan struct{})
	processChannelRelayErrorGateForTest = gate
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(gate) }) }

	observed := make(chan string, 1)
	processChannelRelayErrorForTest = func(_ context.Context, params processChannelRelayErrorParams) {
		observed <- params.Err.Message
	}

	// Always release and drain the parked goroutine before clearing the test seams,
	// even when the assertion below fails via FailNow (which runs deferred funcs).
	t.Cleanup(func() {
		release()
		drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = graceful.Drain(drainCtx)
		processChannelRelayErrorGateForTest = nil
		processChannelRelayErrorForTest = nil
	})

	bizErr := &model.ErrorWithStatusCode{
		Error:      model.Error{Message: originalMessage},
		StatusCode: 400,
	}

	// Spawn the async processor; it is parked on the gate.
	goProcessChannelRelayError(context.Background(), bizErr, processChannelRelayErrorParams{
		RequestID:  "req_bizerr_race",
		UserId:     1,
		RequestURL: "/v1/chat/completions",
	})

	// Simulate the request goroutine rewriting Message for the client response
	// (controller/relay.go line ~397).
	bizErr.Error.Message = decoratedMessage

	// Release the goroutine and wait for it to record the snapshot.
	release()
	drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, graceful.Drain(drainCtx))

	select {
	case got := <-observed:
		require.Equal(t, originalMessage, got,
			"goProcessChannelRelayError must snapshot *bizErr synchronously at spawn "+
				"time; otherwise the async error processor observes the client-decorated "+
				"message and races the request goroutine's rewrite of bizErr.Error.Message")
	case <-time.After(5 * time.Second):
		t.Fatal("async processChannelRelayError did not run")
	}
}
