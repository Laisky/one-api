package model

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/config"
)

// capacityTestChannel builds a cache-test channel for model "target" at
// priority whose model configs are configs (empty for no model config at all).
//
// Parameters:
//   - id, priority: the channel identity and routing tier.
//   - configs: the raw model_configs JSON, or "" for none.
//
// Return values:
//   - *Channel: the unsaved channel.
func capacityTestChannel(id int, priority int64, configs string) *Channel {
	channel := abilityCacheChannel(id, priority)
	if configs != "" {
		channel.ModelConfigs = &configs
	}
	return channel
}

// capacityTestCool persists column for channelID's ("default", "target")
// ability and rebuilds the routing cache so both routing paths observe it.
//
// Parameters:
//   - t: the test handle.
//   - db: the test database.
//   - channelID: the channel whose ability cools down.
//   - column: "suspend_until" or "adaptive_suspend_until".
//   - until: the deadline to persist.
func capacityTestCool(t *testing.T, db *gorm.DB, channelID int, column string, until time.Time) {
	t.Helper()
	require.NoError(t, exactCooldownAbility(db, cachedAbilityKey{"default", "target", channelID}).
		Update(column, until.UTC()).Error)
	require.NoError(t, InitChannelCacheContext(context.Background()))
}

// TestCapacityFloorContract pins the capacity semantics shared by both routing
// paths: an undeclared max_tokens (no model config, or max_tokens unset) is
// unknown and never proven too small; a declared one is eligible only when it is
// strictly larger than the largest declared capacity that answered 413, and a
// smaller rejection never lowers the floor.
func TestCapacityFloorContract(t *testing.T) {
	declared := capacityTestChannel(1, 0, `{"target": {"max_tokens": 8192}}`)
	unset := capacityTestChannel(2, 0, `{"target": {"ratio": 1}}`)
	missing := capacityTestChannel(3, 0, "")
	otherModel := capacityTestChannel(4, 0, `{"other": {"max_tokens": 4096}}`)

	require.Equal(t, int32(8192), ChannelModelCapacity(declared, "target"))
	for _, channel := range []*Channel{unset, missing, otherModel, nil} {
		require.Zero(t, ChannelModelCapacity(channel, "target"), "undeclared capacity is 0")
	}

	floor := RaiseCapacityFloor(NoCapacityFloor, unset, "target")
	require.Equal(t, NoCapacityFloor, floor, "an undeclared rejection proves nothing")
	floor = RaiseCapacityFloor(floor, declared, "target")
	require.Equal(t, int32(8192), floor)
	floor = RaiseCapacityFloor(floor, capacityTestChannel(5, 0, `{"target": {"max_tokens": 4096}}`), "target")
	require.Equal(t, int32(8192), floor, "a smaller rejection never lowers the floor")

	for _, tc := range []struct {
		name     string
		channel  *Channel
		floor    int32
		eligible bool
	}{
		{"no floor keeps a declared channel", declared, NoCapacityFloor, true},
		{"equal capacity is too small", declared, 8192, false},
		{"smaller capacity is too small", declared, 16384, false},
		{"larger capacity qualifies", declared, 4096, true},
		{"unset max_tokens stays eligible", unset, 8192, true},
		{"missing model config stays eligible", missing, 8192, true},
	} {
		require.Equal(t, tc.eligible, capacityExceedsFloor(tc.channel, "target", tc.floor), tc.name)
	}
}

// TestCountRetryCandidatesIncludesCoolingAbilities pins the retry ceiling on
// both routing paths: every enabled, untried ability of the pool counts,
// including one cooling under the fixed or the adaptive deadline, while excluded
// channels and disabled abilities do not. The routable count (what selection
// may dispatch now) still drops the cooling abilities.
func TestCountRetryCandidatesIncludesCoolingAbilities(t *testing.T) {
	for _, memory := range []bool{true, false} {
		t.Run(map[bool]string{true: "memory-cache", false: "database"}[memory], func(t *testing.T) {
			db := setupAbilityCacheTest(t,
				abilityCacheChannel(1, 10), abilityCacheChannel(2, 5), abilityCacheChannel(3, 3),
				abilityCacheChannel(4, 0), abilityCacheChannel(5, 0))
			require.NoError(t, db.Model(&Ability{}).Where("channel_id = ?", 5).Update("enabled", false).Error)
			require.NoError(t, InitChannelCacheContext(context.Background()))
			capacityTestCool(t, db, 2, "suspend_until", time.Now().Add(time.Hour))
			capacityTestCool(t, db, 3, "adaptive_suspend_until", time.Now().Add(time.Hour))
			config.MemoryCacheEnabled = memory
			ctx := context.Background()

			count, err := CountRetryCandidates(ctx, "default", "target", map[int]bool{1: true})
			require.NoError(t, err)
			require.Equal(t, 3, count, "channels 2-4 are untried; cooling ones keep their slot, the disabled one has none")

			count, err = CountRetryCandidates(ctx, "default", "target", map[int]bool{1: true, 4: true})
			require.NoError(t, err)
			require.Equal(t, 2, count)

			routable, err := CountAvailableChannels(ctx, "default", "target")
			require.NoError(t, err)
			require.Equal(t, 2, routable, "control: both cooldown columns still hide channels from routing")
		})
	}
}

