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

// abilityCooldownCASAttempts bounds the read-compare-write retries of one
// cooldown write against concurrent writers of the same ability.
const abilityCooldownCASAttempts = 16

// AbilityUpstreamRevisionUnknown disables the upstream-revision fence for an
// outcome whose routed channel snapshot is unknown (tests, administrative
// tooling); the time-based configuration fence still applies.
const AbilityUpstreamRevisionUnknown int64 = -1

// RecordAbilityCooldownFailure advances the exact ability's persisted upstream
// failure ladder without an upstream-revision fence. See
// RecordAbilityCooldownFailureAtRevision.
func RecordAbilityCooldownFailure(ctx context.Context, group, modelName string, channelID int, startedAt, completedAt time.Time) (time.Duration, error) {
	return recordAbilityCooldownFailure(ctx, cachedAbilityKey{group, modelName, channelID}, AbilityUpstreamRevisionUnknown, startedAt, completedAt)
}

// RecordAbilityCooldownFailureAtRevision advances the exact ability's persisted
// upstream failure ladder and publishes its deadline locally. upstreamRevision
// is the routed channel snapshot's UpstreamRevision; an attempt routed with an
// older snapshot (a replica whose cache still holds a replaced key or endpoint)
// is discarded. startedAt and completedAt are captured around dispatch. A
// failure completed before a recovery cannot reopen its cooldown. Returns the
// rung duration, zero if the event was stale or the ability no longer exists,
// or a persistence error.
func RecordAbilityCooldownFailureAtRevision(ctx context.Context, group, modelName string, channelID int, upstreamRevision int64, startedAt, completedAt time.Time) (time.Duration, error) {
	return recordAbilityCooldownFailure(ctx, cachedAbilityKey{group, modelName, channelID}, upstreamRevision, startedAt, completedAt)
}

// ResetAbilityCooldown records a genuine upstream success for an exact ability
// without an upstream-revision fence. See ResetAbilityCooldownAtRevision.
func ResetAbilityCooldown(ctx context.Context, group, modelName string, channelID int, startedAt, completedAt time.Time) error {
	return ResetAbilityCooldownAtRevision(ctx, group, modelName, channelID, AbilityUpstreamRevisionUnknown, startedAt, completedAt)
}

// ResetAbilityCooldownAtRevision records a genuine upstream success for an
// exact ability. Completion order controls recovery; startedAt and
// upstreamRevision (the routed channel snapshot's UpstreamRevision, or
// AbilityUpstreamRevisionUnknown) guard against old configurations.
// Independent fixed suspensions are preserved. Returns a persistence error, if any.
func ResetAbilityCooldownAtRevision(ctx context.Context, group, modelName string, channelID int, upstreamRevision int64, startedAt, completedAt time.Time) error {
	key := cachedAbilityKey{group, modelName, channelID}
	if err := validateAbilityCooldownOutcome(key, startedAt, completedAt); err != nil {
		return errors.WithStack(err)
	}
	start, completion := startedAt.UnixNano(), completedAt.UnixNano()
	// A healthy success changes only the durable ordering watermark, not
	// routing or the local cache. The shared write fences late foreign events.
	result := currentUpstreamAbility(exactCooldownAbility(cooldownOutcomeDB(ctx), key), key, upstreamRevision).
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
	// A recovery that changed the cooldown publishes its committed state itself.
	_, _, err := applyAbilityCooldownOutcome(ctx, key, upstreamRevision, start, completion, true)
	return err
}

