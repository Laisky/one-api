package model

import (
	"context"
	"time"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/common/config"
)

// AbilityRoutable reports whether normal routing could select channelID for the
// exact (group, model) pool right now: the ability row exists, is enabled (which
// mirrors the channel status) and is outside both its fixed and adaptive
// cooldowns. It is the point form of the pooled selectors' eligibility rule and
// reads the same source they do: the routing cache when the memory cache is
// enabled (routableCachedChannelsLocked), otherwise the abilities table
// (availableAbilitiesQuery). Selection paths that bypass the pooled selectors,
// such as Responses-API provider affinity, must use it so a soft preference never
// routes where normal selection would not. The model comparison is
// case-sensitive, exactly as in routing.
//
// Parameters:
//   - ctx: lifecycle scope for the database lookup.
//   - group: the caller's routing group.
//   - model: the caller-facing model name, matched exactly.
//   - channelID: the channel whose ability is checked.
//
// Return values:
//   - bool: true when the ability is routable; false when it is missing,
//     disabled, or cooling down.
//   - error: a wrapped failure on the database path (uninitialized database,
//     cancelled context, or query error); the cache path never fails.
func AbilityRoutable(ctx context.Context, group, model string, channelID int) (bool, error) {
	now := time.Now().UTC()
	if config.MemoryCacheEnabled {
		return cachedAbilityRoutable(group, model, channelID, now), nil
	}
	return databaseAbilityRoutable(ctx, group, model, channelID, now)
}

// cachedAbilityRoutable reports whether channelID is among the routable cached
// channels for (group, model) at now. It reuses the cached selectors' single
// eligibility pass, so the cached pool's membership rules (enabled channel,
// enabled ability) and its cooldown filter apply unchanged.
//
// Parameters:
//   - group, model: the exact routing pool.
//   - channelID: the channel to look for.
//   - now: the UTC selection time.
//
// Return values:
//   - bool: true when the channel is a routable member of the cached pool.
func cachedAbilityRoutable(group, model string, channelID int, now time.Time) bool {
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	candidates, _ := routableCachedChannelsLocked(group, model, nil, now)
	for _, channel := range candidates {
		if channel.Id == channelID {
			return true
		}
	}
	return false
}

// databaseAbilityRoutable reports whether the abilities table holds an enabled,
// non-cooling exact ability for (group, model, channelID) at now. It applies the
// shared availableAbilitiesQuery predicate, so both the fixed suspend_until and
// the adaptive_suspend_until deadlines are honored exactly as by the database
// selectors.
//
// Parameters:
//   - ctx: lifecycle scope for the query; database/sql rejects a context that
//     is already done before it acquires a connection.
//   - group, model: the exact routing pool.
//   - channelID: the channel whose ability is checked.
//   - now: the UTC selection time bound to both cooldown predicates.
//
// Return values:
//   - bool: true when a routable ability row exists.
//   - error: a wrapped failure when the database is uninitialized, the context
//     is done, or the query fails.
func databaseAbilityRoutable(ctx context.Context, group, model string, channelID int, now time.Time) (bool, error) {
	if DB == nil {
		return false, errors.New("database not initialized")
	}
	var count int64
	if err := availableAbilitiesQuery(DB.WithContext(ctx), group, model, now, nil).
		Where("channel_id = ?", channelID).
		Count(&count).Error; err != nil {
		return false, errors.Wrapf(err, "check ability routability for group %q, model %q, channel %d", group, model, channelID)
	}
	return count > 0, nil
}
