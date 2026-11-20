package model

import "time"

// Versioned publication of persisted cooldown deadlines into cached routing.
//
// The abilities row is the source of truth and abilities.cooldown_revision
// orders its committed states: every statement that changes suspend_until or
// adaptive_suspend_until also increments the revision (the adaptive CAS,
// SuspendAbility's CAS, the upstream-change lift and the re-enable restart).
// Row locking serializes those statements, so for one ability a higher
// revision is always a later committed state.
//
// Two kinds of publishers fill cachedAbilitySuspensions, and neither holds a
// lock across database I/O that the other kind waits for:
//
//   - A cooldown writer (adaptive failure or recovery, fixed suspension)
//     publishes the exact state its own revision CAS committed, with that
//     revision, straight after the commit. It needs no read-back, so a
//     committed cooldown cannot be lost to a failed read, and an older
//     publication never replaces a newer one, whatever the goroutine order.
//   - A snapshot (cache rebuild or cooldown mirror) reads the table and merges
//     the result. An entry published before the read began is always replaced,
//     because its commit preceded the read. An entry published after the read
//     began may be newer than the snapshot: for an ability the snapshot
//     observed, the higher revision wins; for an ability it did not observe (no
//     active deadline at read time), the entry is kept, because the snapshot
//     cannot speak for it. Comparing revisions only for concurrent publications
//     also keeps a deleted and recreated ability row, whose revision restarts at
//     zero, from being shadowed by its predecessor's state.

// cachedAbilityDeadline is one ability's cached persisted cooldown.
type cachedAbilityDeadline struct {
	// until is the effective deadline; zero records a known lift.
	until time.Time
	// revision is the abilities.cooldown_revision the state was committed at.
	revision int64
	// seq is the publication sequence at which this process learned the state:
	// the writer's publication, or the start of the snapshot read.
	seq uint64
}

// abilityPublicationSeq orders this process's deadline publications and
// snapshot reads. Guarded by channelSyncLock.
var abilityPublicationSeq uint64

// publishCommittedAbilityDeadline publishes the cooldown state a writer's
// revision CAS committed for key, unless the cache already holds that revision
// or a later one. The caller must have committed exactly this state at exactly
// this revision, so the cache never claims a cooldown the database rejected.
//
// Parameters:
//   - key: the exact ability.
//   - fixed, adaptive: the committed suspend_until and adaptive_suspend_until.
//   - revision: the committed cooldown_revision.
func publishCommittedAbilityDeadline(key cachedAbilityKey, fixed, adaptive *time.Time, revision int64) {
	var until time.Time
	if deadline := abilityEffectiveDeadline(&Ability{SuspendUntil: fixed, AdaptiveSuspendUntil: adaptive}); deadline != nil {
		until = *deadline
	}
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	if current, ok := cachedAbilitySuspensions[key]; ok && current.revision >= revision {
		return
	}
	if cachedAbilitySuspensions == nil {
		cachedAbilitySuspensions = make(map[cachedAbilityKey]cachedAbilityDeadline)
	}
	abilityPublicationSeq++
	cachedAbilitySuspensions[key] = cachedAbilityDeadline{until: until, revision: revision, seq: abilityPublicationSeq}
}

// beginAbilityDeadlineSnapshot marks the start of a snapshot read. The caller
// must issue the read only after this returns.
//
// Returns:
//   - uint64: the snapshot's sequence, passed to mergeAbilityDeadlineSnapshotLocked.
func beginAbilityDeadlineSnapshot() uint64 {
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	abilityPublicationSeq++
	return abilityPublicationSeq
}

// mergeAbilityDeadlineSnapshotLocked returns the cache that replaces
// cachedAbilitySuspensions after a snapshot read that began at startSeq.
// observed holds every ability whose row the snapshot read, with its effective
// deadline (zero when none) and revision; a missing ability had no active
// deadline at read time. The caller holds channelSyncLock for writing.
//
// Parameters:
//   - observed: the snapshot's rows by exact ability.
//   - startSeq: the value beginAbilityDeadlineSnapshot returned before the read.
//
// Returns:
//   - map[cachedAbilityKey]cachedAbilityDeadline: the merged cache.
func mergeAbilityDeadlineSnapshotLocked(observed map[cachedAbilityKey]cachedAbilityDeadline, startSeq uint64) map[cachedAbilityKey]cachedAbilityDeadline {
	next := make(map[cachedAbilityKey]cachedAbilityDeadline, len(cachedAbilitySuspensions))
	for key, snapshot := range observed {
		current, cached := cachedAbilitySuspensions[key]
		switch {
		case cached && current.seq > startSeq && current.revision > snapshot.revision:
			// A writer committed and published a later state after the read.
			next[key] = current
		case !snapshot.until.IsZero() || cached:
			// Keep a lift as a tombstone only where the cache held the ability,
			// so a delayed older publication cannot reopen it.
			snapshot.seq = startSeq
			next[key] = snapshot
		}
	}
	for key, current := range cachedAbilitySuspensions {
		if _, seen := observed[key]; !seen && current.seq > startSeq {
			next[key] = current
		}
	}
	return next
}
