package model

import (
	"context"
	"sync"

	"github.com/Laisky/zap"
)

// compactRowRepairQueueSize bounds the read-path repair requests waiting for the worker.
//
// A full queue drops the latency hint, which is safe because the bounded master-side equality
// sweep remains the cross-process correctness and eventual-repair mechanism.
const compactRowRepairQueueSize = 256

// compactRowRepairRequest names one row the read path proved has a missing or wrong shadow.
type compactRowRepairRequest struct {
	// target is the owned-UUID registry target the lookup used.
	target compactTarget
	// id is the row's primary key.
	id int64
}

// compactRowRepairKey identifies one deduplicated read-path repair request.
type compactRowRepairKey struct {
	role   uuidDBRole
	target string
	id     int64
}

// compactRowRepairQueue carries bounded, deduplicated read-path evidence to the local worker.
// The master-side rolling audit remains the cross-process correctness source; this queue is the
// low-latency path for mismatches observed in the same process as the mutating worker.
var compactRowRepairQueue = struct {
	sync.Mutex
	requests []compactRowRepairRequest
	queued   map[compactRowRepairKey]struct{}
}{
	requests: make([]compactRowRepairRequest, 0, compactRowRepairQueueSize),
	queued:   map[compactRowRepairKey]struct{}{},
}

// enqueueCompactRowRepair hands one proven row to the worker without ever blocking the request.
// Parameters:
//   - target: owned-UUID registry target the lookup used.
//   - id: primary key of the row whose shadow is missing or wrong.
//
// Return values: none.
func enqueueCompactRowRepair(target compactTarget, id int64) {
	if id <= 0 {
		return
	}
	key := compactRowRepairKey{role: target.role, target: target.id(), id: id}
	compactRowRepairQueue.Lock()
	defer compactRowRepairQueue.Unlock()
	if _, exists := compactRowRepairQueue.queued[key]; exists {
		return
	}
	if len(compactRowRepairQueue.requests) >= compactRowRepairQueueSize {
		return
	}
	compactRowRepairQueue.requests = append(compactRowRepairQueue.requests,
		compactRowRepairRequest{target: target, id: id})
	compactRowRepairQueue.queued[key] = struct{}{}
}

// drainCompactRowRepairs takes every queued request without blocking.
// Parameters: none.
//
// Return values:
//   - []compactRowRepairRequest: requests queued since the last drain, in arrival order.
func drainCompactRowRepairs() []compactRowRepairRequest {
	compactRowRepairQueue.Lock()
	defer compactRowRepairQueue.Unlock()
	requests := append([]compactRowRepairRequest(nil), compactRowRepairQueue.requests...)
	compactRowRepairQueue.requests = compactRowRepairQueue.requests[:0]
	clear(compactRowRepairQueue.queued)
	return requests
}

// requeueCompactRowRepairs returns unprocessed evidence to the bounded local queue.
// Parameters:
//   - requests: requests that did not reach a terminal repair outcome.
//
// Return values: none.
func requeueCompactRowRepairs(requests []compactRowRepairRequest) {
	compactRowRepairQueue.Lock()
	defer compactRowRepairQueue.Unlock()
	combined := append(append([]compactRowRepairRequest(nil), requests...), compactRowRepairQueue.requests...)
	compactRowRepairQueue.requests = compactRowRepairQueue.requests[:0]
	clear(compactRowRepairQueue.queued)
	for _, request := range combined {
		key := compactRowRepairKey{role: request.target.role, target: request.target.id(), id: request.id}
		if _, exists := compactRowRepairQueue.queued[key]; exists {
			continue
		}
		if len(compactRowRepairQueue.requests) >= compactRowRepairQueueSize {
			break
		}
		compactRowRepairQueue.requests = append(compactRowRepairQueue.requests, request)
		compactRowRepairQueue.queued[key] = struct{}{}
	}
}

