package model

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
)

// In-memory routing state and its synchronization contract.
//
// The abilities table is the routing projection; this file owns the process-local
// copy that the cached selectors read. Two locks protect it:
//
//   - channelSyncLock (RWMutex) guards every variable below. Selectors hold it for
//     reading only while they copy candidates out, so database I/O never runs under it.
//   - abilitySnapshotLock serializes the SNAPSHOT publishers with each other:
//     InitChannelCacheContext (a full rebuild) and
//     refreshCachedAbilitySuspensionsContext (the cross-replica cooldown mirror).
//     They hold it across their database read and swap. Cooldown writers never
//     take it, so no cooldown write waits for another ability's database I/O.
//
// Cooldown writers and snapshot publishers are ordered by the versioned
// publication protocol in ability_deadline_publication.go, not by a lock: a
// writer publishes the exact state its own revision CAS committed, and a
// snapshot merges into the cache without discarding anything newer than the
// state it read.
//
// Lock order is abilitySnapshotLock, then channelSyncLock. Readers take only
// channelSyncLock. Cooldown deadlines are kept separately from the channel
// snapshot, so an expired deadline restores routing without a rebuild.
//
// provisionalAbilityCooldowns is an overlay, not persisted state: the relay
// publishes it on the request goroutine (PublishProvisionalAbilityCooldown, which
// takes only channelSyncLock and never waits for a writer) and the asynchronous
// processor releases it once its write finished. Writers never touch it, so a
// rebuild or mirror swap cannot drop it, and the persisted outcome replaces it
// on release in either direction, including when the outcome was discarded.

// group2model2channels maps group -> model -> enabled channels, sorted by priority
// descending. It includes abilities that are cooling down; selectors filter them
// with cachedAbilitySuspensions at selection time. Guarded by channelSyncLock.
var group2model2channels map[string]map[string][]*Channel

// channelId2channel is the id -> channel snapshot rebuilt by InitChannelCache.
// Guarded by channelSyncLock. It makes channel log enrichment free for code that
// holds only an integer channel id. It contains ENABLED channels only.
var channelId2channel map[int]*Channel

// channelSyncLock guards group2model2channels, channelId2channel,
// cachedAbilitySuspensions and provisionalAbilityCooldowns.
var channelSyncLock sync.RWMutex

// abilitySnapshotLock serializes the cache rebuild and the cooldown mirror with
// each other. It is separate from channelSyncLock so readers remain available
// during their database I/O, and cooldown writers never take it.
var abilitySnapshotLock sync.Mutex

// cachedAbilitySuspensions stores, by exact ability, the persisted effective
// deadline (the later of suspend_until and adaptive_suspend_until) together
// with the abilities.cooldown_revision it was committed at. A zero deadline
// records a known lift, so an older publication cannot reopen it. It is guarded
// by channelSyncLock; expired entries are harmless and are dropped by the next
// rebuild or mirror refresh.
var cachedAbilitySuspensions map[cachedAbilityKey]cachedAbilityDeadline

// provisionalAbilityCooldowns holds, by exact ability, the provisional cooldowns
// of failures whose persistence is still pending, one hold per failure. Guarded
// by channelSyncLock. A key is deleted with its last hold.
var provisionalAbilityCooldowns map[cachedAbilityKey][]provisionalAbilityHold

// provisionalAbilityHoldSeq numbers holds so each release removes exactly its own.
// Guarded by channelSyncLock.
var provisionalAbilityHoldSeq uint64

// provisionalAbilityHold is the provisional cooldown of one failure.
type provisionalAbilityHold struct {
	// id identifies the hold for its release.
	id uint64
	// until is the provisional deadline: publication time plus the first rung.
	until time.Time
}

// cachedAbilityKey identifies one exact routing ability without string-key collisions.
type cachedAbilityKey struct {
	group     string
	model     string
	channelID int
}

