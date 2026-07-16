package model

// This file is the automatic compact migration coordinator (AUTO-008, proposal section 8.4).
//
// One cycle performs at most one attempt per side effect, in dependency order, and every step
// is bounded. The ordering is not stylistic: a table becomes eligible for fill only after its
// shadows AND its complete trigger set verify, and an index is created before historical fill
// so the fill lands into an existing index and the NULL-backlog probe stays off a sequential
// scan.
//
// Row reconciliation is bounded by COMPACT_UUID_MAX_CYCLE_DURATION. DDL and full validation
// deliberately run outside that budget under their own timeouts, because a silently truncated
// validation pass would report clean over rows it never examined.

import (
	"context"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
)

// compactCycleResult is the observable outcome of one coordinator cycle.
type compactCycleResult struct {
	// state is the cycle's aggregate state after applying precedence.
	state compactState
	// updated counts rows whose shadow this cycle wrote.
	updated int
	// examined counts rows this cycle materialized and classified.
	examined int
	// blockers counts observations blocked by invalid authoritative data.
	blockers int
	// progressed reports that the cycle made durable progress of any kind.
	progressed bool
	// completed reports that every applicable compact marker now exists.
	completed bool
	// reason is a bounded, value-free description of a blocking or degraded condition.
	reason string
}

// compactCoordinator holds the state one worker carries across cycles.
//
// The clean-pass epoch lives here rather than in the database because it must reset on process
// restart: a pass observed by a previous process proves nothing about this one's view of the
// objects.
type compactCoordinator struct {
	// topology is the explicitly constructed database topology.
	topology *databaseTopology
	// cursors are the durable per-target traversal positions, keyed by target id.
	cursors map[string]compactCursor
	// cleanPasses counts consecutive clean full passes within the current epoch.
	cleanPasses int
	// epoch identifies the world the clean passes were observed in.
	epoch compactPassEpoch
	// failures counts consecutive transient failures, bounding the backoff exponent.
	failures int
	// cycles counts reconciliation cycles and rotates which target reconciles first, so a
	// target larger than the row budget cannot starve the ones behind it forever.
	cycles uint64
	// workerToken identifies this worker across its own cycles.
	//
	// It is deliberately per-worker rather than per-ownership-acquisition. The worker takes
	// and releases the ownership lock once per cycle, so a per-acquisition token would change
	// every cycle, reset the epoch every cycle, and make the two-clean-pass streak
	// unreachable — the migration would validate forever and never complete. Ownership loss
	// still resets the epoch, via the error path and the failed-acquisition path.
	workerToken uint64
}

// newCompactCoordinator builds a coordinator for one topology.
// Parameters:
//   - topology: explicitly constructed database topology.
//
// Return values:
//   - *compactCoordinator: coordinator with empty cursors and a zero epoch.
func newCompactCoordinator(topology *databaseTopology) *compactCoordinator {
	return &compactCoordinator{
		topology:    topology,
		cursors:     map[string]compactCursor{},
		workerToken: compactOwnerCounter.Add(1),
	}
}

// validateCompactTopology rejects topologies compact storage cannot support.
//
// A rejection here is deliberately narrow in blast radius: mixed-dialect split and SQLite split
// enter blocked_validation for compact work only. Legacy readiness and legacy traffic are
// unaffected, because the compatibility contract guarantees the application keeps working
// exactly as before whether or not compact ever completes.
// Parameters:
//   - topology: explicitly constructed database topology.
//
// Return values:
//   - bool: true when compact work may proceed on this topology.
//   - string: bounded, value-free reason when it may not.
//   - error: wrapped error when the topology itself is malformed.
func validateCompactTopology(topology *databaseTopology) (bool, string, error) {
	if err := topology.validate(); err != nil {
		return false, "", errors.Wrap(err, "validate compact topology")
	}
	primary := dialectName(topology.primary)
	if _, err := compactColumnType(primary); err != nil {
		return false, "primary database dialect has no approved compact representation", nil
	}
	if topology.mode != uuidTopologySplit {
		return true, "", nil
	}

	logDialect := dialectName(topology.log)
	if _, err := compactColumnType(logDialect); err != nil {
		return false, "log database dialect has no approved compact representation", nil
	}
	if primary != logDialect {
		// Per-handle dialect detection is what makes this detectable at all; a process-global
		// database flag would have silently applied one dialect's DDL to the other engine.
		return false, "mixed-dialect split topology is not supported by compact uuid storage in v1", nil
	}
	if primary == "sqlite" {
		return false, "sqlite split topology is not supported by compact uuid storage in v1", nil
	}
	return true, "", nil
}

