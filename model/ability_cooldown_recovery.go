package model

import (
	"context"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/graceful"
)

// abilityRecoveryFlushInterval bounds how often one process durably writes the
// recovery of one ability. Every success of a popular model hits the same
// ability row, so a write per success serializes on that row lock (measured at
// roughly 700 successes/s on MySQL 8.4 and 2,300/s on PostgreSQL 17 for one row,
// adding tens of milliseconds of lock wait). One write per interval keeps the
// completion-order watermark fresh while bounding database pressure.
const abilityRecoveryFlushInterval = time.Second

// abilityRecoveryStateLimit bounds the per-process recovery bookkeeping; idle
// entries are pruned when it is exceeded.
const abilityRecoveryStateLimit = 4096

// abilityRecovery is one provider-backed success in UTC Unix nanoseconds and
// the upstream revision of the channel snapshot it was routed with.
type abilityRecovery struct {
	start, completion int64
	revision          int64
}

// abilityRecoveryKey partitions coalescing by ability and by the upstream
// revision the successes were routed with. Outcomes of different revisions
// never stand in for each other: a success routed with a replaced key or
// endpoint must neither discard a failure of the current upstream nor replace
// a pending recovery of it (its own durable write is fenced and would be lost).
// AbilityUpstreamRevisionUnknown is its own partition, so an unknown-revision
// outcome never supersedes a known-revision one or vice versa (the conservative
// direction). A replaced revision's idle partition lingers until the bounded
// prune at abilityRecoveryStateLimit entries.
type abilityRecoveryKey struct {
	ability  cachedAbilityKey
	revision int64
}

// abilityRecoveryState coalesces the successes of one ability and upstream
// revision in this process.
type abilityRecoveryState struct {
	// pending is the newest success not yet handed to a durable write.
	pending abilityRecovery
	// inflight is the success a running flush is writing.
	inflight abilityRecovery
	// flushing is true while a flush task owns the durable write.
	flushing bool
	// notBefore is the earliest time of the next durable write (monotonic).
	notBefore time.Time
}

// abilityRecoveries holds the per-process recovery coalescing state.
var abilityRecoveries = struct {
	sync.Mutex
	byKey map[abilityRecoveryKey]*abilityRecoveryState
}{byKey: make(map[abilityRecoveryKey]*abilityRecoveryState)}

// QueueAbilityCooldownRecovery records a genuine provider success for an exact
// ability without database I/O on the caller's path. The first success after
// idle, or after this process recorded a failure for the ability, is written at
// once by a graceful critical task; later successes within
// abilityRecoveryFlushInterval are coalesced into the next write. Completion
// order still decides recovery (ResetAbilityCooldown), and a failure this
// process persists after an unwritten newer success is discarded locally.
//
// Trade-off: a success coalesced inside the interval and never followed by
// another success is not written, so a streak recorded meanwhile by another
// process is cleared by the next success after the interval instead.
//
// Parameters:
//   - ctx: a detached context carrying the request-scoped logger.
//   - group, modelName, channelID: the routed ability key.
//   - upstreamRevision: the routed channel snapshot's UpstreamRevision, or
//     AbilityUpstreamRevisionUnknown.
//   - startedAt, completedAt: the successful attempt's dispatch and completion.
//
// Returns:
//   - error: invalid identity or timestamps; persistence errors are logged by
//     the flush task, never returned.
func QueueAbilityCooldownRecovery(ctx context.Context, group, modelName string, channelID int, upstreamRevision int64, startedAt, completedAt time.Time) error {
	ability := cachedAbilityKey{group, modelName, channelID}
	if err := validateAbilityCooldownOutcome(ability, startedAt, completedAt); err != nil {
		return errors.WithStack(err)
	}
	key := abilityRecoveryKey{ability: ability, revision: upstreamRevision}
	recovery := abilityRecovery{start: startedAt.UnixNano(), completion: completedAt.UnixNano(), revision: upstreamRevision}
	now := time.Now()
	abilityRecoveries.Lock()
	state := abilityRecoveries.byKey[key]
	if state == nil {
		pruneAbilityRecoveriesLocked(now)
		state = &abilityRecoveryState{}
		abilityRecoveries.byKey[key] = state
	}
	if recovery.completion > state.pending.completion {
		state.pending = recovery
	}
	if state.flushing || now.Before(state.notBefore) {
		abilityRecoveries.Unlock()
		return nil
	}
	state.flushing = true
	abilityRecoveries.Unlock()
	graceful.GoCritical(ctx, "abilityCooldownRecovery", func(ctx context.Context) {
		flushAbilityRecovery(ctx, key)
	})
	return nil
}

