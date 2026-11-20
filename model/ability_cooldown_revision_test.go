package model

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestAbilityCooldownUpstreamRevisionFence verifies that Channel.Update advances
// UpstreamRevision only when the upstream identity changes, that the revision is
// never accepted from API JSON, and that cooldown outcomes routed with an older
// revision are discarded while outcomes at the current revision apply.
func TestAbilityCooldownUpstreamRevisionFence(t *testing.T) {
	db := useChannelPersistenceTestDB(t)
	ctx := context.Background()
	channel := &Channel{Name: "revision", Key: "old-key", Status: ChannelStatusEnabled, Models: "route", Group: "test"}
	require.NoError(t, channel.Insert())
	load := func() *Channel {
		loaded, err := GetChannelById(channel.Id, true)
		require.NoError(t, err)
		loaded.TestingModel = nil
		return loaded
	}
	require.Zero(t, load().UpstreamRevision)

	renamed := load()
	renamed.Name = "renamed"
	require.NoError(t, renamed.Update())
	require.Zero(t, load().UpstreamRevision, "a non-upstream edit keeps the revision")

	var injected Channel
	require.NoError(t, json.Unmarshal([]byte(`{"UpstreamRevision":99,"upstream_revision":99}`), &injected))
	require.Zero(t, injected.UpstreamRevision, "the revision is not accepted from API input")

	rotated := load()
	rotated.Key = "new-key"
	require.NoError(t, rotated.Update())
	require.EqualValues(t, 1, load().UpstreamRevision)

	key := cachedAbilityKey{"test", "route", channel.Id}
	read := func() Ability {
		var row Ability
		require.NoError(t, exactCooldownAbility(db, key).Take(&row).Error)
		return row
	}
	at := time.Unix(0, read().CooldownConfigChangedAt).UTC().Add(time.Second)
	stale, err := RecordAbilityCooldownFailureAtRevision(ctx, key.group, key.model, key.channelID, 0, at, at)
	require.NoError(t, err)
	require.Zero(t, stale, "an attempt routed with the replaced upstream is discarded")
	require.Zero(t, read().CooldownFailures)

	current, err := RecordAbilityCooldownFailureAtRevision(ctx, key.group, key.model, key.channelID, 1, at, at)
	require.NoError(t, err)
	require.Equal(t, abilityCooldownLadder[0], current)
	require.Equal(t, 1, read().CooldownFailures)

	require.NoError(t, ResetAbilityCooldownAtRevision(ctx, key.group, key.model, key.channelID, 0, at, at.Add(time.Second)))
	require.Equal(t, 1, read().CooldownFailures, "a success on the replaced upstream does not clear the new streak")
	require.NoError(t, ResetAbilityCooldownAtRevision(ctx, key.group, key.model, key.channelID, 1, at, at.Add(time.Second)))
	require.Zero(t, read().CooldownFailures)
}