// runCompactCycle performs one bounded coordinator cycle in dependency order.
// Parameters:
//   - ctx: context bounding the cycle.
//   - coordinator: worker state carried across cycles.
//   - ownership: the acquired ownership claim for this cycle.
//
// Return values:
//   - compactCycleResult: aggregate state and counts.
//   - error: wrapped error for a transient failure the caller should back off from.
func runCompactCycle(ctx context.Context, coordinator *compactCoordinator,
	ownership *compactOwnership) (compactCycleResult, error) {
	topology := coordinator.topology

	supported, reason, err := validateCompactTopology(topology)
	if err != nil {
		return compactCycleResult{}, err
	}
	if !supported {
		coordinator.resetEpoch()
		return compactCycleResult{state: compactStateBlockedValidation, reason: reason}, nil
	}

	markers, err := readCompactMarkerState(ctx, topology)
	if err != nil {
		return compactCycleResult{}, err
	}
	prerequisiteMet, err := compactPrerequisiteMet(ctx, topology)
	if err != nil {
		return compactCycleResult{}, err
	}

	if !prerequisiteMet && !markers.anyPresent() {
		// Wait for v3 before taking the legacy-index baseline, and therefore before any
		// compact DDL at all.
		//
		// This gate is load-bearing, not caution. The v3 finalizer promotes owned UUID
		// indexes and drops the ordinary ones it replaces — it legitimately changes legacy
		// UUID index metadata. The manifest is a pre-expansion baseline that is deliberately
		// never rewritten to match reality, because rewriting it would launder exactly the
		// change the compatibility contract forbids. So a baseline captured while v3 is still
		// running would mismatch the moment v3 finalized, and compact would block on its own
		// prerequisite, permanently.
		//
		// Section 2.1's qualification table is explicit about the required behavior for a
		// populated schema with no v3 markers: "V3 completes automatically first; compact
		// waits safely and then completes." Waiting is that behavior. Section 2.1 also
		// permits compact to expand while v3 runs, but that permission is a "may", and it
		// cannot be taken while v3 still has legacy index DDL outstanding.
		coordinator.resetEpoch()
		return compactCycleResult{
			state:  compactStateWaitingPrerequisite,
			reason: "external uuid v3 markers are not present yet",
		}, nil
	}

	// The manifest must exist and match before any compact DDL. It is the only evidence that
	// no legacy UUID index changed shape, and the contract forbids proceeding without it.
	for _, role := range topology.targetRoles() {
		db := topology.handle(role)
		if err := ensureCompactManifestTable(ctx, db); err != nil {
			return compactCycleResult{}, err
		}
		ok, manifestReason, err := ensureLegacyIndexManifest(ctx, db, role)
		if err != nil {
			return compactCycleResult{}, err
		}
		if !ok {
			coordinator.resetEpoch()
			return compactCycleResult{state: compactStateBlockedValidation, reason: manifestReason}, nil
		}
	}

	if capable, capableReason, err := compactDialectCapable(ctx, topology); err != nil {
		return compactCycleResult{}, err
	} else if !capable {
		coordinator.resetEpoch()
		return compactCycleResult{state: compactStateBlockedValidation, reason: capableReason}, nil
	}

	if err := requireOwnership(ctx, ownership); err != nil {
		return compactCycleResult{}, err
	}

	expanded, err := advanceCompactExpansion(ctx, coordinator, ownership)
	if err != nil {
		return compactCycleResult{}, err
	}
	if expanded {
		// Expansion changed the object set, so any prior clean passes described a
		// different world and cannot be combined with later ones.
		coordinator.resetEpoch()
		return compactCycleResult{state: compactStateExpanding, progressed: true}, nil
	}

	indexed, err := advanceCompactIndexing(ctx, coordinator, ownership)
	if err != nil {
		return compactCycleResult{}, err
	}
	if indexed {
		coordinator.resetEpoch()
		return compactCycleResult{state: compactStateIndexing, progressed: true}, nil
	}

	return runCompactReconciliation(ctx, coordinator, ownership, markers, prerequisiteMet)
}

