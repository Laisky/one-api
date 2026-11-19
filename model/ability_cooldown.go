package model

import (
	"context"
	"time"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
)

var abilityCooldownLadder = [...]time.Duration{
	10 * time.Second, 30 * time.Second, time.Minute, 3 * time.Minute, 10 * time.Minute,
}

// abilityRoutablePredicate builds the SQL eligibility predicate for a trusted
// optional table prefix. Both bound parameters must receive the same UTC time.
func abilityRoutablePredicate(prefix string) string {
	return "(" + prefix + "suspend_until IS NULL OR " + prefix + "suspend_until < ?) AND (" +
		prefix + "adaptive_suspend_until IS NULL OR " + prefix + "adaptive_suspend_until < ?)"
}

// abilityEffectiveDeadline returns the later of the fixed and adaptive cooldown
// deadlines for the supplied ability, or nil if neither is set.
func abilityEffectiveDeadline(ability *Ability) *time.Time {
	fixed, adaptive := ability.SuspendUntil, ability.AdaptiveSuspendUntil
	if fixed == nil || (adaptive != nil && adaptive.After(*fixed)) {
		return adaptive
	}
	return fixed
}

// exactCooldownAbility selects the ability identified by key without relaxing
// the case-sensitive model comparison. It returns an unexecuted scoped query.
func exactCooldownAbility(db *gorm.DB, key cachedAbilityKey) *gorm.DB {
	groupColumn := "`group`"
	if common.UsingPostgreSQL.Load() {
		groupColumn = `"group"`
	}
	return db.Model(&Ability{}).Where(groupColumn+" = ? AND "+exactModelPredicate("model")+" AND channel_id = ?",
		key.group, key.model, key.channelID)
}

// refreshCachedAbilityDeadline reads the current effective deadline after a
// fixed cooldown update and publishes it under abilityCacheUpdateLock.
func refreshCachedAbilityDeadline(ctx context.Context, key cachedAbilityKey) error {
	var ability Ability
	if err := exactCooldownAbility(DB.WithContext(ctx), key).
		Select("suspend_until", "adaptive_suspend_until").Take(&ability).Error; err != nil {
		return errors.Wrap(err, "read persisted ability cooldown deadline")
	}
	publishAbilityDeadline(key, &ability)
	return nil
}

// publishAbilityDeadline publishes the later deadline, or removes the cache
// entry when both deadlines are absent. The caller holds abilityCacheUpdateLock.
func publishAbilityDeadline(key cachedAbilityKey, ability *Ability) {
	var until time.Time
	if deadline := abilityEffectiveDeadline(ability); deadline != nil {
		until = *deadline
	}
	publishCachedAbilitySuspension(key, until)
}

// RecordAbilityCooldownFailure advances the exact ability's persisted upstream
// failure ladder and publishes its deadline locally. startedAt and completedAt
// are captured around dispatch; a failure already in flight at the time of an
// accepted recovery cannot reopen its cooldown. Returns the rung duration, zero if the
// event was stale or the ability no longer exists, or a persistence error.
func RecordAbilityCooldownFailure(ctx context.Context, group, modelName string, channelID int, startedAt, completedAt time.Time) (time.Duration, error) {
	return recordAbilityCooldownFailure(ctx, cachedAbilityKey{group, modelName, channelID}, startedAt, completedAt)
}

