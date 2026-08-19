package model

// This file schedules the compact migration worker and the per-process health monitor
// (AUTO-008, proposal sections 8.1 and 8.6).
//
// Two loops with independent lifecycles live here, and keeping them independent is contractual:
//
//   - The MUTATION worker runs on the master only. It performs DDL, fill, repair, validation,
//     and marker writes.
//   - The HEALTH monitor runs on EVERY process, is strictly read-only, and is what allows a
//     non-master to use compact predicates at all.
//
// The completed worker does not stop. That is the deliberate difference from the v3 backfill's
// lifecycle: compact markers record historical installation, and the audit that detects later
// drift must keep running forever.

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/Laisky/zap"
)

var (
	// compactWorkerMu guards the process-local worker and monitor handles.
	compactWorkerMu sync.Mutex
	// compactWorkerCancel stops the mutating worker at shutdown.
	compactWorkerCancel context.CancelFunc
	// compactWorkerDone closes once the mutating worker has fully exited.
	compactWorkerDone chan struct{}
	// compactMonitorCancel stops the read-only health monitor at shutdown.
	compactMonitorCancel context.CancelFunc
	// compactMonitorDone closes once the health monitor has fully exited.
	compactMonitorDone chan struct{}
	// compactRepairSignal carries lookup-fallback signals to the worker. It is buffered by one
	// because a signal means "look again soon", so coalescing several into one is correct and
	// a full buffer must never block the request path that raised it.
	compactRepairSignal = make(chan struct{}, 1)
)

// signalCompactRepair asks the worker to start a repair cycle promptly.
//
// The send is non-blocking by design: this is called from the runtime lookup fallback path, and
// a request must never wait on the migration worker.
// Parameters: none.
//
// Return values: none.
func signalCompactRepair() {
	select {
	case compactRepairSignal <- struct{}{}:
	default:
	}
}

// startCompactWorker starts the master-only mutating worker.
//
// It is started after the v3 worker has been scheduled or completed, and it supervises nothing
// else: starting or stopping it must never affect the v3 worker or the health monitor.
// Parameters:
//   - ctx: context controlling worker lifetime.
//   - topology: explicitly constructed database topology.
//
// Return values: none.
func startCompactWorker(ctx context.Context, topology *databaseTopology) {
	stopCompactWorker()

	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	compactWorkerMu.Lock()
	compactWorkerCancel = cancel
	compactWorkerDone = done
	compactWorkerMu.Unlock()

	go func() {
		defer close(done)
		defer cancel()
		runCompactWorkerLoop(workerCtx, topology)
	}()
}

// runCompactWorkerLoop drives cycles until cancellation.
//
// The loop never exits on completion. Markers record that the historical installation finished;
// the audit and repair that follow are what keep a later dropped trigger or direct write from
// silently degrading every compact read.
// Parameters:
//   - ctx: context controlling the loop.
//   - topology: explicitly constructed database topology.
//
// Return values: none.
func runCompactWorkerLoop(ctx context.Context, topology *databaseTopology) {
	coordinator := newCompactCoordinator(topology)
	for {
		delay := runCompactWorkerCycle(ctx, coordinator)
		select {
		case <-ctx.Done():
			return
		case <-compactRepairSignal:
			// A lookup fallback is consumed promptly rather than waiting out the idle
			// interval: the proposal requires the signal to be consumed within one second.
		case <-time.After(delay):
		}
	}
}

