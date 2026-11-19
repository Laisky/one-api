package model

import (
	"context"
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
//   - abilityCacheUpdateLock serializes the two WRITERS of cached routing state:
//     InitChannelCacheContext (a full rebuild) and SuspendAbility (one published
//     deadline). Without it, a rebuild that read the abilities table just before a
//     suspension was persisted could publish its older snapshot after that
//     suspension, silently dropping it until the next rebuild.
//
// Lock order is abilityCacheUpdateLock, then channelSyncLock. Readers take only
// channelSyncLock. Cooldown deadlines are kept separately from the channel
// snapshot, so an expired deadline restores routing without a rebuild.

// group2model2channels maps group -> model -> enabled channels, sorted by priority
// descending. It includes abilities that are cooling down; selectors filter them
// with cachedAbilitySuspensions at selection time. Guarded by channelSyncLock.
var group2model2channels map[string]map[string][]*Channel

// channelId2channel is the id -> channel snapshot rebuilt by InitChannelCache.
// Guarded by channelSyncLock. It makes channel log enrichment free for code that
// holds only an integer channel id. It contains ENABLED channels only.
var channelId2channel map[int]*Channel

// channelSyncLock guards group2model2channels, channelId2channel and
// cachedAbilitySuspensions.
var channelSyncLock sync.RWMutex

// abilityCacheUpdateLock serializes database suspension writes with cache rebuilds.
// It is separate from channelSyncLock so readers remain available during database I/O.
var abilityCacheUpdateLock sync.Mutex

// cachedAbilitySuspensions stores suspend_until deadlines by exact ability. It is
// guarded by channelSyncLock; expired entries are harmless and are dropped by the
// next rebuild.
var cachedAbilitySuspensions map[cachedAbilityKey]time.Time

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

// cachedAbilityCoolingLocked reports whether the cached deadline for key still
// blocks routing at now. The caller must hold channelSyncLock.
//
// Parameters:
//   - key: the exact (group, model, channel) ability.
//   - now: the UTC selection time.
//
// Return values:
//   - bool: true when a cached deadline exists and has not yet passed.
func cachedAbilityCoolingLocked(key cachedAbilityKey, now time.Time) bool {
	until, suspended := cachedAbilitySuspensions[key]
	return suspended && abilityCoolingAt(until, now)
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

// publishCachedAbilitySuspension makes a persisted suspension visible to cached
// routing immediately. The caller must hold abilityCacheUpdateLock and must call
// it only after the database write succeeded, so the cache never claims a
// cooldown that the database rejected.
//
// Parameters:
//   - key: the exact suspended ability.
//   - until: the persisted UTC deadline.
func publishCachedAbilitySuspension(key cachedAbilityKey, until time.Time) {
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	if cachedAbilitySuspensions == nil {
		cachedAbilitySuspensions = make(map[cachedAbilityKey]time.Time)
	}
	cachedAbilitySuspensions[key] = until
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
		Where(groupCol+" = ? AND "+exactModelPredicate("model")+" AND channel_id = ? AND suspend_until IS NOT NULL AND suspend_until >= ?",
			group, model, channelID, now).
		Count(&count).Error; err != nil {
		return false, errors.Wrapf(err, "check ability cooldown for group %q, model %q, channel %d", group, model, channelID)
	}
	return count > 0, nil
}
