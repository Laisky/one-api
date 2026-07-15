package model

import (
	"context"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/logger"
)

var (
	// bootstrapMu guards the process-local initialization state shared by the bootstrap
	// orchestrator and the InitDB/InitLogDB compatibility wrappers.
	bootstrapMu sync.Mutex
	// initGeneration increments every time InitDB or InitDatabases opens a new primary
	// handle. Compatibility state is scoped to a generation so a reinitialized process does
	// not inherit the previous generation's reconciliation ownership.
	initGeneration uint64
	// catchUpOwnedGeneration records the generation whose reconciliation is already owned by
	// the orchestrator or by InitDB's compatibility catch-up, so a unified LOG_DB == DB
	// deployment does not run the same catch-up twice.
	catchUpOwnedGeneration uint64
	// catchUpOwnerClaimed reports whether catchUpOwnedGeneration holds a real claim.
	catchUpOwnerClaimed bool
	// catchUpWorkerCancel stops the background catch-up worker at shutdown.
	catchUpWorkerCancel context.CancelFunc
	// catchUpWorkerDone closes once the background catch-up worker has fully exited, so a
	// caller that replaces the database handles can be sure no cycle is still in flight.
	catchUpWorkerDone chan struct{}
)

// InitDatabases is the supported database bootstrap orchestrator. It initializes the primary
// and log schemas, including data_migrations, constructs the explicit topology, and only then
// runs the global UUID migration coordinator exactly once. The application entry point uses
// this path so normal unified startup does not execute both compatibility wrappers.
// Parameters:
//   - ctx: context bounding schema initialization and the background catch-up worker.
//
// Return values:
//   - error: wrapped error when database initialization or finalizer-mode migration fails.
func InitDatabases(ctx context.Context) error {
	ctx = withUUIDMigrationLogger(ctx)

	if err := initPrimaryDatabase(); err != nil {
		return errors.Wrap(err, "initialize primary database")
	}
	topology, err := initLogDatabase()
	if err != nil {
		return errors.Wrap(err, "initialize log database")
	}
	setDatabaseTopology(topology)

	// The orchestrator owns reconciliation for this generation, so a later InitDB call in
	// the same generation does not repeat it.
	claimCatchUpOwnership()

	if !config.IsMasterNode {
		// Non-master nodes never execute catch-up, DDL, validation, or marker writes.
		logger.FromContext(ctx).Debug("external uuid migration skipped on non-master node")
		return nil
	}
	return startExternalUUIDMigration(ctx, topology)
}

// startExternalUUIDMigration runs finalizer mode synchronously or schedules catch-up.
// Finalizer mode must fail startup on any reconciliation, DDL, validation, or marker error.
// Catch-up mode, including its candidate-index DDL, is moved off the readiness-critical path
// into a context-bound background worker.
// Parameters:
//   - ctx: context bounding the migration and the background worker.
//   - topology: explicitly constructed database topology.
//
// Return values:
//   - error: wrapped error only when finalizer-mode migration fails.
func startExternalUUIDMigration(ctx context.Context, topology *databaseTopology) error {
	if externalUUIDBackfillFinalizerEnabled {
		if _, err := runUUIDMigrationCoordinator(ctx, topology, uuidMigrationModeFinalizer); err != nil {
			return errors.Wrap(err, "finalize external resource uuids")
		}
		return nil
	}

	markers, err := readUUIDMarkerState(ctx, topology)
	if err != nil {
		return errors.Wrap(err, "check external uuid completion markers")
	}
	if markers.allPresent() {
		// Completed startup is marker-only: no worker, no target or reference queries.
		return nil
	}
	startUUIDCatchUpWorker(ctx, topology)
	return nil
}

