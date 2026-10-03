package model

// Retention worker lifecycle (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 1 / W1, row
// "Shutdown": "Stop admissions, drain HTTP and background/billing producers,
// close consuming sinks, flush exporters, then close databases; deadlines report
// unfinished work").
//
// The retention cleaners are producers of database work exactly like the relay
// handlers are, and they outlived the resource they write through: main.go
// started them with a context that was never cancelled, so their tickers kept
// firing during shutdown and after CloseDB, issuing DELETEs against a closed
// pool. This file supplies the join half of the fix -- the cancel half is the
// caller's context -- following the pattern CloseDB already uses for the UUID
// catch-up and compact loops: cancel, then WAIT, and only then let the handles
// close.
//
// Waiting matters as much as cancelling. Cancellation only asks a sweep to stop
// at its next chunk boundary (ChunkedDeleteWithStats checks ctx.Err() between
// chunks); without joining, the shutdown sequence would race the last in-flight
// DELETE to the database close and could not report that the work was
// unfinished.

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/Laisky/errors/v2"
)

var (
	// retentionWorkerGroup tracks the retention cleaner goroutines started in
	// this process, so a shutdown can prove they stopped rather than assume it.
	retentionWorkerGroup  sync.WaitGroup
	retentionWorkerMu     sync.Mutex
	retentionWorkerDoneCh = make(chan struct{})
	// retentionWorkersActive mirrors the group's counter, which sync.WaitGroup
	// does not expose. Without it "nothing to wait for" and "waited and timed
	// out" are indistinguishable when the shutdown deadline has already
	// expired: Wait must run in another goroutine, so the select would pick
	// randomly between an immediately-closed done channel and an
	// already-cancelled context, and a deployment with retention disabled could
	// report retention work as unfinished at random.
	retentionWorkersActive  atomic.Int64
	backgroundWorkerMu      sync.Mutex
	backgroundWorkersActive atomic.Int64
	backgroundWorkersDone   = make(chan struct{})
)

// StartBackgroundWorker starts a joinable non-retention database producer.
//
// Parameters:
//   - ctx: lifecycle scope for work; nil uses context.Background.
//   - work: the worker body; nil starts nothing.
//
// Return values: none.
func StartBackgroundWorker(ctx context.Context, work func(context.Context)) {
	if work == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	backgroundWorkerMu.Lock()
	if backgroundWorkersActive.Load() == 0 {
		backgroundWorkersDone = make(chan struct{})
	}
	backgroundWorkersActive.Add(1)
	backgroundWorkerMu.Unlock()

	go func() {
		defer func() {
			backgroundWorkerMu.Lock()
			if backgroundWorkersActive.Add(-1) == 0 {
				close(backgroundWorkersDone)
			}
			backgroundWorkerMu.Unlock()
		}()
		work(ctx)
	}()
}

// WaitForBackgroundWorkers waits for non-retention database producers to stop.
//
// Parameters:
//   - ctx: shutdown deadline; nil uses context.Background.
//
// Return values:
//   - error: wrapped context error when work remains at the deadline.
func WaitForBackgroundWorkers(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	backgroundWorkerMu.Lock()
	if backgroundWorkersActive.Load() == 0 {
		backgroundWorkerMu.Unlock()
		return nil
	}
	done := backgroundWorkersDone
	backgroundWorkerMu.Unlock()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		select {
		case <-done:
			return nil
		default:
			return errors.Wrap(ctx.Err(), "wait for background workers")
		}
	}
}

// addRetentionWorker registers one retention cleaner goroutine before it starts.
//
// Parameters: none.
//
// Return values: none.
func addRetentionWorker() {
	retentionWorkerMu.Lock()
	if retentionWorkersActive.Load() == 0 {
		retentionWorkerDoneCh = make(chan struct{})
	}
	retentionWorkerGroup.Add(1)
	retentionWorkersActive.Add(1)
	retentionWorkerMu.Unlock()
}

// retentionWorkerDone records that one retention cleaner goroutine returned.
//
// Parameters: none.
//
// Return values: none.
func retentionWorkerDone() {
	retentionWorkerMu.Lock()
	retentionWorkerGroup.Done()
	if retentionWorkersActive.Add(-1) == 0 {
		close(retentionWorkerDoneCh)
	}
	retentionWorkerMu.Unlock()
}

// WaitForRetentionCleaners blocks until every retention cleaner goroutine
// started by StartTraceRetentionCleaner and StartAsyncTaskRetentionCleaner has
// returned, or until ctx expires.
//
// It does NOT cancel anything: the caller owns the workers' context and must
// cancel it first, otherwise this call simply waits for the deadline. The
// returned error carries ctx's cause, which is what lets the shutdown sequence
// attribute the unfinished retention work to the expired deadline.
//
// Parameters:
//   - ctx: the shutdown deadline; a nil context is treated as background.
//
// Return values:
//   - error: wrapped ctx error when the cleaners were still running at the
//     deadline; nil when every cleaner had returned.
func WaitForRetentionCleaners(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	retentionWorkerMu.Lock()
	if retentionWorkersActive.Load() == 0 {
		retentionWorkerMu.Unlock()
		return nil
	}
	done := retentionWorkerDoneCh
	retentionWorkerMu.Unlock()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		// A worker that returned in the same instant the deadline expired is
		// finished work, not unfinished work; give the join that last look
		// before reporting.
		select {
		case <-done:
			return nil
		default:
			return errors.Wrap(ctx.Err(), "wait for retention cleaners")
		}
	}
}