// abilityCoolingAt reports whether a suspend_until deadline still blocks routing
// at now. It is the in-memory form of the SQL routability predicate
// `suspend_until IS NULL OR suspend_until < now` used by availableAbilitiesQuery,
// so both routing paths agree on the exact boundary.
//
// Parameters:
//   - until: the persisted suspension deadline.
//   - now: the UTC selection time.
//
// Return values:
//   - bool: true while the ability must not be selected.
func abilityCoolingAt(until, now time.Time) bool {
	return !until.Before(now)
}

// cachedAbilityCoolingLocked reports whether the cached persisted deadline or an
// outstanding provisional cooldown for key still blocks routing at now. The
// caller must hold channelSyncLock.
//
// Parameters:
//   - key: the exact (group, model, channel) ability.
//   - now: the UTC selection time.
//
// Return values:
//   - bool: true when either deadline exists and has not yet passed.
func cachedAbilityCoolingLocked(key cachedAbilityKey, now time.Time) bool {
	if cached, suspended := cachedAbilitySuspensions[key]; suspended && abilityCoolingAt(cached.until, now) {
		return true
	}
	if len(provisionalAbilityCooldowns) == 0 {
		return false
	}
	for _, hold := range provisionalAbilityCooldowns[key] {
		if abilityCoolingAt(hold.until, now) {
			return true
		}
	}
	return false
}

// PublishProvisionalAbilityCooldown hides one exact ability from this replica's
// cached routing for the first adaptive rung, synchronously, while the failure
// that cools it is persisted asynchronously. Without it, requests arriving
// between the provider failure and the database write are routed straight back
// to the failing ability. The caller must call the returned release once its
// persistence attempt finished, whatever the outcome; release then removes this
// hold, so the persisted state (published by the writer, or unchanged when the
// outcome was discarded or the write failed) decides routing again. Holds of
// concurrent failures of one ability stack, each with its own deadline, so a
// released hold never leaves its deadline behind. A hold that is never released
// still expires with the first rung. It never waits for database I/O.
//
// It does nothing when the memory cache is disabled (database routing reads
// the persisted row) or the identity is incomplete.
//
// Parameters:
//   - group, model: the exact ability; model is the caller-facing name.
//   - channelID: the channel whose ability failed.
//
// Return values:
//   - func(): the idempotent release of this hold.
//   - time.Duration: the provisional cooldown, zero when nothing was published.
func PublishProvisionalAbilityCooldown(group, model string, channelID int) (func(), time.Duration) {
	if !config.MemoryCacheEnabled || group == "" || model == "" || channelID <= 0 {
		return func() {}, 0
	}
	key := cachedAbilityKey{group, model, channelID}
	duration := abilityCooldownLadder[0]
	until := time.Now().UTC().Add(duration)
	channelSyncLock.Lock()
	if provisionalAbilityCooldowns == nil {
		provisionalAbilityCooldowns = make(map[cachedAbilityKey][]provisionalAbilityHold)
	}
	provisionalAbilityHoldSeq++
	id := provisionalAbilityHoldSeq
	provisionalAbilityCooldowns[key] = append(provisionalAbilityCooldowns[key], provisionalAbilityHold{id: id, until: until})
	channelSyncLock.Unlock()
	return func() { releaseProvisionalAbilityCooldown(key, id) }, duration
}

// releaseProvisionalAbilityCooldown removes the hold id from key's provisional
// cooldowns, deleting the key with its last hold. Releasing a hold that is
// already gone does nothing, so release is idempotent.
//
// Parameters:
//   - key: the exact ability.
//   - id: the hold to remove.
func releaseProvisionalAbilityCooldown(key cachedAbilityKey, id uint64) {
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	holds := provisionalAbilityCooldowns[key]
	index := slices.IndexFunc(holds, func(hold provisionalAbilityHold) bool { return hold.id == id })
	if index < 0 {
		return
	}
	if holds = slices.Delete(holds, index, index+1); len(holds) == 0 {
		delete(provisionalAbilityCooldowns, key)
		return
	}
	provisionalAbilityCooldowns[key] = holds
}