// runCompactSteadyState is the completed worker's cycle.
//
// External UUIDs are a one-time migration. Once the historical rows are derived and the markers
// written, every write derives its own shadow in the database, through the trigger, in the same
// statement; and every compact read is verified against its authoritative text, so a wrong shadow
// can cost a fallback to the text index but never a wrong answer. What can still go wrong is that
// the machinery stops working, and that is what this cycle checks, at a cost that does not grow
// with the number of rows:
//
//  1. Object metadata: shadow columns, triggers — including whether PostgreSQL has them
//     enabled — indexes, and the legacy index manifest. Catalog reads only.
//  2. Rows the read path proved wrong, repaired one by one through the primary key.
//  3. One gap probe per owned target. An owned UUID is never legitimately NULL, so on a clean
//     table the probe is a single seek into an empty index range. It is what notices rows that
//     bypassed the trigger — the supported `pg_dump --data-only --disable-triggers` restore
//     re-enables the triggers afterwards, so the catalog alone would never see it — because such
//     a row leaves its owned shadow NULL along with every other.
//  4. One bounded equality page per target. This is the eventual-detection backstop for
//     non-NULL drift, nullable foreign-key drift, and evidence observed on another process.
//
// There is no post-completion full traversal. Each equality page is bounded by the configured
// batch and global row/time budgets, so routine cycle cost remains independent of table size.
//
// Evidence of drift the catalog or an owned probe can see sets fullAuditRequired, and the next
// cycle takes the recovery path: recreate objects, repair in id order, then two clean full passes
// before ready. That traversal is the one-time price of an abnormal event, never a routine one.
// Parameters:
//   - ctx: context bounding the cycle.
//   - coordinator: worker state carried across cycles.
//   - ownership: ownership claim for the cycle.
//
// Return values:
//   - compactCycleResult: aggregate state and counts.
//   - error: wrapped error for a transient failure.
func runCompactSteadyState(ctx context.Context, coordinator *compactCoordinator,
	ownership *compactOwnership) (compactCycleResult, error) {
	topology := coordinator.topology
	result := compactCycleResult{state: compactStateReady}

	verified, reason, err := validateCompactObjects(ctx, topology)
	if err != nil {
		return result, err
	}
	if !verified {
		// Object drift. Rows written while an object was missing or disabled may disagree, so
		// only a full audit may follow.
		if err := requireOwnership(ctx, ownership); err != nil {
			return result, err
		}
		if err := coordinator.persistFullAuditRequired(ctx); err != nil {
			return result, err
		}
		result.state = compactStateDegraded
		result.reason = reason
		signalCompactRepair()
		return result, nil
	}

	if err := repairCompactRowsFromLookups(ctx, coordinator, ownership, &result); err != nil {
		return result, err
	}
	if result.state != compactStateReady {
		return result, nil
	}

	for _, target := range compactTargetsForTopology(topology) {
		if target.kind != compactKindOwned {
			continue
		}
		if err := requireOwnership(ctx, ownership); err != nil {
			return result, err
		}
		gap, err := compactTargetHasGap(ctx, topology.handle(target.role), target)
		if err != nil {
			return result, err
		}
		if gap {
			if err := requireOwnership(ctx, ownership); err != nil {
				return result, err
			}
			if err := coordinator.persistFullAuditRequired(ctx); err != nil {
				return result, err
			}
			result.state = compactStateDegraded
			result.reason = "compact NULL backlog found for " + target.id()
			signalCompactRepair()
			return result, nil
		}
	}

	if err := reconcileCompactSteadySweep(ctx, coordinator, ownership, &result); err != nil {
		return result, err
	}
	if result.state != compactStateReady {
		return result, nil
	}

	result.completed = true
	return result, nil
}

// repairCompactRowsFromLookups repairs the rows the read path proved wrong.
//
// Each request is one primary-key read and at most one conditional single-row update, so the
// work is proportional to the evidence, never to the table. Requests are processed twice when a
// unique collision gets in the way: a lookup that found row B holding row A's identifier queues
// B before A, and if A is attempted first it collides with B's stale shadow, which B's own
// repair then clears.
// Parameters:
//   - ctx: context bounding the repairs.
//   - coordinator: worker state carried across cycles.
//   - ownership: ownership claim for the cycle.
//   - result: cycle result to accumulate counts into.
//
// Return values:
//   - error: wrapped error when a read or repair fails.
func repairCompactRowsFromLookups(ctx context.Context, coordinator *compactCoordinator,
	ownership *compactOwnership, result *compactCycleResult) error {
	pending := drainCompactRowRepairs()
	if len(pending) == 0 {
		return nil
	}
	if err := requireOwnership(ctx, ownership); err != nil {
		requeueCompactRowRepairs(pending)
		return err
	}
	rowCtx, cancel := context.WithTimeout(ctx, compactCycleDuration())
	defer cancel()
	budget := compactRowBudget()
	for attempt := 0; attempt < 2 && len(pending) > 0; attempt++ {
		retry := []compactRowRepairRequest{}
		for index, request := range pending {
			if result.examined >= budget {
				requeueCompactRowRepairs(append(retry, pending[index:]...))
				signalCompactRepair()
				return nil
			}
			if err := requireOwnership(rowCtx, ownership); err != nil {
				requeueCompactRowRepairs(append(retry, pending[index:]...))
				return err
			}
			db := coordinator.topology.handle(request.target.role)
			if db == nil {
				continue
			}
			candidates, err := readCompactCandidateByID(rowCtx, db, request.target, request.id)
			if err != nil {
				requeueCompactRowRepairs(append(retry, pending[index:]...))
				return err
			}
			if len(candidates) == 0 {
				continue
			}
			if !compactCandidatesNeedAttention(request.target, candidates) {
				continue
			}
			if result.state == compactStateReady {
				if err := requireOwnership(rowCtx, ownership); err != nil {
					requeueCompactRowRepairs(append(retry, pending[index:]...))
					return err
				}
				if err := coordinator.persistFullAuditRequired(rowCtx); err != nil {
					requeueCompactRowRepairs(append(retry, pending[index:]...))
					return err
				}
				result.state = compactStateDegraded
				result.reason = "compact lookup found shadow drift; full audit required"
			}
			if err := requireOwnership(rowCtx, ownership); err != nil {
				requeueCompactRowRepairs(append(retry, pending[index:]...))
				return err
			}
			progress := compactTargetProgress{}
			cursor := int64(0)
			if err := reconcileCompactBatch(rowCtx, db, request.target, candidates, &progress, &cursor); err != nil {
				requeueCompactRowRepairs(append(retry, pending[index:]...))
				return err
			}
			result.examined += progress.examined
			result.updated += progress.updated
			result.blockers += progress.blockers
			if progress.collisions > 0 {
				retry = append(retry, request)
			}
		}
		pending = retry
	}
	if len(pending) > 0 {
		result.blockers += len(pending)
		result.state = compactStateBlockedValidation
		result.reason = "compact uniqueness permutation found during targeted repair"
		return nil
	}
	if result.blockers > 0 {
		result.state = compactStateBlockedValidation
		result.reason = "invalid authoritative data found during targeted repair"
		return nil
	}
	if result.updated > 0 {
		// Worth knowing, not worth a scan: rows only disagree after something bypassed the
		// trigger, and the reads that found them were answered correctly all along.
		compactLogger(ctx).Warn("compact uuid repaired shadows the read path found wrong",
			zap.Int("repaired_rows", result.updated))
		result.reason = "compact shadow drift repaired from lookup evidence; full audit required"
		result.progressed = true
	}
	return nil
}