// compactDialectCapable checks any dialect-specific engine prerequisite.
//
// SQLite is the case that matters: the persistent triggers use core unhex(), which exists only
// in SQLite 3.41.0 and newer, and they must run inside whichever engine each supported binary
// links. An incapable engine blocks compact work and leaves legacy service untouched.
// Parameters:
//   - ctx: context bounding the probe.
//   - topology: explicitly constructed database topology.
//
// Return values:
//   - bool: true when every handle's engine can host the contract.
//   - string: bounded, value-free reason when one cannot.
//   - error: wrapped error when the probe cannot be executed.
func compactDialectCapable(ctx context.Context, topology *databaseTopology) (bool, string, error) {
	for _, role := range topology.targetRoles() {
		db := topology.handle(role)
		if dialectName(db) != "sqlite" {
			continue
		}
		capable, reason, err := compactSQLiteCapable(ctx, db)
		if err != nil {
			return false, "", err
		}
		if !capable {
			return false, reason, nil
		}
	}
	return true, "", nil
}

// advanceCompactExpansion expands and triggers at most one table per cycle.
//
// One table per cycle keeps a single side effect per cycle and bounds how much DDL a cycle can
// hold locks for. The trigger set is installed immediately after the table's shadows verify,
// which is what closes the MySQL column/trigger window: MySQL auto-commits DDL so a gap is
// unavoidable, and the historical fill covers every row written inside it.
// Parameters:
//   - ctx: context bounding the DDL.
//   - coordinator: worker state.
//   - ownership: ownership claim, rechecked around the side effect.
//
// Return values:
//   - bool: true when this cycle expanded or triggered a table.
//   - error: wrapped error when DDL fails or ownership is lost.
func advanceCompactExpansion(ctx context.Context, coordinator *compactCoordinator,
	ownership *compactOwnership) (bool, error) {
	for _, table := range compactTablesForTopology(coordinator.topology) {
		db := coordinator.topology.handle(table.role)

		ready, err := compactTableExpanded(ctx, db, table)
		if err != nil {
			return false, err
		}
		if !ready {
			if err := requireOwnership(ctx, ownership); err != nil {
				return false, err
			}
			if _, err := expandCompactTable(ctx, db, table); err != nil {
				return false, err
			}
			if err := requireOwnership(ctx, ownership); err != nil {
				return false, err
			}
			if err := installCompactTriggers(ctx, db, table); err != nil {
				return false, err
			}
			return true, nil
		}

		state, err := verifyCompactTriggers(ctx, db, table)
		if err != nil {
			return false, err
		}
		if !state.installed {
			if err := requireOwnership(ctx, ownership); err != nil {
				return false, err
			}
			if err := installCompactTriggers(ctx, db, table); err != nil {
				return false, err
			}
			return true, nil
		}
	}
	return false, nil
}