// routableCachedChannelsLocked returns a new slice with the cached channels for
// (group, model) that are outside their cooldown and not in excluded, in cached
// priority order. It is the single eligibility pass shared by every cached
// selector, so callers own the result and may sort or filter it in place without
// another copy. The caller must hold channelSyncLock for reading.
//
// Parameters:
//   - group, model: the exact routing pool.
//   - excluded: channel IDs to omit; nil omits none.
//   - now: the UTC selection time.
//
// Return values:
//   - []*Channel: eligible candidates, possibly empty.
//   - int: how many channels were routable before exclusions were applied, so
//     callers can distinguish an empty pool from an exhausted one.
func routableCachedChannelsLocked(group, model string, excluded map[int]bool, now time.Time) ([]*Channel, int) {
	pool := group2model2channels[group][model]
	candidates := make([]*Channel, 0, len(pool))
	routable := 0
	for _, channel := range pool {
		if cachedAbilityCoolingLocked(cachedAbilityKey{group, model, channel.Id}, now) {
			continue
		}
		routable++
		if !excluded[channel.Id] {
			candidates = append(candidates, channel)
		}
	}
	return candidates, routable
}

// CountCachedRetryCandidates counts the cached channels that could still serve
// (group, model): outside their cooldown and not already excluded. It sizes retry
// budgets without allocating, and it counts candidates directly rather than
// assuming the failed channel is still listed, because a concurrent cooldown may
// already have hidden it.
//
// Parameters:
//   - group, model: the exact routing pool.
//   - excluded: channel IDs already tried by this request.
//
// Return values:
//   - int: the number of remaining eligible channels.
//   - error: non-nil when the memory cache is disabled, so callers keep their
//     database-path fallback.
func CountCachedRetryCandidates(group, model string, excluded map[int]bool) (int, error) {
	if !config.MemoryCacheEnabled {
		return 0, errors.New("MemoryCache is disabled")
	}
	now := time.Now().UTC()
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	count := 0
	for _, channel := range group2model2channels[group][model] {
		if !excluded[channel.Id] && !cachedAbilityCoolingLocked(cachedAbilityKey{group, model, channel.Id}, now) {
			count++
		}
	}
	return count, nil
}

// AbilityCoolingDown reports whether the exact (group, model, channel) ability is
// inside a temporary suspension. It is the shared eligibility predicate for
// selection paths that do not use the pooled selectors, such as provider affinity,
// and it reads the same source as normal routing: the cooldown cache when the
// memory cache is enabled, otherwise the abilities table.
//
// Parameters:
//   - ctx: lifecycle scope for the database lookup.
//   - group, model: the exact ability pool; model is the caller-facing name.
//   - channelID: the channel whose ability is checked.
//
// Return values:
//   - bool: true while the ability is cooling down. A missing ability is not cooling.
//   - error: wrapped database failure on the database path.
func AbilityCoolingDown(ctx context.Context, group, model string, channelID int) (bool, error) {
	now := time.Now().UTC()
	if config.MemoryCacheEnabled {
		channelSyncLock.RLock()
		defer channelSyncLock.RUnlock()
		return cachedAbilityCoolingLocked(cachedAbilityKey{group, model, channelID}, now), nil
	}
	if DB == nil {
		return false, errors.New("database not initialized")
	}
	groupCol := "`group`"
	if common.UsingPostgreSQL.Load() {
		groupCol = `"group"`
	}
	var count int64
	if err := DB.WithContext(ctx).Model(&Ability{}).
		Where(groupCol+" = ? AND "+exactModelPredicate("model")+" AND channel_id = ? ",
			group, model, channelID).
		Where("(suspend_until >= ? OR adaptive_suspend_until >= ?)", now, now).
		Count(&count).Error; err != nil {
		return false, errors.Wrapf(err, "check ability cooldown for group %q, model %q, channel %d", group, model, channelID)
	}
	return count > 0, nil
}
