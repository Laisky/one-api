package model

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/graceful"
)

// drainAbilityRecoveries waits for queued recovery flush tasks to finish.
func drainAbilityRecoveries(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, graceful.Drain(ctx))
}

// TestQueueAbilityCooldownRecoveryCoalescesAndSupersedes verifies the success
// coalescer: the first success is written at once, later successes inside the
// flush interval cost no write, an unwritten newer success discards an older
// failure without opening a cooldown, and a failure recorded by this process
// re-arms recovery so the next success clears the streak immediately.
func TestQueueAbilityCooldownRecoveryCoalescesAndSupersedes(t *testing.T) {
	db := setupTestDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previous := DB
	DB = db
	t.Cleanup(func() { DB = previous })
	ctx := context.Background()
	key := cachedAbilityKey{"test", "coalesce", 733}
	require.NoError(t, db.Create(&Ability{Group: key.group, Model: key.model, ChannelId: key.channelID, Enabled: true}).Error)
	t.Cleanup(func() {
		abilityRecoveries.Lock()
		delete(abilityRecoveries.byKey, key)
		abilityRecoveries.Unlock()
	})
	read := func() Ability {
		var row Ability
		require.NoError(t, exactCooldownAbility(db, key).Take(&row).Error)
		return row
	}
	succeed := func(at time.Time) {
		require.NoError(t, QueueAbilityCooldownRecovery(ctx, key.group, key.model, key.channelID, AbilityUpstreamRevisionUnknown, at.Add(-time.Millisecond), at))
		drainAbilityRecoveries(t)
	}
	base := time.Now().UTC()

	succeed(base)
	require.Equal(t, base.UnixNano(), read().CooldownRecovery, "the first success is written at once")
	revision := read().CooldownRevision
	succeed(base.Add(time.Second))
	require.Equal(t, revision, read().CooldownRevision, "a success inside the interval costs no write")

	got, err := RecordAbilityCooldownFailure(ctx, key.group, key.model, key.channelID, base.Add(100*time.Millisecond), base.Add(500*time.Millisecond))
	require.NoError(t, err)
	require.Zero(t, got, "an unwritten newer local success supersedes an older failure")
	require.Zero(t, read().CooldownFailures)

	got, err = RecordAbilityCooldownFailure(ctx, key.group, key.model, key.channelID, base.Add(2*time.Second), base.Add(3*time.Second))
	require.NoError(t, err)
	require.Equal(t, abilityCooldownLadder[0], got)
	require.Equal(t, 1, read().CooldownFailures)

	succeed(base.Add(4 * time.Second))
	row := read()
	require.Zero(t, row.CooldownFailures, "a local failure re-arms recovery for the next success")
	require.Nil(t, row.AdaptiveSuspendUntil)
	require.Equal(t, base.Add(4*time.Second).UnixNano(), row.CooldownRecovery)
}