// startUUIDCatchUpWorker resumes historical catch-up in a context-bound background worker.
// Each cycle is bounded by the row and time budget, so readiness is never delayed by the
// backlog or by candidate-index DDL. This function is the bootstrap boundary for catch-up:
// it logs each terminal error exactly once.
// Parameters:
//   - ctx: context controlling worker lifetime.
//   - topology: explicitly constructed database topology.
//
// Return values: none.
func startUUIDCatchUpWorker(ctx context.Context, topology *databaseTopology) {
	// Retiring any previous worker before starting a new one keeps at most one cycle in
	// flight against the current handles.
	stopUUIDCatchUpWorker()

	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	bootstrapMu.Lock()
	catchUpWorkerCancel = cancel
	catchUpWorkerDone = done
	bootstrapMu.Unlock()

	go func() {
		defer close(done)
		defer cancel()
		// idlePasses counts consecutive full no-work passes. Sustained quiescence is the
		// automatic stand-in for the drained mixed-writer window: once no cycle has found
		// anything to reconcile for the configured number of idle passes, every writer that
		// is still running has proven itself UUID-aware, and the worker finalizes so a
		// default deployment completes without any operator flag.
		idlePasses := 0
		for {
			delay := uuidCatchUpIdleInterval()
			result, err := runUUIDMigrationCoordinator(workerCtx, topology, uuidMigrationModeCatchUp)
			switch {
			case err != nil:
				if workerCtx.Err() != nil {
					return
				}
				idlePasses = 0
				logger.FromContext(workerCtx).Error("external uuid catch-up cycle failed", zap.Error(err))
			case result.completed:
				// Markers appeared, so another process finalized; nothing left to do.
				return
			case result.updated > 0 || result.budgetExhausted:
				idlePasses = 0
				delay = uuidCatchUpActiveInterval()
			default:
				idlePasses++
				if uuidAutoFinalizeEnabled() && idlePasses >= uuidAutoFinalizeIdlePasses() {
					if runAutoFinalize(workerCtx, topology) {
						return
					}
					// A failed attempt is retryable by design: no marker was written and
					// the ordinary indexes stay usable. Requiring a fresh quiescence
					// streak spaces retries out instead of hammering validation.
					idlePasses = 0
				}
			}
			select {
			case <-workerCtx.Done():
				return
			case <-time.After(delay):
			}
		}
	}()
}

// runAutoFinalize attempts automatic finalization from the background worker.
// It runs the same full finalizer the operator flag would run — ordered reconciliation,
// unique-index promotion, global validation, then markers — and is idempotent: rerunning
// after a failure or a crash resumes from durable state and never rewrites populated values.
// Unlike the flag-driven synchronous path it must never take the process down; a failure is
// reported once and retried after the next quiescence streak.
// Parameters:
//   - ctx: worker context bounding the attempt.
//   - topology: explicitly constructed database topology.
//
// Return values:
//   - bool: true when finalization completed and the worker may exit.
func runAutoFinalize(ctx context.Context, topology *databaseTopology) bool {
	log := logger.FromContext(ctx)
	log.Info("external uuid catch-up reached sustained quiescence; finalizing automatically",
		zap.String("topology", string(topology.mode)))
	if _, err := runUUIDMigrationCoordinator(ctx, topology, uuidMigrationModeFinalizer); err != nil {
		if ctx.Err() != nil {
			return false
		}
		log.Error("automatic external uuid finalization failed; will retry after the next quiescent window",
			zap.Error(err))
		return false
	}
	log.Info("external uuid migration completed automatically",
		zap.String("topology", string(topology.mode)))
	return true
}

// stopUUIDCatchUpWorker cancels the background catch-up worker and waits for it to exit.
//
// Waiting is required, not tidiness: a caller that replaces or closes the database handles
// while a cycle is still running would leave the worker issuing statements against a
// replaced or closed pool. The mutex is released before waiting so the worker can finish
// without contending on it.
// Parameters: none.
//
// Return values: none.
func stopUUIDCatchUpWorker() {
	// The whole stop is serialized so two concurrent callers both observe a stopped worker.
	// Releasing the lock and clearing the fields first would let the loser see a nil cancel
	// and return while the worker was still running — exactly the case CloseDB and
	// initPrimaryDatabase must not hit, since they go on to replace or close the handles.
	// Holding the lock across the wait cannot deadlock: the worker goroutine never acquires
	// bootstrapMu, and startUUIDCatchUpWorker stops before it locks.
	bootstrapMu.Lock()
	defer bootstrapMu.Unlock()

	if catchUpWorkerCancel == nil {
		return
	}
	catchUpWorkerCancel()
	if catchUpWorkerDone != nil {
		<-catchUpWorkerDone
	}
	catchUpWorkerCancel = nil
	catchUpWorkerDone = nil
}