// flushAbilityRecovery durably writes the newest coalesced success of key and
// then arms the rate limit. It runs as a graceful critical task, so shutdown
// drains it. Persistence errors are logged here and not returned.
//
// Parameters:
//   - ctx: a detached context carrying the request-scoped logger.
//   - key: the ability whose pending success is written.
func flushAbilityRecovery(ctx context.Context, key abilityRecoveryKey) {
	lg := gmw.GetLogger(ctx)
	abilityRecoveries.Lock()
	state := abilityRecoveries.byKey[key]
	if state == nil {
		abilityRecoveries.Unlock()
		return
	}
	recovery := state.pending
	state.pending, state.inflight = abilityRecovery{}, recovery
	abilityRecoveries.Unlock()

	if recovery.completion != 0 {
		if err := ResetAbilityCooldownAtRevision(ctx, key.ability.group, key.ability.model, key.ability.channelID, recovery.revision,
			time.Unix(0, recovery.start).UTC(), time.Unix(0, recovery.completion).UTC()); err != nil {
			lg.Warn("failed to record upstream cooldown recovery", zap.Error(err))
		}
	}

	abilityRecoveries.Lock()
	state.inflight = abilityRecovery{}
	state.flushing = false
	state.notBefore = time.Now().Add(abilityRecoveryFlushInterval)
	abilityRecoveries.Unlock()
}

// abilityRecoverySupersedes reports whether this process holds an unwritten or
// in-flight success of the same ability AND upstream revision that completed at
// or after completion, so a failure that completed earlier is stale and must
// not open a cooldown. A success routed with another revision never supersedes.
//
// Parameters:
//   - ability: the ability of the failure.
//   - revision: the failure's routed upstream revision.
//   - completion: the failure's completion in UTC Unix nanoseconds.
//
// Returns:
//   - bool: true when a newer local success of the same revision supersedes it.
func abilityRecoverySupersedes(ability cachedAbilityKey, revision, completion int64) bool {
	abilityRecoveries.Lock()
	defer abilityRecoveries.Unlock()
	state := abilityRecoveries.byKey[abilityRecoveryKey{ability: ability, revision: revision}]
	return state != nil && (state.pending.completion >= completion || state.inflight.completion >= completion)
}

// rearmAbilityRecovery lets the next success of the ability, at any upstream
// revision, write immediately because this process just recorded a failure for
// it; recovery must not wait for the rate limit then. Re-arming every revision
// is safe: it only lifts the write rate limit, a success of a replaced revision
// is still fenced by its durable write, and supersession stays per revision.
//
// Parameters:
//   - ability: the ability that failed.
func rearmAbilityRecovery(ability cachedAbilityKey) {
	abilityRecoveries.Lock()
	defer abilityRecoveries.Unlock()
	for key, state := range abilityRecoveries.byKey {
		if key.ability == ability {
			state.notBefore = time.Time{}
		}
	}
}

// pruneAbilityRecoveriesLocked drops idle entries whose rate limit has passed
// once the bookkeeping exceeds abilityRecoveryStateLimit. The caller holds the
// abilityRecoveries lock.
//
// Parameters:
//   - now: the current monotonic time.
func pruneAbilityRecoveriesLocked(now time.Time) {
	if len(abilityRecoveries.byKey) < abilityRecoveryStateLimit {
		return
	}
	for key, state := range abilityRecoveries.byKey {
		if !state.flushing && state.pending.completion == 0 && !now.Before(state.notBefore) {
			delete(abilityRecoveries.byKey, key)
		}
	}
}