// currentUpstreamAbility narrows an ability query to rows whose channel still
// has the routed upstream revision, so an outcome measured against a replaced
// key or endpoint matches nothing. AbilityUpstreamRevisionUnknown (or any
// negative revision) leaves the query unchanged.
//
// Parameters:
//   - query: the scoped ability query.
//   - key: the ability key; its channel id selects the channel row.
//   - upstreamRevision: the routed channel snapshot's revision.
//
// Returns:
//   - *gorm.DB: the narrowed, unexecuted query.
func currentUpstreamAbility(query *gorm.DB, key cachedAbilityKey, upstreamRevision int64) *gorm.DB {
	if upstreamRevision < 0 {
		return query
	}
	return query.Where("EXISTS (SELECT 1 FROM channels WHERE channels.id = ? AND channels.upstream_revision = ?)", key.channelID, upstreamRevision)
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
// one currently enabled ability. It holds no lock across database I/O; the
// committed state is published by applyAbilityCooldownOutcome.
func recordAbilityCooldownFailure(ctx context.Context, key cachedAbilityKey, upstreamRevision int64, startedAt, completedAt time.Time) (time.Duration, error) {
	if err := validateAbilityCooldownOutcome(key, startedAt, completedAt); err != nil {
		return 0, errors.WithStack(err)
	}
	if abilityRecoverySupersedes(key, upstreamRevision, completedAt.UnixNano()) {
		// A newer success of this process is still being coalesced; once written
		// it would discard this failure anyway, so do not open a cooldown now.
		return 0, nil
	}
	duration, applied, err := applyAbilityCooldownOutcome(ctx, key, upstreamRevision, startedAt.UnixNano(), completedAt.UnixNano(), false)
	if err != nil || !applied {
		return duration, err
	}
	rearmAbilityRecovery(key)
	return duration, nil
}

// applyAbilityCooldownOutcome updates one enabled ability with revision CAS.
// The bounded history holds one completion time per failure episode (rung). A
// late success discards episodes that completed at or before it, without
// discarding later ones; ties favor recovery. See mergeAbilityFailureEpisode for
// how a failure joins or opens an episode. upstreamRevision fences outcomes of
// attempts routed with a replaced upstream (see currentUpstreamAbility). An
// applied outcome publishes the committed state, both deadlines at the new
// revision, to cached routing before returning (publishCommittedAbilityDeadline);
// the CAS on cooldown_revision proves suspend_until is unchanged since the read,
// because every deadline write increments the revision. Returns the rung
// duration in force, whether an event was applied, and any error.
func applyAbilityCooldownOutcome(ctx context.Context, key cachedAbilityKey, upstreamRevision int64, start, completion int64, success bool) (time.Duration, bool, error) {
	db := cooldownOutcomeDB(ctx)
	for range abilityCooldownCASAttempts {
		var ability Ability
		if err := currentUpstreamAbility(exactCooldownAbility(db, key), key, upstreamRevision).
			Select("cooldown_history", "cooldown_config_changed_at", "cooldown_recovery", "cooldown_revision", "suspend_until", "adaptive_suspend_until").
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
			var applies bool
			if history, applies = mergeAbilityFailureEpisode(history, ability.AdaptiveSuspendUntil, start, completion); !applies {
				return 0, false, nil
			}
		}
		duration, err := applyAbilityFailureHistory(&ability, history)
		if err != nil {
			return 0, false, err
		}
		result := currentUpstreamAbility(exactCooldownAbility(db, key), key, upstreamRevision).Where("enabled = ? AND cooldown_revision = ? AND cooldown_config_changed_at = ?", true, ability.CooldownRevision, ability.CooldownConfigChangedAt).
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
			publishCommittedAbilityDeadline(key, ability.SuspendUntil, ability.AdaptiveSuspendUntil, ability.CooldownRevision+1)
			return duration, true, nil
		}
	}
	return 0, false, errors.New("ability cooldown changed repeatedly while recording outcome")
}

// mergeAbilityFailureEpisode folds one failure into the per-episode history.
// A rung is one failure episode, not one failed request: requests that were
// already in flight, or dispatched by a replica whose routing had not yet seen
// the cooldown, describe the same rate-limit or outage episode. So a failure
// opens a new episode (the next rung) only when its attempt was dispatched at or
// after the adaptive deadline then in force, i.e. it is a failed probe after the
// cooldown ended. A failure dispatched before that deadline joins the current
// episode: the episode's completion moves to the later failure, extending the
// cooldown at the same rung. A failure that completed no later than an already
// recorded episode adds nothing, which also makes duplicates idempotent. The
// result does not depend on the order in which concurrent outcomes are persisted.
//
// Parameters:
//   - history: the ascending episode completion times; it may be modified.
//   - deadline: the persisted adaptive deadline of the latest episode, or nil.
//   - start, completion: the failed attempt's dispatch and completion times in
//     UTC Unix nanoseconds.
//
// Returns:
//   - []int64: the updated history, capped at the ladder length.
//   - bool: false when the failure leaves the persisted state unchanged.
func mergeAbilityFailureEpisode(history []int64, deadline *time.Time, start, completion int64) ([]int64, bool) {
	if len(history) > 0 && history[len(history)-1] >= completion {
		return history, false
	}
	if len(history) > 0 && deadline != nil && start < deadline.UnixNano() {
		history[len(history)-1] = completion
		return history, true
	}
	history = append(history, completion)
	if len(history) > len(abilityCooldownLadder) {
		history = history[len(history)-len(abilityCooldownLadder):]
	}
	return history, true
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