// beginInitGeneration records that a new primary handle was opened and clears the previous
// generation's reconciliation ownership.
// Parameters: none.
//
// Return values: none.
func beginInitGeneration() {
	bootstrapMu.Lock()
	defer bootstrapMu.Unlock()
	initGeneration++
	catchUpOwnerClaimed = false
}

// claimCatchUpOwnership marks the current generation's reconciliation as owned.
// Parameters: none.
//
// Return values:
//   - bool: true when this call took ownership, false when it was already claimed.
func claimCatchUpOwnership() bool {
	bootstrapMu.Lock()
	defer bootstrapMu.Unlock()
	if catchUpOwnerClaimed && catchUpOwnedGeneration == initGeneration {
		return false
	}
	catchUpOwnerClaimed = true
	catchUpOwnedGeneration = initGeneration
	return true
}

// compatibilityCatchUpAlreadyRan reports whether the current generation's reconciliation is
// already owned by the orchestrator or by InitDB's compatibility catch-up.
// Parameters: none.
//
// Return values:
//   - bool: true when reconciliation for this generation is already owned.
func compatibilityCatchUpAlreadyRan() bool {
	bootstrapMu.Lock()
	defer bootstrapMu.Unlock()
	return catchUpOwnerClaimed && catchUpOwnedGeneration == initGeneration
}

// runCompatibilityCatchUp performs the primary-only, marker-free catch-up that InitDB-only
// callers rely on. It cannot finalize split-database state: only the global coordinator
// invoked through InitLogDB or InitDatabases has completion authority.
//
// The work runs in the background worker rather than inline. The historical side effect that
// InitDB triggers primary catch-up is preserved, but a large backlog and its candidate-index
// DDL must not sit on the readiness-critical path, and a single bounded cycle would leave the
// remaining backlog stranded until the next restart.
// Parameters:
//   - ctx: context bounding the background worker.
//
// Return values:
//   - error: wrapped error when the catch-up topology cannot be constructed.
func runCompatibilityCatchUp(ctx context.Context) error {
	ctx = withUUIDMigrationLogger(ctx)
	if config.LogSQLDSN != "" {
		// A dedicated log database is configured, so this deployment is split and the
		// primary logs table is not authoritative. Reconciling it here could scan or
		// mutate a stale primary logs table, so only the global coordinator reached
		// through InitLogDB or InitDatabases may run.
		return nil
	}
	if !claimCatchUpOwnership() {
		return nil
	}

	topology, err := newUnifiedTopology(DB)
	if err != nil {
		return errors.Wrap(err, "construct primary-only catch-up topology")
	}
	setDatabaseTopology(topology)

	// Never finalize synchronously here, even when the finalizer flag is set: the
	// flag-driven synchronous path belongs to the global coordinator reached through
	// InitLogDB or InitDatabases. What this path starts is the ordinary background worker
	// over the unified topology — which is authoritative here, because a dedicated log DSN
	// was ruled out above — and that worker may complete the migration automatically after
	// sustained quiescence, exactly as it would for any other bootstrap path.
	markers, err := readUUIDMarkerState(ctx, topology)
	if err != nil {
		return errors.Wrap(err, "check external uuid completion markers")
	}
	if markers.allPresent() {
		return nil
	}
	startUUIDCatchUpWorker(ctx, topology)
	return nil
}

// resetBootstrapStateForTest clears process-local bootstrap state between tests.
// Parameters: none.
//
// Return values: none.
func resetBootstrapStateForTest() {
	stopUUIDCatchUpWorker()
	bootstrapMu.Lock()
	catchUpOwnerClaimed = false
	initGeneration++
	bootstrapMu.Unlock()
	setDatabaseTopology(nil)
}