// ResetAbilityCooldown records genuine upstream success for an exact ability.
// startedAt must follow known failures to qualify as recovery; completedAt
// fences delayed and in-flight older attempts. It preserves independent fixed suspensions.
// Returns an error only when the outcome could not be safely persisted.
func ResetAbilityCooldown(ctx context.Context, group, modelName string, channelID int, startedAt, completedAt time.Time) error {
	key := cachedAbilityKey{group, modelName, channelID}
	if err := validateAbilityCooldownOutcome(key, startedAt, completedAt); err != nil {
		return errors.WithStack(err)
	}
	// A single conditional statement avoids a read and a process-wide lock on
	// the healthy hot path. A write is required even without a visible streak:
	// otherwise an earlier delayed failure on another instance can reopen it.
	result := exactCooldownAbility(DB.WithContext(ctx).Session(&gorm.Session{SkipDefaultTransaction: true}), key).
		Where("enabled = ? AND cooldown_recovery < ? AND cooldown_last_failure < ?", true, startedAt.UnixNano(), startedAt.UnixNano()).
		UpdateColumns(map[string]any{
			"cooldown_failures":      0,
			"cooldown_last_failure":  0,
			"cooldown_recovery":      completedAt.UnixNano(),
			"adaptive_suspend_until": nil,
			"cooldown_revision":      gorm.Expr("cooldown_revision + 1"),
		})
	if result.Error != nil {
		return errors.Wrap(result.Error, "persist ability cooldown recovery")
	}
	if result.RowsAffected == 0 {
		return nil
	}
	// Synchronize publication with a rebuild that may have read the pre-reset
	// snapshot. Most successes have no cached cooldown and need no SELECT.
	abilityCacheUpdateLock.Lock()
	defer abilityCacheUpdateLock.Unlock()
	channelSyncLock.RLock()
	_, cached := cachedAbilitySuspensions[key]
	channelSyncLock.RUnlock()
	if !cached {
		return nil
	}
	return refreshCachedAbilityDeadline(ctx, key)
}

// validateAbilityCooldownOutcome checks database initialization, the exact routing
// key, and dispatch timestamps before updating the cooldown. It returns an error
// for missing identity or inconsistent timestamps.
func validateAbilityCooldownOutcome(key cachedAbilityKey, startedAt, completedAt time.Time) error {
	if DB == nil {
		return errors.New("database not initialized")
	}
	if key.group == "" || key.model == "" || key.channelID == 0 {
		return errors.New("group, modelName, and channelId must be specified for recording ability cooldown")
	}
	if startedAt.IsZero() || completedAt.Before(startedAt) {
		return errors.New("invalid ability cooldown outcome timestamps")
	}
	return nil
}

// recordAbilityCooldownFailure atomically applies a timestamped failure to one
// enabled ability. A revision compare-and-swap serializes concurrent writes across
// replicas without backend-specific row locks. Local rebuilds cannot overwrite its
// publication; other replicas still refresh normally.
func recordAbilityCooldownFailure(ctx context.Context, key cachedAbilityKey, startedAt, completedAt time.Time) (time.Duration, error) {
	if err := validateAbilityCooldownOutcome(key, startedAt, completedAt); err != nil {
		return 0, errors.WithStack(err)
	}
	abilityCacheUpdateLock.Lock()
	defer abilityCacheUpdateLock.Unlock()
	start, completion := startedAt.UnixNano(), completedAt.UnixNano()
	db := DB.WithContext(ctx)

	// Bound re-reads in a pathological multi-replica conflict storm.
	for range 16 {
		var ability Ability
		if err := exactCooldownAbility(db, key).Where("enabled = ?", true).Take(&ability).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return 0, nil
			}
			return 0, errors.Wrap(err, "read ability cooldown outcome state")
		}
		if start <= ability.CooldownRecovery {
			return 0, nil
		}
		ability.CooldownFailures = min(ability.CooldownFailures+1, len(abilityCooldownLadder))
		ability.CooldownLastFailure = max(ability.CooldownLastFailure, completion)
		duration := abilityCooldownLadder[ability.CooldownFailures-1]
		deadline := time.Unix(0, ability.CooldownLastFailure).UTC().Add(duration)
		if ability.AdaptiveSuspendUntil == nil || deadline.After(*ability.AdaptiveSuspendUntil) {
			ability.AdaptiveSuspendUntil = &deadline
		}
		result := exactCooldownAbility(db, key).Where("enabled = ? AND cooldown_revision = ?", true, ability.CooldownRevision).
			UpdateColumns(map[string]any{
				"cooldown_failures":      ability.CooldownFailures,
				"cooldown_last_failure":  ability.CooldownLastFailure,
				"adaptive_suspend_until": ability.AdaptiveSuspendUntil,
				"cooldown_revision":      ability.CooldownRevision + 1,
			})
		if result.Error != nil {
			return 0, errors.Wrap(result.Error, "persist ability cooldown outcome")
		}
		if result.RowsAffected == 1 {
			// A concurrent independent fixed write may have updated its column;
			// reread the merged state before publishing this process's snapshot.
			return duration, refreshCachedAbilityDeadline(ctx, key)
		}
	}
	return 0, errors.New("ability cooldown changed repeatedly while recording outcome")
}