// runCompactWorkerCycle runs one cycle and returns the delay before the next.
//
// This function is the worker's error boundary: each terminal error is logged exactly once
// here, never both logged and returned by an inner layer.
// Parameters:
//   - ctx: context bounding the cycle.
//   - coordinator: worker state carried across cycles.
//
// Return values:
//   - time.Duration: delay before the next cycle.
func runCompactWorkerCycle(ctx context.Context, coordinator *compactCoordinator) time.Duration {
	log := compactLogger(ctx)

	if !compactAutoMigrateEnabled() {
		// Emergency pause: mutate nothing, keep serving legacy, and keep auditing read-only.
		publishCompactPassive(coordinator.topology, "automatic compact migration is paused by configuration")
		return compactIdleInterval()
	}

	started := time.Now()
	ownership, acquired, err := acquireCompactOwnership(ctx, coordinator.topology)
	recordCompactDuration(coordinator.topology, compactOperationLock, time.Since(started))
	if err != nil {
		if ctx.Err() != nil {
			return compactIdleInterval()
		}
		coordinator.failures++
		coordinator.resetEpoch()
		log.Error("compact uuid ownership acquisition failed", zap.Error(err))
		publishCompactState(coordinator.topology, compactStateRetryWait, false, "ownership acquisition failed")
		return compactBackoffDelay(coordinator.failures)
	}
	if !acquired {
		// Another instance owns the work, so this worker's clean-pass streak no longer
		// describes a world it alone controlled: the owner may repair rows or change objects
		// between our passes. Discarding the streak is the conservative reading of the
		// proposal's "owner change" epoch reset, and costs only one extra validation pass.
		coordinator.resetEpoch()
		// This process still audits read-only, so it can use compact predicates once its own
		// audit passes, master or not.
		runCompactHealthAudit(ctx, coordinator.topology)
		return compactIdleInterval()
	}
	defer ownership.release()

	cycleStarted := time.Now()
	result, err := runCompactCycle(ctx, coordinator, ownership)
	recordCompactDuration(coordinator.topology, compactOperationCycle, time.Since(cycleStarted))
	if err != nil {
		if ctx.Err() != nil {
			return compactIdleInterval()
		}
		coordinator.failures++
		coordinator.resetEpoch()
		log.Error("compact uuid migration cycle failed", zap.Error(err))
		recordCompactAction(coordinator.topology, compactActionCycle, uuidResultFailure)
		publishCompactState(coordinator.topology, compactStateRetryWait, false, "transient cycle failure")
		return compactBackoffDelay(coordinator.failures)
	}

	if result.progressed {
		// Any durable progress resets the backoff exponent.
		coordinator.failures = 0
		recordCompactProgress(coordinator.topology)
	}
	recordCompactAction(coordinator.topology, compactActionCycle, uuidResultSuccess)
	publishCompactState(coordinator.topology, result.state, result.state == compactStateReady, result.reason)

	if result.state == compactStateReady && !result.progressed {
		return compactIdleInterval()
	}
	if result.state == compactStateBlockedValidation {
		log.Warn("compact uuid migration is blocked by invalid source data or an engine blocker",
			zap.String("reason", result.reason),
			zap.Int("blocker_rows", result.blockers))
		return compactIdleInterval()
	}
	return compactActiveInterval()
}

// compactBackoffDelay returns the bounded full-jitter delay after consecutive failures.
//
// The proposal fixes the shape: full jitter over [0, retry × 2^min(failures, 5)]. Full jitter
// rather than a fixed exponential is what stops a fleet of instances that all lost the same
// database from retrying in lockstep and hammering it the moment it returns.
// Parameters:
//   - failures: consecutive failure count.
//
// Return values:
//   - time.Duration: jittered delay before the next attempt.
func compactBackoffDelay(failures int) time.Duration {
	exponent := failures
	if exponent > 5 {
		exponent = 5
	}
	if exponent < 0 {
		exponent = 0
	}
	ceiling := compactRetryInterval() * time.Duration(1<<uint(exponent))
	if ceiling <= 0 {
		return compactRetryInterval()
	}
	//nolint:gosec // Jitter spreads retries across instances; it is not a security decision.
	return time.Duration(rand.Int63n(int64(ceiling) + 1))
}

// startCompactHealthMonitor starts the read-only per-process audit loop.
//
// Every process runs this, master or not. It is what lets a non-master use compact predicates:
// its own fresh audit, never another process's claim and never a marker alone.
// Parameters:
//   - ctx: context controlling monitor lifetime.
//   - topology: explicitly constructed database topology.
//
// Return values: none.
func startCompactHealthMonitor(ctx context.Context, topology *databaseTopology) {
	stopCompactHealthMonitor()

	monitorCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	compactWorkerMu.Lock()
	compactMonitorCancel = cancel
	compactMonitorDone = done
	compactWorkerMu.Unlock()

	go func() {
		defer close(done)
		defer cancel()
		for {
			runCompactHealthAudit(monitorCtx, topology)
			select {
			case <-monitorCtx.Done():
				return
			case <-time.After(compactIdleInterval()):
			}
		}
	}()
}