// TestCapacityFloorSelectionHonorsBothCooldowns pins that the capacity-aware
// selector keeps the shared eligibility rules on both routing paths: a channel
// large enough for the floor is still never selected while it cools down under
// either deadline, and a channel too small for the floor is skipped even when
// it is the highest remaining tier.
func TestCapacityFloorSelectionHonorsBothCooldowns(t *testing.T) {
	for _, memory := range []bool{true, false} {
		for _, column := range []string{"suspend_until", "adaptive_suspend_until"} {
			t.Run(map[bool]string{true: "memory-cache", false: "database"}[memory]+"/"+column, func(t *testing.T) {
				db := setupAbilityCacheTest(t,
					capacityTestChannel(1, 10, `{"target": {"max_tokens": 4096}}`),
					capacityTestChannel(2, 5, `{"target": {"max_tokens": 16384}}`),
					capacityTestChannel(3, 3, `{"target": {"max_tokens": 8192}}`),
					capacityTestChannel(4, 0, ""),
				)
				capacityTestCool(t, db, 2, column, time.Now().Add(time.Hour))
				config.MemoryCacheEnabled = memory
				ctx := context.Background()

				for range 10 {
					channel, err := CacheGetRandomSatisfiedChannelAboveCapacityWithContext(ctx, "default", "target", false, map[int]bool{}, 4096)
					require.NoError(t, err)
					require.Equal(t, 3, channel.Id, "the cooling 16k channel is skipped; the 8k channel is the highest eligible tier")
				}
				channel, err := CacheGetRandomSatisfiedChannelAboveCapacityWithContext(ctx, "default", "target", false, map[int]bool{3: true}, 4096)
				require.NoError(t, err)
				require.Equal(t, 4, channel.Id, "an undeclared capacity stays eligible")

				_, err = CacheGetRandomSatisfiedChannelAboveCapacityWithContext(ctx, "default", "target", false, map[int]bool{3: true, 4: true}, 4096)
				require.Error(t, err)
				require.Contains(t, err.Error(), "no available channels support model", "exhaustion keeps the classified wording")
			})
		}
	}
}

// TestTryLargerMaxTokensTreatsExcludedChannelsAsRejections pins the legacy
// selector flag on both routing paths: every excluded channel counts as a 413
// rejection, so the floor is the largest capacity any of them declares, and a
// smaller channel is skipped rather than only one of an equal size.
func TestTryLargerMaxTokensTreatsExcludedChannelsAsRejections(t *testing.T) {
	for _, memory := range []bool{true, false} {
		t.Run(map[bool]string{true: "memory-cache", false: "database"}[memory], func(t *testing.T) {
			setupAbilityCacheTest(t,
				capacityTestChannel(1, 10, `{"target": {"max_tokens": 8192}}`),
				capacityTestChannel(2, 5, `{"target": {"max_tokens": 4096}}`),
				capacityTestChannel(3, 0, `{"target": {"max_tokens": 16384}}`),
			)
			config.MemoryCacheEnabled = memory

			for range 10 {
				channel, err := CacheGetRandomSatisfiedChannelExcluding("default", "target", false, map[int]bool{1: true}, true)
				require.NoError(t, err)
				require.Equal(t, 3, channel.Id, "the 4k channel cannot serve a request the 8k channel rejected")
			}
			channel, err := CacheGetRandomSatisfiedChannelExcluding("default", "target", false, map[int]bool{1: true}, false)
			require.NoError(t, err)
			require.Equal(t, 2, channel.Id, "control: without the flag the walk is a plain priority walk")
		})
	}
}