// advanceCompactIndexing creates at most one compact index per cycle.
// Parameters:
//   - ctx: context bounding the DDL.
//   - coordinator: worker state.
//   - ownership: ownership claim, rechecked around the side effect.
//
// Return values:
//   - bool: true when this cycle created an index.
//   - error: wrapped error when DDL fails or ownership is lost.
func advanceCompactIndexing(ctx context.Context, coordinator *compactCoordinator,
	ownership *compactOwnership) (bool, error) {
	for _, target := range compactTargetsForTopology(coordinator.topology) {
		db := coordinator.topology.handle(target.role)
		valid, err := verifyCompactIndex(ctx, db, target)
		if err != nil {
			return false, err
		}
		if valid {
			continue
		}
		if err := requireOwnership(ctx, ownership); err != nil {
			return false, err
		}
		if _, err := ensureCompactIndex(ctx, db, target); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

// runCompactReconciliation repairs rows, validates, and completes when the evidence allows.
// Parameters:
//   - ctx: context bounding the cycle.
//   - coordinator: worker state.
//   - ownership: ownership claim.
//   - markers: compact marker state read at cycle entry.
//   - prerequisiteMet: whether every applicable v3 marker exists.
//
// Return values:
//   - compactCycleResult: aggregate state and counts.
//   - error: wrapped error for a transient failure.
func runCompactReconciliation(ctx context.Context, coordinator *compactCoordinator,
	ownership *compactOwnership, markers compactMarkerState, prerequisiteMet bool) (compactCycleResult, error) {
	result := compactCycleResult{state: compactStateBackfilling}

	// Row reconciliation runs under the cycle's own row-time budget, separate from the DDL
	// and validation timeouts above.
	rowCtx, cancel := context.WithTimeout(ctx, compactCycleDuration())
	defer cancel()

	budget := compactRowBudget()
	// The starting target rotates every cycle. Iterating the registry in fixed order shares one
	// global budget in a fixed sequence, which starves every target after the first as soon as a
	// table is larger than the budget: reconcileCompactTarget counts EXAMINED rows, not just
	// repaired ones, so a clean 100k-row target still consumes the whole 10,000-row budget on
	// every cycle, forever. Measured on a live 100k fixture: users.inviter_uuid sorts before
	// users.uuid, took the entire budget every cycle, and users.uuid never reconciled at all — so
	// validation kept reporting it actionable and the migration could never complete. Rotation
	// keeps each cycle's full throughput while guaranteeing every target reaches the front within
	// one rotation.
	targets := compactTargetsForTopology(coordinator.topology)
	if len(targets) > 0 {
		offset := int(coordinator.cycles % uint64(len(targets)))
		targets = append(append([]compactTarget{}, targets[offset:]...), targets[:offset]...)
	}
	coordinator.cycles++

	for _, target := range targets {
		if budget <= 0 {
			break
		}
		if err := requireOwnership(ctx, ownership); err != nil {
			return result, err
		}
		db := coordinator.topology.handle(target.role)
		cursor := coordinator.cursors[target.id()]
		progress, err := reconcileCompactTarget(rowCtx, db, target, cursor, budget)
		if err != nil {
			return result, err
		}
		// wrapped is NOT carried forward. It marks that this cycle's traversal reached the end
		// and rewound; the next cycle must be free to traverse again, which is what keeps the
		// post-completion rolling audit alive. Persisting it would freeze the audit forever.
		coordinator.cursors[target.id()] = compactCursor{
			position:  progress.cursor,
			updatedAt: time.Now().UTC(),
		}
		result.examined += progress.examined
		result.updated += progress.updated
		result.blockers += progress.blockers
		budget -= progress.examined
		recordCompactBacklog(target, progress)

		if progress.collisions > 0 {
			// An uncorrectable compact uniqueness permutation. It is derived data, but the
			// only way to resolve it row-by-row would be to mutate somebody's authoritative
			// text, so the contract is to block rather than to "fix" it. Blocking here also
			// stops the row spinning as actionable-but-unrepairable on every later pass.
			coordinator.resetEpoch()
			result.state = compactStateBlockedValidation
			result.blockers += progress.collisions
			result.reason = "compact uniqueness permutation for " + target.id() +
				" cannot be corrected without mutating authoritative text"
			return result, nil
		}
	}

	if result.updated > 0 {
		// Repair invalidates the clean-pass streak: the passes that preceded it observed a
		// world where these rows were still wrong.
		coordinator.resetEpoch()
		result.progressed = true
		return result, nil
	}

	report, err := runCompactValidationPass(ctx, coordinator.topology)
	if err != nil {
		coordinator.resetEpoch()
		return result, err
	}
	return applyCompactValidation(ctx, coordinator, ownership, markers, prerequisiteMet, report, result)
}

// applyCompactValidation turns one validation report into a state and, when earned, markers.
//
// The completion gate is deliberately conjunctive: two clean passes from the SAME epoch, the
// v3 prerequisite met, no blockers, and matching global fingerprints. Any one of those missing
// means no marker is written, and a missing marker is always the safe outcome — the
// application keeps serving authoritative text either way.
// Parameters:
//   - ctx: context bounding validation and marker writes.
//   - coordinator: worker state carrying the clean-pass epoch.
//   - ownership: ownership claim.
//   - markers: compact marker state read at cycle entry.
//   - prerequisiteMet: whether every applicable v3 marker exists.
//   - report: the validation pass just completed.
//   - result: partially filled cycle result.
//
// Return values:
//   - compactCycleResult: aggregate state and counts.
//   - error: wrapped error for a transient failure.
func applyCompactValidation(ctx context.Context, coordinator *compactCoordinator,
	ownership *compactOwnership, markers compactMarkerState, prerequisiteMet bool,
	report compactValidationReport, result compactCycleResult) (compactCycleResult, error) {
	result.blockers += report.blockers

	if report.objectReason != "" {
		coordinator.resetEpoch()
		result.state = compactStateExpanding
		result.reason = report.objectReason
		if markers.anyPresent() {
			// A marker plus missing or wrong objects is drift, not first-time expansion.
			result.state = compactStateDegraded
		}
		return result, nil
	}
	if report.blockers > 0 {
		coordinator.resetEpoch()
		result.state = compactStateBlockedValidation
		result.reason = report.blockerReason
		return result, nil
	}
	if report.actionable > 0 {
		coordinator.resetEpoch()
		result.state = compactStateBackfilling
		if markers.anyPresent() {
			result.state = compactStateDegraded
		}
		return result, nil
	}

	// A clean pass. Record it against the current epoch; a mismatched epoch restarts the count
	// rather than combining evidence from two different worlds.
	coordinator.recordCleanPass(ownership)
	if coordinator.cleanPasses < 2 {
		result.state = compactStateValidating
		return result, nil
	}

	if !prerequisiteMet {
		// Compact may expand, trigger, and backfill while v3 is still running, but it must
		// never write a completion marker before the v3 markers exist.
		result.state = compactStateWaitingPrerequisite
		result.reason = "external uuid v3 markers are not present yet"
		return result, nil
	}
	if markers.allPresent() {
		result.state = compactStateReady
		result.completed = true
		return result, nil
	}

	if err := requireOwnership(ctx, ownership); err != nil {
		return result, err
	}
	_, matched, err := verifyCompactFingerprints(ctx, coordinator.topology)
	if err != nil {
		coordinator.resetEpoch()
		return result, err
	}
	if !matched {
		coordinator.resetEpoch()
		result.state = compactStateDegraded
		result.reason = "global equality fingerprints did not match"
		return result, nil
	}
	if err := writeCompactCompletionMarkers(ctx, coordinator.topology, markers); err != nil {
		return result, err
	}

	compactLogger(ctx).Info("compact uuid storage reached validated completion",
		zap.String("topology", string(coordinator.topology.mode)),
		zap.Int("examined", report.examined))
	result.state = compactStateReady
	result.completed = true
	result.progressed = true
	return result, nil
}