// runCompactHealthAudit performs one read-only audit and publishes the result.
//
// It issues no DDL, no repair, and no marker write, so it is safe on every process including
// non-masters and replicas.
// Parameters:
//   - ctx: context bounding the audit.
//   - topology: explicitly constructed database topology.
//
// Return values: none.
func runCompactHealthAudit(ctx context.Context, topology *databaseTopology) {
	started := time.Now()
	defer func() { recordCompactDuration(topology, compactOperationAudit, time.Since(started)) }()

	supported, reason, err := validateCompactTopology(topology)
	if err != nil || !supported {
		if reason == "" {
			reason = "compact topology is unsupported"
		}
		for _, role := range topology.targetRoles() {
			disableCompactReads(role, compactStateBlockedValidation, reason)
		}
		publishCompactStateMetrics(topology, compactStateBlockedValidation)
		return
	}

	markers, err := readCompactMarkerState(ctx, topology)
	if err != nil {
		for _, role := range topology.targetRoles() {
			disableCompactReads(role, compactStatePassiveLegacy, "compact marker state could not be read")
		}
		recordCompactAction(topology, compactActionAudit, uuidResultFailure)
		return
	}
	if !markers.allPresent() {
		for _, role := range topology.targetRoles() {
			disableCompactReads(role, compactStatePassiveLegacy, "compact completion markers are not present")
		}
		publishCompactStateMetrics(topology, compactStatePassiveLegacy)
		return
	}

	verified, objectReason, err := validateCompactObjects(ctx, topology)
	if err != nil {
		for _, role := range topology.targetRoles() {
			disableCompactReads(role, compactStatePassiveLegacy, "compact object audit failed")
		}
		recordCompactAction(topology, compactActionAudit, uuidResultFailure)
		return
	}
	if !verified {
		// Markers exist but the objects do not match: this is drift. Compact predicates are
		// disabled process-wide immediately, and the marker timestamp is left alone.
		for _, role := range topology.targetRoles() {
			disableCompactReads(role, compactStateDegraded, objectReason)
		}
		publishCompactStateMetrics(topology, compactStateDegraded)
		recordCompactAction(topology, compactActionAudit, uuidResultSuccess)
		signalCompactRepair()
		return
	}

	for _, role := range topology.targetRoles() {
		publishCompactHealth(role, compactHealth{
			state:           compactStateReady,
			compactReadable: true,
			observedAt:      time.Now().UTC(),
		})
	}
	publishCompactStateMetrics(topology, compactStateReady)
	recordCompactAction(topology, compactActionAudit, uuidResultSuccess)
}

// publishCompactState publishes one cycle's state to the gate and to metrics.
// Parameters:
//   - topology: explicitly constructed database topology.
//   - state: aggregate state to publish.
//   - readable: whether compact predicates may be used.
//   - reason: bounded, value-free reason.
//
// Return values: none.
func publishCompactState(topology *databaseTopology, state compactState, readable bool, reason string) {
	for _, role := range topology.targetRoles() {
		if readable {
			publishCompactHealth(role, compactHealth{
				state:           state,
				compactReadable: true,
				observedAt:      time.Now().UTC(),
			})
			continue
		}
		disableCompactReads(role, state, reason)
	}
	publishCompactStateMetrics(topology, state)
}

// publishCompactPassive records that the worker is intentionally not mutating anything.
// Parameters:
//   - topology: explicitly constructed database topology.
//   - reason: bounded, value-free reason.
//
// Return values: none.
func publishCompactPassive(topology *databaseTopology, reason string) {
	publishCompactState(topology, compactStatePassiveLegacy, false, reason)
}

// stopCompactWorker cancels the mutating worker and waits for it to exit.
//
// Waiting is required rather than tidy: a caller that replaces or closes the database handles
// while a cycle is mid-DDL would leave the worker issuing statements against a closed pool.
// Parameters: none.
//
// Return values: none.
func stopCompactWorker() {
	compactWorkerMu.Lock()
	defer compactWorkerMu.Unlock()
	if compactWorkerCancel == nil {
		return
	}
	compactWorkerCancel()
	if compactWorkerDone != nil {
		<-compactWorkerDone
	}
	compactWorkerCancel = nil
	compactWorkerDone = nil
}

// stopCompactHealthMonitor cancels the health monitor and waits for it to exit.
// Parameters: none.
//
// Return values: none.
func stopCompactHealthMonitor() {
	compactWorkerMu.Lock()
	defer compactWorkerMu.Unlock()
	if compactMonitorCancel == nil {
		return
	}
	compactMonitorCancel()
	if compactMonitorDone != nil {
		<-compactMonitorDone
	}
	compactMonitorCancel = nil
	compactMonitorDone = nil
}

// stopCompactLoops cancels both loops in the required order.
//
// Mutation workers are joined before health monitors, and both before either database handle is
// closed: a mutating cycle holds DDL and locks, so it must be the first thing to stop and the
// first thing proven stopped.
// Parameters: none.
//
// Return values: none.
func stopCompactLoops() {
	stopCompactWorker()
	stopCompactHealthMonitor()
	resetCompactHealthForTest()
}