// reconcileCompactSteadySweep advances bounded equality pages under the shared cycle budget.
// The starting target rotates each cycle so a minimum configured budget cannot starve targets
// that sort later in the registry.
// Parameters:
//   - ctx: context bounding steady-state row reads and repairs.
//   - coordinator: worker state carrying per-target sweep cursors.
//   - ownership: ownership claim held by the mutating worker.
//   - result: cycle result to accumulate into.
//
// Return values:
//   - error: wrapped error when a bounded read or repair fails.
func reconcileCompactSteadySweep(ctx context.Context, coordinator *compactCoordinator,
	ownership *compactOwnership, result *compactCycleResult) error {
	rowCtx, cancel := context.WithTimeout(ctx, compactCycleDuration())
	defer cancel()
	budget := compactRowBudget() - result.examined
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
			return err
		}
		cursor := coordinator.cursors[target.id()]
		batch := compactBatchRows(target)
		if batch > budget {
			batch = budget
		}
		candidates, err := readCompactCandidates(rowCtx,
			coordinator.topology.handle(target.role), target, cursor.sweep, batch)
		if err != nil {
			return err
		}
		if len(candidates) == 0 && cursor.sweep != 0 {
			// The sweep reached the end of the table. Rewind and read the first page in the
			// same cycle: otherwise a table no larger than one page is examined only on every
			// other cycle, and drift behind the cursor on any table waits one cycle longer
			// than the budget requires.
			cursor.sweep = 0
			candidates, err = readCompactCandidates(rowCtx,
				coordinator.topology.handle(target.role), target, 0, batch)
			if err != nil {
				return err
			}
		}
		if len(candidates) == 0 {
			cursor.sweep = 0
			coordinator.cursors[target.id()] = cursor
			continue
		}
		progress := compactTargetProgress{}
		if compactCandidatesNeedAttention(target, candidates) {
			if err := requireOwnership(rowCtx, ownership); err != nil {
				return err
			}
			if err := coordinator.persistFullAuditRequired(ctx); err != nil {
				return err
			}
		}
		if err := requireOwnership(rowCtx, ownership); err != nil {
			return err
		}
		if err := reconcileCompactBatch(rowCtx, coordinator.topology.handle(target.role), target,
			candidates, &progress, &cursor.sweep); err != nil {
			return err
		}
		coordinator.cursors[target.id()] = cursor
		result.examined += progress.examined
		result.updated += progress.updated
		result.blockers += progress.blockers
		budget -= progress.examined
		if progress.collisions > 0 {
			result.blockers += progress.collisions
			result.state = compactStateBlockedValidation
			result.reason = "compact uniqueness permutation for " + target.id()
			return coordinator.persistFullAuditRequired(ctx)
		}
		if progress.blockers > 0 {
			result.state = compactStateBlockedValidation
			result.reason = "invalid authoritative data found during steady-state audit for " + target.id()
			return coordinator.persistFullAuditRequired(ctx)
		}
		if progress.updated > 0 {
			result.state = compactStateDegraded
			result.reason = "compact shadow drift repaired during bounded steady-state audit"
			result.progressed = true
			return coordinator.persistFullAuditRequired(ctx)
		}
	}
	return nil
}

// compactCandidatesNeedAttention reports whether a page contains drift or authoritative blockers.
// Parameters:
//   - target: registry target whose derivation rules classify the page.
//   - candidates: bounded row observations to inspect without mutation.
//
// Return values:
//   - bool: true when durable audit-required state must precede reconciliation.
func compactCandidatesNeedAttention(target compactTarget, candidates []compactCandidate) bool {
	for _, candidate := range candidates {
		classification, _ := classifyCompactRow(target, candidate)
		if classification != compactRowValid {
			return true
		}
	}
	return false
}
