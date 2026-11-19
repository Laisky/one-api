package model

import (
	"context"
	"encoding/json"
	"slices"
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
// are captured around dispatch. A failure completed before a recovery cannot
// reopen its cooldown. Returns the rung duration, zero if the
// event was stale or the ability no longer exists, or a persistence error.
func RecordAbilityCooldownFailure(ctx context.Context, group, modelName string, channelID int, startedAt, completedAt time.Time) (time.Duration, error) {
	return recordAbilityCooldownFailure(ctx, cachedAbilityKey{group, modelName, channelID}, startedAt, completedAt)
}

// ResetAbilityCooldown records a genuine upstream success for an exact ability.
// Completion order controls recovery; startedAt guards against old configurations.
// Independent fixed suspensions are preserved. Returns a persistence error, if any.
func ResetAbilityCooldown(ctx context.Context, group, modelName string, channelID int, startedAt, completedAt time.Time) error {
	key := cachedAbilityKey{group, modelName, channelID}
	if err := validateAbilityCooldownOutcome(key, startedAt, completedAt); err != nil {
		return errors.WithStack(err)
	}
	start, completion := startedAt.UnixNano(), completedAt.UnixNano()
	// A healthy success changes only the durable ordering watermark, not
	// routing or the local cache. The shared write fences late foreign events.
	result := exactCooldownAbility(cooldownOutcomeDB(ctx), key).
		Where("enabled = ? AND cooldown_config_changed_at < ? AND cooldown_recovery < ? AND cooldown_failures = 0 AND adaptive_suspend_until IS NULL", true, start, completion).
		UpdateColumns(map[string]any{
			"cooldown_recovery": completion,
			"cooldown_revision": gorm.Expr("cooldown_revision + 1"),
		})
	if result.Error != nil {
		return errors.Wrap(result.Error, "persist healthy ability cooldown watermark")
	}
	if result.RowsAffected == 1 {
		return nil
	}
	_, applied, err := applyAbilityCooldownOutcome(ctx, key, start, completion, true)
	if err != nil || !applied {
		return err
	}
	return publishAbilityCooldownRecovery(ctx, key)
}

// cooldownOutcomeDB returns the context-bound database handle for atomic
// single-statement updates without GORM's surrounding default transaction.
func cooldownOutcomeDB(ctx context.Context) *gorm.DB {
	return DB.WithContext(ctx).Session(&gorm.Session{SkipDefaultTransaction: true})
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

// recordAbilityCooldownFailure atomically applies and publishes a failure for
// one currently enabled ability, serializing local publication with rebuilds.
func recordAbilityCooldownFailure(ctx context.Context, key cachedAbilityKey, startedAt, completedAt time.Time) (time.Duration, error) {
	if err := validateAbilityCooldownOutcome(key, startedAt, completedAt); err != nil {
		return 0, errors.WithStack(err)
	}
	abilityCacheUpdateLock.Lock()
	defer abilityCacheUpdateLock.Unlock()
	duration, applied, err := applyAbilityCooldownOutcome(ctx, key, startedAt.UnixNano(), completedAt.UnixNano(), false)
	if err != nil || !applied {
		return duration, err
	}
	return duration, refreshCachedAbilityDeadline(ctx, key)
}

// applyAbilityCooldownOutcome updates one enabled ability with revision CAS.
// A bounded history of the newest failures lets a late success discard failures
// at or before its completion, without discarding later failures. Ties favor
// recovery. Returns rung duration, whether an event was applied, and any error.
func applyAbilityCooldownOutcome(ctx context.Context, key cachedAbilityKey, start, completion int64, success bool) (time.Duration, bool, error) {
	db := cooldownOutcomeDB(ctx)
	for range 16 {
		var ability Ability
		if err := exactCooldownAbility(db, key).
			Select("cooldown_history", "cooldown_config_changed_at", "cooldown_recovery", "cooldown_revision").
			Where("enabled = ? AND cooldown_config_changed_at < ? AND cooldown_recovery < ?", true, start, completion).
			Take(&ability).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return 0, false, nil
			}
			return 0, false, errors.Wrap(err, "read ability cooldown outcome state")
		}
		history, err := abilityFailureHistory(ability.CooldownHistory)
		if err != nil {
			return 0, false, err
		}
		if success {
			ability.CooldownRecovery = completion
			history = slices.DeleteFunc(history, func(at int64) bool { return at <= completion })
		} else {
			if _, found := slices.BinarySearch(history, completion); found {
				return 0, false, nil
			}
			if len(history) == len(abilityCooldownLadder) && completion < history[0] {
				return 0, false, nil
			}
			history = append(history, completion)
			slices.Sort(history)
			if len(history) > len(abilityCooldownLadder) {
				history = history[len(history)-len(abilityCooldownLadder):]
			}
		}
		duration, err := applyAbilityFailureHistory(&ability, history)
		if err != nil {
			return 0, false, err
		}
		result := exactCooldownAbility(db, key).Where("enabled = ? AND cooldown_revision = ? AND cooldown_config_changed_at = ?", true, ability.CooldownRevision, ability.CooldownConfigChangedAt).
			UpdateColumns(map[string]any{
				"cooldown_history":       ability.CooldownHistory,
				"cooldown_failures":      ability.CooldownFailures,
				"cooldown_last_failure":  ability.CooldownLastFailure,
				"cooldown_recovery":      ability.CooldownRecovery,
				"adaptive_suspend_until": ability.AdaptiveSuspendUntil,
				"cooldown_revision":      ability.CooldownRevision + 1,
			})
		if result.Error != nil {
			return 0, false, errors.Wrap(result.Error, "persist ability cooldown outcome")
		}
		if result.RowsAffected == 1 {
			return duration, true, nil
		}
	}
	return 0, false, errors.New("ability cooldown changed repeatedly while recording outcome")
}

// abilityFailureHistory decodes and validates the capped, ascending completion
// timestamps persisted with the ability; an empty value represents no failures.
func abilityFailureHistory(encoded string) ([]int64, error) {
	if encoded == "" {
		return nil, nil
	}
	var history []int64
	if err := json.Unmarshal([]byte(encoded), &history); err != nil {
		return nil, errors.Wrap(err, "decode ability cooldown history")
	}
	if len(history) > len(abilityCooldownLadder) || !slices.IsSorted(history) {
		return nil, errors.New("invalid persisted ability cooldown history")
	}
	return history, nil
}

// applyAbilityFailureHistory replaces the persisted history and derives its
// streak, latest failure and adaptive deadline. It returns the current rung.
func applyAbilityFailureHistory(ability *Ability, history []int64) (time.Duration, error) {
	ability.CooldownFailures = len(history)
	if len(history) == 0 {
		ability.CooldownHistory, ability.CooldownLastFailure, ability.AdaptiveSuspendUntil = "", 0, nil
		return 0, nil
	}
	encoded, err := json.Marshal(history)
	if err != nil {
		return 0, errors.Wrap(err, "encode ability cooldown history")
	}
	ability.CooldownHistory = string(encoded)
	ability.CooldownLastFailure = history[len(history)-1]
	duration := abilityCooldownLadder[len(history)-1]
	deadline := time.Unix(0, ability.CooldownLastFailure).UTC().Add(duration)
	ability.AdaptiveSuspendUntil = &deadline
	return duration, nil
}
