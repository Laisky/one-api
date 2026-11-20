package model

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestQueueAbilityCooldownRecoveryIsPartitionedByUpstreamRevision verifies that
// the recovery coalescer never lets an outcome routed with a replaced upstream
// (an older UpstreamRevision) stand in for one routed with the current upstream:
// an unwritten stale success must not discard a delayed current-revision
// failure, and must not replace a pending current-revision recovery.
func TestQueueAbilityCooldownRecoveryIsPartitionedByUpstreamRevision(t *testing.T) {
	db := useChannelPersistenceTestDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	ctx := context.Background()
	channel := &Channel{Name: "partition", Key: "old-key", Status: ChannelStatusEnabled, Models: "route", Group: "test"}
	require.NoError(t, channel.Insert())
	rotated, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)
	rotated.Key, rotated.TestingModel = "new-key", nil
	require.NoError(t, rotated.Update())
	key := cachedAbilityKey{"test", "route", channel.Id}
	t.Cleanup(func() {
		abilityRecoveries.Lock()
		for stateKey := range abilityRecoveries.byKey {
			if stateKey.ability == key {
				delete(abilityRecoveries.byKey, stateKey)
			}
		}
		abilityRecoveries.Unlock()
	})
	read := func() Ability {
		var row Ability
		require.NoError(t, exactCooldownAbility(db, key).Take(&row).Error)
		return row
	}
	const current, stale = int64(1), int64(0)
	base := time.Unix(0, read().CooldownConfigChangedAt).UTC().Add(time.Second)
	succeed := func(revision int64, at time.Time) {
		require.NoError(t, QueueAbilityCooldownRecovery(ctx, key.group, key.model, key.channelID, revision, at.Add(-time.Millisecond), at))
		drainAbilityRecoveries(t)
	}

	// Arm both partitions' rate limits (the stale write itself is fenced), then
	// leave a newer stale-revision success unwritten inside the flush interval.
	succeed(current, base)
	succeed(stale, base.Add(500*time.Millisecond))
	succeed(stale, base.Add(3*time.Second))
	got, err := RecordAbilityCooldownFailureAtRevision(ctx, key.group, key.model, key.channelID, current, base.Add(time.Second), base.Add(2*time.Second))
	require.NoError(t, err)
	require.Equal(t, abilityCooldownLadder[0], got, "a stale-revision success must not discard a current-revision failure")
	require.Equal(t, 1, read().CooldownFailures)

	// As if the streak had been recorded by another process (no local re-arm),
	// a current-revision recovery is coalesced inside the interval, then a newer
	// stale-revision success arrives. Writing the current partition must clear
	// the streak: the stale success must not have replaced it.
	abilityRecoveries.Lock()
	for stateKey, state := range abilityRecoveries.byKey {
		if stateKey.ability == key {
			state.notBefore = time.Now().Add(time.Hour)
		}
	}
	abilityRecoveries.Unlock()
	require.NoError(t, QueueAbilityCooldownRecovery(ctx, key.group, key.model, key.channelID, current, base.Add(6*time.Second), base.Add(6*time.Second)))
	require.NoError(t, QueueAbilityCooldownRecovery(ctx, key.group, key.model, key.channelID, stale, base.Add(7*time.Second), base.Add(7*time.Second)))
	drainAbilityRecoveries(t)
	require.Equal(t, 1, read().CooldownFailures, "both successes are coalesced, nothing written yet")
	flushAbilityRecovery(ctx, abilityRecoveryKey{ability: key, revision: current})
	row := read()
	require.Zero(t, row.CooldownFailures, "a stale success must not replace a pending current-revision recovery")
	require.Equal(t, base.Add(6*time.Second).UnixNano(), row.CooldownRecovery)
}
