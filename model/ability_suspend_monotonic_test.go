package model

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/config"
)

// monotonicPersistedDeadline returns the persisted suspend_until of channelID's
// ("default", "target") ability, or the zero time when none is set.
//
// Parameters:
//   - t: the test handle.
//   - db: the test database.
//   - channelID: the channel whose ability is read.
//
// Return values:
//   - time.Time: the persisted fixed deadline in UTC, or zero.
func monotonicPersistedDeadline(t *testing.T, db *gorm.DB, channelID int) time.Time {
	t.Helper()
	var ability Ability
	require.NoError(t, exactCooldownAbility(db, cachedAbilityKey{"default", "target", channelID}).Take(&ability).Error)
	if ability.SuspendUntil == nil {
		return time.Time{}
	}
	return ability.SuspendUntil.UTC()
}

// monotonicCachedDeadline returns the cached routing deadline of channelID's
// ("default", "target") ability, or the zero time when none is cached.
//
// Parameters:
//   - channelID: the channel whose cached deadline is read.
//
// Return values:
//   - time.Time: the cached deadline, or zero.
func monotonicCachedDeadline(channelID int) time.Time {
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	return cachedAbilitySuspensions[cachedAbilityKey{"default", "target", channelID}].until
}

// monotonicPickID selects a channel for ("default", "target") through the
// routing path config.MemoryCacheEnabled selects and returns its ID.
//
// Parameters:
//   - t: the test handle.
//
// Return values:
//   - int: the selected channel ID.
func monotonicPickID(t *testing.T) int {
	t.Helper()
	channel, err := CacheGetRandomSatisfiedChannelWithContext(context.Background(), "default", "target", false)
	require.NoError(t, err)
	return channel.Id
}

// TestSuspendAbilityNeverShortensActiveCooldown reproduces the review finding
// that a later, shorter suspension overwrote a longer active one: a one-hour
// cooldown (a quota exhaustion or a provider Retry-After) was cut to the 30s of a
// later plain failure on the same ability, in the database and in the cache, so
// the exhausted ability was routed again early. A suspension only ever extends
// the fixed cooldown; a longer one still extends it, an expired one is replaced,
// and a missing ability is still an error.
func TestSuspendAbilityNeverShortensActiveCooldown(t *testing.T) {
	for _, memory := range []bool{true, false} {
		t.Run(map[bool]string{true: "memory-cache", false: "database"}[memory], func(t *testing.T) {
			db := setupAbilityCacheTest(t, abilityCacheChannel(1, 10), abilityCacheChannel(2, 5))
			config.MemoryCacheEnabled = memory
			ctx := context.Background()

			require.NoError(t, SuspendAbility(ctx, "default", "target", 1, time.Hour))
			long := monotonicPersistedDeadline(t, db, 1)
			require.True(t, long.After(time.Now().UTC().Add(59*time.Minute)), "control: the hour-long cooldown is persisted")

			require.NoError(t, SuspendAbility(ctx, "default", "target", 1, 30*time.Second),
				"a shorter suspension of a cooling ability is not an error")
			require.Equal(t, long, monotonicPersistedDeadline(t, db, 1), "the persisted deadline must not move earlier")
			if memory {
				require.Equal(t, long, monotonicCachedDeadline(1), "the cached deadline must not move earlier")
			}
			for range 20 {
				require.Equal(t, 2, monotonicPickID(t), "the hour-long cooldown must survive a later 30s suspension")
			}

			require.NoError(t, SuspendAbility(ctx, "default", "target", 1, 2*time.Hour))
			extended := monotonicPersistedDeadline(t, db, 1)
			require.True(t, extended.After(long.Add(30*time.Minute)), "a longer suspension extends the deadline")
			if memory {
				require.Equal(t, extended, monotonicCachedDeadline(1), "the extension is published")
			}

			require.NoError(t, SuspendAbility(ctx, "default", "target", 2, -time.Second))
			require.NoError(t, SuspendAbility(ctx, "default", "target", 2, time.Hour))
			require.True(t, monotonicPersistedDeadline(t, db, 2).After(time.Now().UTC().Add(30*time.Minute)),
				"an expired deadline is replaced by a new one")

			require.Error(t, SuspendAbility(ctx, "default", "missing", 1, time.Hour), "a missing ability is still reported")
		})
	}
}

// TestSuspendAbilityNoOpPublishesPersistedDeadline pins that a suspension kept
// out by a longer persisted cooldown still refreshes this replica's cache from
// the database, merging the fixed and adaptive deadlines. A cooldown persisted
// by another replica (not yet mirrored here) therefore takes effect at the next
// local suspension instead of the shorter one being cached.
func TestSuspendAbilityNoOpPublishesPersistedDeadline(t *testing.T) {
	for _, column := range []string{"suspend_until", "adaptive_suspend_until"} {
		t.Run(column, func(t *testing.T) {
			db := setupAbilityCacheTest(t, abilityCacheChannel(1, 10), abilityCacheChannel(2, 5))
			ctx := context.Background()
			remote := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
			require.NoError(t, exactCooldownAbility(db, cachedAbilityKey{"default", "target", 1}).
				Update(column, remote).Error)
			require.True(t, monotonicCachedDeadline(1).IsZero(), "control: the remote cooldown is not cached yet")

			require.NoError(t, SuspendAbility(ctx, "default", "target", 1, 30*time.Second))

			require.True(t, remote.Equal(monotonicCachedDeadline(1)), "the longer persisted deadline is published")
			for range 20 {
				require.Equal(t, 2, monotonicPickID(t))
			}
		})
	}
}
