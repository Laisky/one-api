package model

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestAdaptiveAbilityCooldownRestartsOnReenable verifies that both channel
// status writers restart a frozen adaptive streak when a disabled channel is
// re-enabled, fence outcomes of attempts dispatched before the re-enable, keep
// the independent fixed suspension, and leave an enabled channel's streak alone
// when it is merely set to enabled again.
func TestAdaptiveAbilityCooldownRestartsOnReenable(t *testing.T) {
	for _, automatic := range []bool{false, true} {
		name := "admin"
		if automatic {
			name = "automatic"
		}
		t.Run(name, func(t *testing.T) {
			db := setupTestDB(t)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			previous := DB
			DB = db
			t.Cleanup(func() { DB = previous })
			ctx := context.Background()
			channel := &Channel{Id: 732, Group: "test", Models: "route", Status: ChannelStatusEnabled}
			require.NoError(t, db.Create(channel).Error)
			require.NoError(t, syncAbilitiesWithDB(db, channel, false))
			key := cachedAbilityKey{"test", "route", channel.Id}
			read := func() Ability {
				var row Ability
				require.NoError(t, exactCooldownAbility(db, key).Take(&row).Error)
				return row
			}
			fail := func(start, completion time.Time) time.Duration {
				got, err := RecordAbilityCooldownFailure(ctx, key.group, key.model, key.channelID, start, completion)
				require.NoError(t, err)
				return got
			}
			setStatus := func(status int) {
				if !automatic {
					require.NoError(t, SetChannelStatusWithContext(ctx, channel.Id, status))
					return
				}
				from := ChannelStatusAutoDisabled
				if status != ChannelStatusEnabled {
					from = ChannelStatusEnabled
				}
				change, err := TransitionChannelStatusWithContext(ctx, channel.Id, status, from)
				require.NoError(t, err)
				require.True(t, change.Changed)
			}

			// Attempts dispatched after the ability was created (its fence), each a
			// failed probe once the previous cooldown ended.
			event := time.Unix(0, read().CooldownConfigChangedAt).UTC().Add(time.Nanosecond)
			for _, rung := range abilityCooldownLadder[:3] {
				require.Equal(t, rung, fail(event, event))
				event = event.Add(rung)
			}
			fixed := time.Now().UTC().Truncate(time.Second).Add(time.Hour)
			require.NoError(t, exactCooldownAbility(db, key).Update("suspend_until", fixed).Error)

			// Setting an enabled channel to enabled is not a re-enable.
			if !automatic {
				setStatus(ChannelStatusEnabled)
				require.Equal(t, 3, read().CooldownFailures)
			}
			disabled := ChannelStatusManuallyDisabled
			if automatic {
				disabled = ChannelStatusAutoDisabled
			}
			inFlight := time.Now().UTC()
			setStatus(disabled)
			setStatus(ChannelStatusEnabled)
			row := read()
			require.True(t, row.Enabled)
			require.Zero(t, row.CooldownFailures)
			require.Empty(t, row.CooldownHistory)
			require.Nil(t, row.AdaptiveSuspendUntil)
			require.Equal(t, fixed, *row.SuspendUntil, "the fixed suspension is independent of the adaptive streak")
			require.Greater(t, row.CooldownConfigChangedAt, inFlight.UnixNano())

			after := time.Unix(0, row.CooldownConfigChangedAt).UTC().Add(time.Second)
			require.Zero(t, fail(inFlight, after), "an attempt dispatched before the re-enable is fenced")
			require.Equal(t, abilityCooldownLadder[0], fail(after, after), "the next failure restarts at the first rung")
		})
	}
}
